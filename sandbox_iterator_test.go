package agr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSandboxIteratorCrossesEmptyPagesAndCopiesQuery(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		response := map[string]any{"InstanceSet": []any{}, "RequestId": "iterator-fixture"}
		switch request["NextToken"] {
		case nil:
			response["InstanceSet"] = []any{instanceJSON("RUNNING")}
			response["NextToken"] = "opaque-one"
		case "opaque-one":
			response["NextToken"] = "opaque-two"
		case "opaque-two":
			item := instanceJSON("RUNNING")
			item["InstanceId"] = "sb-next"
			response["InstanceSet"] = []any{item}
		default:
			t.Errorf("unexpected cursor %v", request["NextToken"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	opts := SandboxPageOptions{MaxResults: 1, Filter: SandboxFilter{InstanceIDs: []string{"sb-test"}, Metadata: map[string][]string{"job": {"before"}}}}
	iterator := client.Sandboxes().Iterate(opts)
	defer iterator.Close()
	opts.Filter.InstanceIDs[0] = "after"
	opts.Filter.Metadata["job"][0] = "after"
	for _, want := range []string{"sb-test", "sb-next"} {
		item, err := iterator.Next(context.Background())
		if err != nil || item.ID != want {
			t.Fatalf("Next = %+v, %v; want %s", item, err, want)
		}
	}
	if _, err := iterator.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final Next = %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %d", len(requests))
	}
	for _, request := range requests {
		if !reflect.DeepEqual(request["InstanceIds"], []any{"sb-test"}) {
			t.Fatalf("query was mutated: %#v", request)
		}
		filters, ok := request["Filters"].([]any)
		if !ok || len(filters) != 1 || !reflect.DeepEqual(filters[0], map[string]any{"Name": "metadata:job", "Values": []any{"before"}}) {
			t.Fatalf("metadata filter was mutated: %#v", request)
		}
	}
	if requests[1]["NextToken"] != "opaque-one" || requests[2]["NextToken"] != "opaque-two" {
		t.Fatalf("cursor forwarding = %#v", requests)
	}
}

func TestSandboxIteratorCloseCancelsInFlightRequest(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	transport := roundTripFuncRoot(func(r *http.Request) (*http.Response, error) {
		once.Do(func() { close(entered) })
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint("https://cloud.example.test"), WithHTTPClient(&http.Client{Transport: transport}))
	iterator := client.Sandboxes().Iterate(SandboxPageOptions{})
	result := make(chan error, 1)
	go func() { _, err := iterator.Next(context.Background()); result <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("iterator request did not start")
	}
	if err := iterator.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		var failure *Error
		if !errors.As(err, &failure) || failure.Code != Canceled {
			t.Fatalf("in-flight Next = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the request")
	}
	if _, err := iterator.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("Next after Close = %v", err)
	}
}

func TestSandboxIteratorRejectsRepeatedEmptyCursor(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"InstanceSet": []any{}, "NextToken": "loop", "RequestId": "iterator-fixture"}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	iterator := client.Sandboxes().Iterate(SandboxPageOptions{})
	defer iterator.Close()
	_, err := iterator.Next(context.Background())
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != Protocol || requests != 2 {
		t.Fatalf("repeated empty cursor: error=%v requests=%d", err, requests)
	}
}

func TestSandboxIteratorPreservesServerErrorWithoutRestarting(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		response := map[string]any{"InstanceSet": []any{instanceJSON("RUNNING")}, "NextToken": "expired", "RequestId": "page-one"}
		if requests == 2 {
			response = map[string]any{"RequestId": "expired-request", "Error": map[string]any{"Code": "InvalidParameter.NextToken", "Message": "sensitive detail"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	iterator := client.Sandboxes().Iterate(SandboxPageOptions{})
	defer iterator.Close()
	if item, err := iterator.Next(context.Background()); err != nil || item.ID != "sb-test" {
		t.Fatalf("first page = %+v, %v", item, err)
	}
	_, err = iterator.Next(context.Background())
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != InvalidArgument || failure.RequestID != "expired-request" || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("expired cursor error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("expired cursor triggered %d requests", requests)
	}
}
