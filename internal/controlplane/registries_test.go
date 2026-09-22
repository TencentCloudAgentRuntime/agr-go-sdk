package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegistryCommonRequestMappingCoversAllA5aActions(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		action := r.Header.Get("X-Tc-Action")
		mu.Lock()
		bodies[action] = append(bodies[action], body)
		mu.Unlock()
		response := registryResponseForAction(action)
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-guangzhou", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	ctx := context.Background()
	reg, rec, ver := "reg-12345678", "rec-12345678", "rv-12345678"
	if _, err := api.CreateRegistry(ctx, CreateRegistryRequest{Name: "test", ApprovalMode: "MANUAL", Tags: []RegistryTag{{Key: "k", Value: "v"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetRegistry(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListRegistries(ctx, RegistryListRequest{Limit: 1, Filters: []RegistryFilter{{Name: "name", Values: []string{"test"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.UpdateRegistry(ctx, reg, UpdateRegistryRequest{Description: ""}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteRegistry(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := api.CreateRegistryRecord(ctx, reg, CreateRegistryRecordRequest{Name: "custom", DescriptorType: "CUSTOM", CustomDescriptors: json.RawMessage(`{"kind":"custom"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetRegistryRecord(ctx, reg, rec, &RegistrySelector{Label: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListRegistryRecords(ctx, reg, RegistryRecordListRequest{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := api.UpdateRegistryRecord(ctx, reg, rec, UpdateRegistryRecordRequest{Description: &empty}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteRegistryRecord(ctx, reg, rec, DeleteRegistryRecordRequest{VersionID: &empty, Reason: "boundary"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListRegistryRecordVersions(ctx, reg, rec, RegistryRecordListRequest{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	for _, decision := range []string{"approve", "reject", "cancel"} {
		if _, _, err := api.DecideRegistryRecord(ctx, decision, reg, rec, ver, "review"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := api.PreviewRegistryRecord(ctx, reg, rec, &RegistrySelector{VersionID: ver}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.SyncRegistryRecord(ctx, reg, rec, &RegistrySelector{VersionID: ver}, "sync"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListRegistryAuditLogs(ctx, reg, RegistryAuditLogRequest{RecordID: rec, Limit: 1}); err != nil {
		t.Fatal(err)
	}

	want := []string{"ApproveRegistryRecord", "CancelRegistryRecord", "CreateRegistry", "CreateRegistryRecord", "DeleteRegistry", "DeleteRegistryRecord", "DescribeRegistry", "DescribeRegistryAuditLogList", "DescribeRegistryList", "DescribeRegistryRecord", "DescribeRegistryRecordList", "DescribeRegistryRecordVersionList", "PreviewRegistryRecord", "RejectRegistryRecord", "SyncRegistryRecord", "UpdateRegistry", "UpdateRegistryRecord"}
	for _, action := range want {
		if len(bodies[action]) == 0 {
			t.Errorf("missing request for %s", action)
		}
	}
	if got := bodies["CreateRegistryRecord"][0]["CustomDescriptors"]; got != `{"kind":"custom"}` {
		t.Fatalf("CustomDescriptors encoded incorrectly: %#v", got)
	}
	if got, ok := bodies["UpdateRegistry"][0]["Description"]; !ok || got != "" {
		t.Fatalf("empty Description lost: %#v", bodies["UpdateRegistry"][0])
	}
	if got, ok := bodies["DeleteRegistryRecord"][0]["VersionId"]; !ok || got != "" {
		t.Fatalf("explicit empty VersionId lost: %#v", bodies["DeleteRegistryRecord"][0])
	}
	if got := bodies["DescribeRegistryRecord"][0]["Label"]; got != "stable" {
		t.Fatalf("selector mapping lost: %#v", bodies["DescribeRegistryRecord"][0])
	}
	if got := bodies["PreviewRegistryRecord"][0]["VersionId"]; got != ver {
		t.Fatalf("VersionId wire name lost: %#v", bodies["PreviewRegistryRecord"][0])
	}
	if _, leaked := bodies["PreviewRegistryRecord"][0]["VersionID"]; leaked {
		t.Fatalf("Go acronym leaked into wire request: %#v", bodies["PreviewRegistryRecord"][0])
	}
	if got := bodies["DescribeRegistryAuditLogList"][0]["RecordId"]; got != rec {
		t.Fatalf("RecordId wire name lost: %#v", bodies["DescribeRegistryAuditLogList"][0])
	}
	if _, leaked := bodies["DescribeRegistryAuditLogList"][0]["RecordID"]; leaked {
		t.Fatalf("Go acronym leaked into wire request: %#v", bodies["DescribeRegistryAuditLogList"][0])
	}
}

func TestRegistrySourceInputsAreEncodedOnce(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": registryResponseForAction("CreateRegistryRecord")})
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	requests := []CreateRegistryRecordRequest{
		{Name: "mcp", DescriptorType: "MCP", MCPSource: &RegistrySource{Type: "MANUAL", Descriptors: json.RawMessage(`{"name":"mcp"}`)}},
		{Name: "a2a", DescriptorType: "A2A", AgentSource: &RegistrySource{Type: "URL_IMPORT", EndpointURL: "https://example.invalid/agent.json"}},
		{Name: "skill", DescriptorType: "AGENT_SKILLS", SkillSource: &RegistrySkillSource{Type: "MANUAL", SkillMD: "# Skill"}},
	}
	for _, request := range requests {
		if _, err := api.CreateRegistryRecord(context.Background(), "reg", request); err != nil {
			t.Fatal(err)
		}
	}
	if got := bodies[0]["MCPSource"].(map[string]any)["Descriptors"]; got != `{"name":"mcp"}` {
		t.Fatalf("MCP descriptors=%#v", got)
	}
	if got := bodies[1]["AgentSource"].(map[string]any)["EndpointURL"]; got != "https://example.invalid/agent.json" {
		t.Fatalf("Agent URL=%#v", got)
	}
	if got := bodies[2]["SkillSource"].(map[string]any)["SkillMd"]; got != "# Skill" {
		t.Fatalf("SkillMd=%#v", got)
	}
}

func TestRegistrySyncCancellationDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := api.SyncRegistryRecord(ctx, "reg", "rec", nil, "")
	if !errors.Is(err, context.Canceled) || calls.Load() > 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestRegistryPreviewRejectsOversizedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		result := `{"body":"` + strings.Repeat("x", maxRegistryJSONBytes) + `"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"PreviewResult": result, "ResolvedVersionId": "rv", "RequestId": "request-1"}})
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	_, err := api.PreviewRegistryRecord(context.Background(), "reg", "rec", nil)
	var cloud *Error
	if !errors.As(err, &cloud) || cloud.Code != "ClientError.MalformedResponse" {
		t.Fatalf("error=%#v", err)
	}
}

func registryResponseForAction(action string) map[string]any {
	response := map[string]any{"RequestId": "request-1"}
	switch action {
	case "CreateRegistry":
		response["RegistryId"], response["Registry"] = "reg-12345678", validRegistryResponse()
	case "DescribeRegistry", "UpdateRegistry":
		response["Registry"] = validRegistryResponse()
	case "DescribeRegistryList":
		response["RegistrySet"], response["TotalCount"] = []any{validRegistryResponse()}, 1
	case "CreateRegistryRecord":
		response["RecordId"], response["Record"], response["Version"] = "rec-12345678", validRegistryRecordResponse(), validRegistryVersionResponse()
	case "DescribeRegistryRecord":
		response["Record"], response["Version"], response["ResolvedBy"], response["ResolvedLabel"] = validRegistryRecordResponse(), validRegistryVersionResponse(), "LABEL", "stable"
	case "DescribeRegistryRecordList":
		response["RecordSet"], response["TotalCount"] = []any{validRegistryRecordResponse()}, 1
	case "UpdateRegistryRecord":
		response["Record"] = validRegistryRecordResponse()
	case "DescribeRegistryRecordVersionList":
		response["VersionSet"], response["TotalCount"] = []any{validRegistryVersionResponse()}, 1
	case "ApproveRegistryRecord", "RejectRegistryRecord", "CancelRegistryRecord":
		response["Version"] = validRegistryVersionResponse()
	case "PreviewRegistryRecord":
		response["PreviewResult"], response["ResolvedVersionId"] = `{"StatusCode":200,"Body":"{}","HasUpdate":false}`, "rv-12345678"
	case "SyncRegistryRecord":
		response["SyncStatus"], response["ResolvedVersionId"], response["LastSyncTime"] = "UNCHANGED", "rv-12345678", "2026-09-22T01:02:05Z"
	case "DescribeRegistryAuditLogList":
		response["AuditLogSet"], response["TotalCount"] = []any{map[string]any{"AuditLogId": "log-1", "RegistryId": "reg-12345678", "RecordId": "rec-12345678", "Actor": "10001", "Action": "record.create", "Details": `{"safe":true}`, "CreateTime": "2026-09-22T01:02:05Z"}}, 1
	}
	return response
}
func validRegistryResponse() map[string]any {
	return map[string]any{"RegistryId": "reg-12345678", "Name": "test", "Description": "", "ApprovalMode": "MANUAL", "Region": "ap-guangzhou", "Status": "FUTURE", "CreateTime": "2026-09-22T01:02:03Z", "UpdateTime": "2026-09-22T01:02:04Z", "RecordCount": 1, "Tags": []any{map[string]any{"Key": "k", "Value": "v"}}}
}
func validRegistryRecordResponse() map[string]any {
	return map[string]any{"RecordId": "rec-12345678", "RegistryId": "reg-12345678", "Name": "custom", "Description": "", "DescriptorType": "CUSTOM", "LifecycleStatus": "FUTURE", "AppId": 1, "CreatorUin": "10001", "CreatorSubAccountUin": "", "CreateTime": "2026-09-22T01:02:03Z", "UpdateTime": "2026-09-22T01:02:04Z", "VersionCount": 1, "LabelSet": []string{"stable"}}
}
func validRegistryVersionResponse() map[string]any {
	return map[string]any{"VersionId": "rv-12345678", "RecordId": "rec-12345678", "Revision": 1, "Status": "FUTURE", "ApprovalMode": "MANUAL", "CreateTime": "2026-09-22T01:02:03Z", "UpdateTime": "2026-09-22T01:02:04Z", "Descriptors": `{"kind":"custom"}`, "SourceType": "MANUAL", "SourceConfig": `{}`, "ApprovalActions": []any{}, "LabelSet": []string{"stable"}}
}
