package agr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fsconnect "github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/gen/filesystem/filesystemconnect"
)

func TestSandboxDomainKeepsPublicHostAndRuntimeConnectionTogether(t *testing.T) {
	fsPath, fsHandler := fsconnect.NewFilesystemHandler(filesystemFixture{})
	var controlHosts, runtimeHosts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, fsPath) {
			fsHandler.ServeHTTP(w, r)
			return
		}
		response := map[string]any{"RequestId": "domain-fixture"}
		instance := instanceJSON("RUNNING")
		instance["ExpiresAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		switch r.Header.Get("X-Tc-Action") {
		case "StartSandboxInstance":
			response["Instance"] = instance
		case "DescribeSandboxInstanceList":
			response["InstanceSet"] = []any{instance}
			response["TotalCount"] = 1
		case "AcquireSandboxInstanceToken":
			response["Token"] = "private-fixture-token"
		default:
			t.Errorf("unexpected Cloud action %q", r.Header.Get("X-Tc-Action"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	baseTransport := server.Client().Transport
	transport := roundTripFuncRoot(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Host, ".example.test") {
			runtimeHosts = append(runtimeHosts, r.URL.Host)
			if r.Header.Get("X-Access-Token") != "private-fixture-token" {
				t.Error("runtime request lacks its instance token")
			}
			clone := r.Clone(r.Context())
			clone.URL.Scheme = "http"
			clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
			return baseTransport.RoundTrip(clone)
		}
		controlHosts = append(controlHosts, r.URL.Host)
		return baseTransport.RoundTrip(r)
	})
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithSandboxDomain("sandbox.example.test"), WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	if client.cfg.dataPlaneDomain != "tencentags.com" {
		t.Fatal("Sandbox domain changed Deployment configuration")
	}
	created, err := client.Sandboxes().Create(context.Background(), CreateOptions{Tool: ToolRef{ID: "tool-id"}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Close()
	connected, err := client.Sandboxes().Connect(context.Background(), created.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer connected.Close()
	for _, sandbox := range []*Sandbox{created, connected} {
		host, err := sandbox.GetHost(8080)
		if err != nil || host != "8080-sb-test.sandbox.example.test" {
			t.Fatalf("GetHost = %q, %v", host, err)
		}
	}
	otherClient, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithSandboxDomain("other.example.test"), WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	otherConnection, err := otherClient.Sandboxes().Connect(context.Background(), created.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer otherConnection.Close()
	if host, err := otherConnection.GetHost(8080); err != nil || host != "8080-sb-test.other.example.test" {
		t.Fatalf("other Client GetHost = %q, %v", host, err)
	}
	if len(runtimeHosts) != 3 || len(controlHosts) < 3 {
		t.Fatalf("request counts: runtime=%v control=%v", runtimeHosts, controlHosts)
	}
	for _, host := range runtimeHosts[:2] {
		if host != "49983-sb-test.sandbox.example.test" {
			t.Fatalf("runtime host = %q", host)
		}
	}
	if runtimeHosts[2] != "49983-sb-test.other.example.test" {
		t.Fatalf("other Client runtime host = %q", runtimeHosts[2])
	}
	for _, host := range controlHosts {
		if host != strings.TrimPrefix(server.URL, "http://") {
			t.Fatalf("Cloud host = %q", host)
		}
	}
}

func TestSandboxDomainDefaultsAndClientIsolation(t *testing.T) {
	newClient := func(options ...ClientOption) *Client {
		t.Helper()
		client, err := NewClient(append([]ClientOption{WithRegion("ap-test")}, options...)...)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	for _, test := range []struct {
		client *Client
		want   string
	}{
		{newClient(), "8080-sb-id.ap-test.tencentags.com"},
		{newClient(WithSandboxDomain("one.example.test")), "8080-sb-id.one.example.test"},
		{newClient(WithSandboxDomain("two.example.test")), "8080-sb-id.two.example.test"},
	} {
		got, err := newSandbox(test.client, "sb-id", nil).GetHost(8080)
		if err != nil || got != test.want {
			t.Fatalf("GetHost = %q, %v; want %q", got, err, test.want)
		}
	}
	for _, domain := range []string{"", "https://example.test", "UPPER.example.test", "bad/path"} {
		_, err := NewClient(WithRegion("ap-test"), WithSandboxDomain(domain))
		var failure *Error
		if !errors.As(err, &failure) || failure.Code != InvalidArgument || failure.Reason != "SANDBOX_DOMAIN_INVALID" {
			t.Fatalf("domain %q: %v", domain, err)
		}
	}
}
