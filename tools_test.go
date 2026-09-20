package agr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolAndPreCacheTypedRequestMapping(t *testing.T) {
	bodies := map[string][]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		action := r.Header.Get("X-TC-Action")
		bodies[action] = append(bodies[action], body)
		response := map[string]any{"RequestId": "request-" + action}
		switch action {
		case "CreateSandboxTool":
			response["ToolId"] = "sdt-created"
		case "DescribeSandboxToolList":
			response["TotalCount"] = 1
			response["SandboxToolSet"] = []any{toolJSON("sdt-created", "ACTIVE")}
		case "CreatePreCacheImageTask":
			response["Image"] = "repo/image:tag"
			response["ImageDigest"] = "sha256:abc"
			response["ImageRegistryType"] = "enterprise"
		case "DescribePreCacheImageTask":
			response["Image"] = "repo/image:tag"
			response["ImageDigest"] = "sha256:abc"
			response["ImageRegistryType"] = "enterprise"
			response["Status"] = "Success"
			response["Message"] = "ready"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	persistent := false
	created, err := client.Tools().Create(context.Background(), CreateToolOptions{Name: "sdk-tool", Type: ToolCustom, Network: ToolNetworkConfiguration{Mode: ToolNetworkSandbox}, Description: "sdk", DefaultTimeout: "5m", ClientToken: "token", Tags: []ToolTag{{Key: "owner", Value: "sdk"}}, CustomConfiguration: &ToolCustomConfiguration{Image: "repo/image:tag", ImageRegistryType: ImageRegistryEnterprise, Command: []string{"/bin/sh"}}, Persistent: &persistent})
	if err != nil {
		t.Fatal(err)
	}
	if created.ToolID != "sdt-created" || created.RequestID == "" {
		t.Fatalf("create = %+v", created)
	}
	info, err := client.Tools().Get(context.Background(), created.ToolID)
	if err != nil || info.ID != "sdt-created" || info.State != ToolActive || info.CustomConfiguration.ImageDigest != "sha256:abc" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	description := ""
	tags := []ToolTag{{Key: "owner", Value: "updated"}}
	updated, err := client.Tools().Update(context.Background(), created.ToolID, UpdateToolOptions{Description: &description, Tags: &tags})
	if err != nil || updated.ToolID != "sdt-created" {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if err = client.Tools().Delete(context.Background(), created.ToolID); err != nil {
		t.Fatal(err)
	}
	pre, err := client.PreCacheImages().Create(context.Background(), CreatePreCacheImageOptions{Image: "repo/image:tag", RegistryType: ImageRegistryEnterprise})
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.PreCacheImages().Get(context.Background(), pre.Ref)
	if err != nil || status.Status != "Success" || status.Ref.Digest != "sha256:abc" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	createBody := bodies["CreateSandboxTool"][0]
	if createBody["ToolName"] != "sdk-tool" || createBody["ToolType"] != "custom" || createBody["Persistent"] != false {
		t.Fatalf("create body=%#v", createBody)
	}
	updateBody := bodies["UpdateSandboxTool"][0]
	if value, ok := updateBody["Description"]; !ok || value != "" {
		t.Fatalf("update did not preserve explicit empty description: %#v", updateBody)
	}
	preGet := bodies["DescribePreCacheImageTask"][0]
	if preGet["Image"] != "repo/image:tag" || preGet["ImageDigest"] != "sha256:abc" || preGet["ImageRegistryType"] != "enterprise" {
		t.Fatalf("pre-cache get=%#v", preGet)
	}
	if len(bodies["CreateSandboxTool"]) != 1 || len(bodies["UpdateSandboxTool"]) != 1 || len(bodies["DescribeSandboxToolList"]) != 1 {
		t.Fatalf("unexpected implicit tool request sequence: %#v", bodies)
	}
}

func TestToolManagersKeepClientIdentityAndTransportIsolated(t *testing.T) {
	type endpoint struct {
		server *httptest.Server
		calls  *atomic.Int32
	}
	newEndpoint := func(credentialID, toolID string) endpoint {
		calls := &atomic.Int32{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if got := r.Header.Get("Authorization"); !strings.Contains(got, "Credential="+credentialID+"/") {
				t.Errorf("endpoint %s received wrong identity: %q", toolID, got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{
				"SandboxToolSet": []any{toolJSON(toolID, "ACTIVE")},
				"TotalCount":     1,
				"RequestId":      "request-" + toolID,
			}})
		}))
		return endpoint{server: server, calls: calls}
	}

	first := newEndpoint("identity-a", "sdt-a")
	defer first.server.Close()
	second := newEndpoint("identity-b", "sdt-b")
	defer second.server.Close()
	clientA, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"identity-a", "key-a"}), WithControlPlaneEndpoint(first.server.URL), WithHTTPClient(first.server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	clientB, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"identity-b", "key-b"}), WithControlPlaneEndpoint(second.server.URL), WithHTTPClient(second.server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	pageA, err := clientA.Tools().List(context.Background(), ToolListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	pageB, err := clientB.Tools().List(context.Background(), ToolListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(pageA.Items) != 1 || pageA.Items[0].ID != "sdt-a" || len(pageB.Items) != 1 || pageB.Items[0].ID != "sdt-b" {
		t.Fatalf("cross-client tool result: A=%+v B=%+v", pageA.Items, pageB.Items)
	}
	if first.calls.Load() != 1 || second.calls.Load() != 1 {
		t.Fatalf("cross-client transport calls: first=%d second=%d", first.calls.Load(), second.calls.Load())
	}
}

func TestToolGetRequiresExactSingleMatch(t *testing.T) {
	for _, test := range []struct {
		name  string
		items []any
	}{{"zero", nil}, {"multiple", []any{toolJSON("sdt-want", "ACTIVE"), toolJSON("sdt-other", "ACTIVE")}}, {"mismatch", []any{toolJSON("sdt-other", "ACTIVE")}}} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"SandboxToolSet": test.items, "TotalCount": len(test.items), "RequestId": "r"}})
			}))
			defer server.Close()
			client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
			_, err := client.Tools().Get(context.Background(), "sdt-want")
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != NotFound || failure.ResourceType != "tool" || failure.ResourceID != "sdt-want" {
				t.Fatalf("error=%#v", err)
			}
		})
	}
}

func TestToolDeleteNotFoundIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"RequestId": "r", "Error": map[string]any{"Code": "ResourceNotFound.Tool", "Message": "not found"}}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err := client.Tools().Delete(context.Background(), "sdt-missing"); err != nil {
		t.Fatal(err)
	}
}

func TestToolInvalidInputsMakeNoNetworkCallsAndInputsAreCopied(t *testing.T) {
	var calls atomic.Int32
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: roundTripFuncRoot(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("network") })}))
	if _, err := client.PreCacheImages().Create(context.Background(), CreatePreCacheImageOptions{Image: "x", RegistryType: "custom"}); err == nil {
		t.Fatal("accepted unverified custom registry")
	}
	empty := []ToolTag{}
	if _, err := client.Tools().Update(context.Background(), "sdt", UpdateToolOptions{Tags: &empty}); err == nil {
		t.Fatal("accepted unrepresentable empty tag replacement")
	}
	if _, err := client.Tools().Create(context.Background(), CreateToolOptions{Name: "bad name", Type: ToolCustom, Network: ToolNetworkConfiguration{Mode: ToolNetworkSandbox}}); err == nil {
		t.Fatal("accepted invalid name")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid inputs made %d calls", calls.Load())
	}
	vpc := &ToolVPCConfiguration{SubnetIDs: []string{"before"}}
	tags := []ToolTag{{Key: "k", Value: "before"}}
	command := []string{"before"}
	persistent := false
	copy := copyCreateToolOptions(CreateToolOptions{Network: ToolNetworkConfiguration{Mode: ToolNetworkVPC, VPC: vpc}, Tags: tags, CustomConfiguration: &ToolCustomConfiguration{Command: command}, Persistent: &persistent})
	vpc.SubnetIDs[0] = "after"
	tags[0].Value = "after"
	command[0] = "after"
	persistent = true
	if copy.Network.VPC.SubnetIDs[0] != "before" || copy.Tags[0].Value != "before" || copy.CustomConfiguration.Command[0] != "before" || *copy.Persistent {
		t.Fatal("tool options retain caller memory")
	}
}

func TestToolWaitForCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"SandboxToolSet": []any{toolJSON("sdt-wait", "CREATING")}, "TotalCount": 1, "RequestId": "r"}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.Tools().WaitFor(ctx, "sdt-wait", ToolActive)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != DeadlineExceeded {
		t.Fatalf("error=%#v", err)
	}
}

func TestToolErrorsAndOptionsDoNotLeakSensitiveValues(t *testing.T) {
	opts := CreateToolOptions{CustomConfiguration: &ToolCustomConfiguration{Env: []ToolEnvironmentVariable{{Name: "TOKEN", Value: "secret-value"}}}}
	if strings.Contains(opts.String(), "secret-value") || strings.Contains(opts.GoString(), "secret-value") {
		t.Fatal("tool options leaked env")
	}
	err := withResourceError(codeError(Conflict, "Tools.Update", "RESOURCE_CONFLICT"), "tool", "sdt-private")
	if strings.Contains(err.Error(), "sdt-private") {
		t.Fatal("error string leaked resource id")
	}
}

func toolJSON(id, state string) map[string]any {
	return map[string]any{"ToolId": id, "ToolName": "sdk-tool", "ToolType": "custom", "Status": state, "Description": "sdk", "Persistent": false, "DefaultTimeoutSeconds": 300, "NetworkConfiguration": map[string]any{"NetworkMode": "SANDBOX"}, "Tags": []any{map[string]any{"Key": "owner", "Value": "sdk"}}, "CreateTime": "2026-09-20T00:00:00Z", "UpdateTime": "2026-09-20T00:01:00Z", "CustomConfiguration": map[string]any{"Image": "repo/image:tag", "ImageDigest": "sha256:abc", "ImageRegistryType": "enterprise"}}
}
