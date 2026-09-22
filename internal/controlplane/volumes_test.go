package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCommonActionAllowlistRejectsUnreviewedActionWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	api := NewAGS(Config{Region: "ap-test", Endpoint: "ags.tencentcloudapi.com", Timeout: time.Second, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network")
	})}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	if err := api.commonCall(context.Background(), "CreateSandboxTool", map[string]any{}, &struct{}{}); err == nil {
		t.Fatal("unreviewed CommonClient Action accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("unreviewed Action made %d calls", calls.Load())
	}
}

func TestCommonVolumeCancellationDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := api.ListVolumes(ctx, VolumeListRequest{Limit: 1})
	if !errors.Is(err, context.Canceled) || calls.Load() > 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestCommonVolumeRejectsMalformedCollections(t *testing.T) {
	for _, tc := range []struct {
		value     any
		requestID string
	}{{nil, "r"}, {map[string]any{}, "r"}, {"bad", "r"}, {[]any{}, ""}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"VolumeSet": tc.value, "TotalCount": 0, "RequestId": tc.requestID}})
		}))
		api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
		_, err := api.ListVolumes(context.Background(), VolumeListRequest{Limit: 1})
		server.Close()
		var cloud *Error
		if !errors.As(err, &cloud) || cloud.Code != "ClientError.MalformedResponse" {
			t.Fatalf("value=%#v requestID=%q error=%#v", tc.value, tc.requestID, err)
		}
	}
}

func TestCommonVolumeTemplateRejectsMalformedCollectionAndStorage(t *testing.T) {
	tests := []map[string]any{
		{"VolumeTemplateSet": "bad", "TotalCount": 0, "RequestId": "r"},
		{"VolumeTemplateSet": []any{map[string]any{
			"VolumeTemplateId": "volt", "VolumeTemplateName": "template", "AccessMode": "ReadWriteMany", "ReclaimPolicy": "Retain", "Status": "ACTIVE", "StorageType": "Cfs",
			"StorageSpec": map[string]any{"Cos": map[string]any{"Endpoint": "e", "BucketName": "b", "BucketPathPattern": "p"}},
		}}, "TotalCount": 1, "RequestId": "r"},
	}
	for _, response := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
		}))
		api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
		_, err := api.ListVolumeTemplates(context.Background(), VolumeListRequest{Limit: 1})
		server.Close()
		var cloud *Error
		if !errors.As(err, &cloud) || cloud.Code != "ClientError.MalformedResponse" {
			t.Fatalf("response=%#v error=%#v", response, err)
		}
	}
}

func TestCommonVolumeListKeepsUnknownStorageOpaque(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{
			"VolumeSet": []any{map[string]any{
				"VolumeId": "vol-opaque", "VolumeName": "opaque", "AccessMode": "ReadWriteOnce", "ReclaimPolicy": "Retain", "Status": "ACTIVE", "StorageType": "AgentCbs",
				"Storage": map[string]any{"AgentCbs": map[string]any{"DiskId": "must-not-escape"}},
			}},
			"TotalCount": 1, "RequestId": "r",
		}})
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) {
		return Credential{SecretID: "id", SecretKey: "key"}, nil
	})
	page, err := api.ListVolumes(context.Background(), VolumeListRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].StorageType != "AgentCbs" || page.Items[0].Storage.COS != nil || page.Items[0].Storage.CFS != nil {
		t.Fatalf("opaque Volume = %+v", page.Items)
	}
	if value, err := volumeStorageSpecFromWire("FutureBackend", volumeStorageSpecWire{}); err != nil || value.COS != nil || value.CFS != nil {
		t.Fatalf("opaque VolumeTemplate storage = %+v, %v", value, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
