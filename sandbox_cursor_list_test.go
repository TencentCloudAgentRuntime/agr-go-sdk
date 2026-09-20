package agr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSandboxCursorListWireAndOpaqueContinuation(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		if _, exists := request["Offset"]; exists {
			t.Error("cursor request contains Offset")
		}
		if _, exists := request["Limit"]; exists {
			t.Error("cursor request contains Limit")
		}
		response := map[string]any{"InstanceSet": []any{}, "TotalCount": 2, "RequestId": "cursor-request"}
		if len(requests) == 1 {
			response["InstanceSet"] = []any{instanceJSON("RUNNING")}
			response["NextToken"] = "opaque/+=token"
		} else {
			response["NextToken"] = "still-more"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	opts := SandboxPageOptions{Filter: SandboxFilter{InstanceIDs: []string{"sb-test"}, ToolID: "tool-id", States: []SandboxState{Running}, Metadata: map[string][]string{"job": {"sdk"}}}, MaxResults: 1, NeedTotalCount: true}
	first, err := client.Sandboxes().ListPage(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.NextToken != "opaque/+=token" || first.TotalCount == nil || *first.TotalCount != 2 || len(first.Items) != 1 {
		t.Fatalf("first page = %+v", first)
	}
	opts.NextToken = first.NextToken
	second, err := client.Sandboxes().ListPage(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.NextToken != "still-more" || second.TotalCount == nil || len(second.Items) != 0 {
		t.Fatalf("empty continuation page = %+v", second)
	}
	wantFirst := map[string]any{"InstanceIds": []any{"sb-test"}, "ToolId": "tool-id", "MaxResults": float64(1), "NeedTotalCount": true, "Filters": []any{map[string]any{"Name": "Status", "Values": []any{"RUNNING"}}, map[string]any{"Name": "metadata:job", "Values": []any{"sdk"}}}}
	if !reflect.DeepEqual(requests[0], wantFirst) {
		t.Fatalf("first request = %#v; want %#v", requests[0], wantFirst)
	}
	if requests[1]["NextToken"] != "opaque/+=token" {
		t.Fatalf("continuation request = %#v", requests[1])
	}
}

func TestSandboxCursorListUnknownTotalAndCopy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"InstanceSet": []any{}, "TotalCount": 0, "RequestId": "r"}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	page, err := client.Sandboxes().ListPage(context.Background(), SandboxPageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalCount != nil {
		t.Fatalf("unrequested total = %v", page.TotalCount)
	}
	input := SandboxPageOptions{Filter: SandboxFilter{InstanceIDs: []string{"before"}, States: []SandboxState{Stopped}, Metadata: map[string][]string{"key": {"before"}}}}
	copy := copyPageOptions(input)
	input.Filter.InstanceIDs[0] = "after"
	input.Filter.States[0] = Running
	input.Filter.Metadata["key"][0] = "after"
	if copy.Filter.InstanceIDs[0] != "before" || copy.Filter.States[0] != Stopped || copy.Filter.Metadata["key"][0] != "before" {
		t.Fatal("cursor options retain caller collections")
	}
}

func TestSandboxCursorListRejectsInvalidInputWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFuncRoot(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected network")
	})
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: transport}))
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "sb"
	}
	cases := []SandboxPageOptions{{MaxResults: -1}, {MaxResults: 101}, {Filter: SandboxFilter{InstanceIDs: ids}}, {Filter: SandboxFilter{InstanceIDs: []string{""}}}, {Filter: SandboxFilter{Metadata: map[string][]string{"key": {"a", "b"}}}}, {Filter: SandboxFilter{States: []SandboxState{Resuming}}}}
	for _, opts := range cases {
		if _, err := client.Sandboxes().ListPage(context.Background(), opts); err == nil {
			t.Fatalf("accepted invalid options %+v", opts)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests made %d HTTP calls", calls.Load())
	}
}

func TestSandboxCursorListDoesNotRestartExpiredToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"RequestId": "expired-request", "Error": map[string]any{"Code": "InvalidParameter.NextToken", "Message": "expired secret body"}}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	_, err := client.Sandboxes().ListPage(context.Background(), SandboxPageOptions{NextToken: "expired"})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != InvalidArgument || failure.RequestID != "expired-request" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %#v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expired token retried %d times", calls.Load())
	}
}

func TestSandboxCursorListCancellation(t *testing.T) {
	transport := roundTripFuncRoot(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint("https://cloud.example.test"), WithHTTPClient(&http.Client{Transport: transport}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.Sandboxes().ListPage(ctx, SandboxPageOptions{})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != DeadlineExceeded {
		t.Fatalf("error = %#v", err)
	}
}

type roundTripFuncRoot func(*http.Request) (*http.Response, error)

func (f roundTripFuncRoot) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
