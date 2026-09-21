package agr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeploymentTypedRequestMappingAndLazyToken(t *testing.T) {
	bodies := map[string][]map[string]any{}
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		action := r.Header.Get("X-TC-Action")
		bodies[action] = append(bodies[action], body)
		response := map[string]any{"RequestId": "request-" + action}
		switch action {
		case "CreateDeployment", "DescribeDeployment", "ModifyDeployment":
			response["Deployment"] = deploymentJSON("dpl-a1b2c3d4", "ACTIVE")
		case "DescribeDeploymentList":
			response["DeploymentSet"] = []any{deploymentJSON("dpl-a1b2c3d4", "ACTIVE")}
			response["TotalCount"] = 1
		case "AcquireDeploymentToken":
			response["Token"] = "dpt_private"
			response["ExpiresAt"] = time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer cloud.Close()

	base := cloud.Client().Transport
	var dataCalls atomic.Int32
	router := roundTripFuncRoot(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "49983-dpl-a1b2c3d4.ap-test.agents.example.test" {
			dataCalls.Add(1)
			if request.Header.Get("X-Access-Token") != "dpt_private" || request.Header.Get("Authorization") != "Bearer business" {
				t.Errorf("data headers=%v", request.Header)
			}
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")), Request: request}, nil
		}
		return base.RoundTrip(request)
	})
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(cloud.URL), WithDataPlaneDomain("example.test"), WithHTTPClient(&http.Client{Transport: router}))
	if err != nil {
		t.Fatal(err)
	}
	min, max, concurrency := int64(0), int64(2), int64(10)
	idle, action := int64(30), DeploymentIdleStop
	created, err := client.Deployments().Create(context.Background(), CreateDeploymentOptions{
		Name: "sdk-deployment", ToolID: "sdt-a1b2c3d4",
		Scaling:   &CreateDeploymentScaling{MinInstances: &min, MaxInstances: &max, MaxConcurrency: &concurrency},
		Lifecycle: &CreateDeploymentLifecycle{IdleTimeoutSeconds: &idle, IdleAction: &action},
		Affinity:  &DeploymentAffinity{Mode: DeploymentAffinityBestEffort, HeaderName: "X-Workspace-Session"},
		Tags:      []DeploymentTag{{Key: "owner", Value: "sdk"}},
	})
	if err != nil || created.ID != "dpl-a1b2c3d4" || created.RequestID == "" {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	page, err := client.Deployments().List(context.Background(), DeploymentListOptions{Limit: 1, Filters: []DeploymentFilter{{Name: "tool-id", Values: []string{"sdt-a1b2c3d4"}}}})
	if err != nil || len(page.Items) != 1 || page.TotalCount != 1 || page.RequestID == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	emptyTags := []DeploymentTag{}
	updated, err := client.Deployments().Update(context.Background(), created.ID, UpdateDeploymentOptions{Scaling: &DeploymentScaling{MinInstances: 0, MaxInstances: 1, MaxConcurrency: 1}, Lifecycle: &DeploymentLifecycle{IdleTimeoutSeconds: 60, IdleAction: DeploymentIdlePause}, Tags: &emptyTags})
	if err != nil || updated.ID != created.ID {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	connection, err := client.Deployments().Connect(context.Background(), created.ID, DeploymentConnectOptions{Port: 49983})
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies["AcquireDeploymentToken"]) != 0 {
		t.Fatal("Connect acquired a token eagerly")
	}
	response, err := connection.Do(context.Background(), DeploymentRequest{Method: http.MethodGet, Path: "/health", Header: http.Header{"Authorization": []string{"Bearer business"}, "X-Access-Token": []string{"caller"}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	_ = connection.Close()
	if dataCalls.Load() != 1 || len(bodies["AcquireDeploymentToken"]) != 1 {
		t.Fatalf("data calls=%d token calls=%d", dataCalls.Load(), len(bodies["AcquireDeploymentToken"]))
	}
	if err = client.Deployments().Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	createBody := bodies["CreateDeployment"][0]
	if createBody["DeploymentName"] != "sdk-deployment" || createBody["ToolId"] != "sdt-a1b2c3d4" {
		t.Fatalf("create body=%#v", createBody)
	}
	listBody := bodies["DescribeDeploymentList"][0]
	if listBody["Offset"] != float64(0) || listBody["Limit"] != float64(1) {
		t.Fatalf("list body=%#v", listBody)
	}
	updateBody := bodies["ModifyDeployment"][0]
	if tags, ok := updateBody["Tags"].([]any); !ok || len(tags) != 0 {
		t.Fatalf("explicit empty tags lost: %#v", updateBody)
	}
}

func TestDeploymentValidationMakesNoNetworkCallsAndCopiesInputs(t *testing.T) {
	var calls atomic.Int32
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: roundTripFuncRoot(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network")
	})}))
	if err != nil {
		t.Fatal(err)
	}
	badMin, badMax := int64(2), int64(1)
	if _, err = client.Deployments().Create(context.Background(), CreateDeploymentOptions{Name: "Bad_Name", Scaling: &CreateDeploymentScaling{MinInstances: &badMin, MaxInstances: &badMax}}); err == nil {
		t.Fatal("accepted invalid create")
	}
	if _, err = client.Deployments().List(context.Background(), DeploymentListOptions{Limit: 201}); err == nil {
		t.Fatal("accepted invalid pagination")
	}
	if _, err = client.Deployments().Update(context.Background(), "dpl", UpdateDeploymentOptions{}); err == nil {
		t.Fatal("accepted empty update")
	}
	if _, err = client.Deployments().Connect(context.Background(), "dpl", DeploymentConnectOptions{}); err == nil {
		t.Fatal("accepted invalid port")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid inputs made %d calls", calls.Load())
	}

	min := int64(0)
	tags := []DeploymentTag{{Key: "owner", Value: "before"}}
	copy := copyCreateDeploymentOptions(CreateDeploymentOptions{Scaling: &CreateDeploymentScaling{MinInstances: &min}, Tags: tags})
	min = 9
	tags[0].Value = "after"
	if *copy.Scaling.MinInstances != 0 || copy.Tags[0].Value != "before" {
		t.Fatal("create options retained caller memory")
	}
}

func TestDeploymentWaitDeletedUsesExactNotFoundAndDeleteFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		response := map[string]any{"RequestId": "r"}
		if call == 1 {
			response["Deployment"] = deploymentJSON("dpl-delete", "DELETING")
		} else {
			response["Error"] = map[string]any{"Code": "ResourceNotFound.Deployment", "Message": "not found"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Deployments().WaitDeleted(ctx, "dpl-delete"); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentPreservesUnknownStateAndRejectsDeleteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := "FUTURE_STATE"
		if r.URL.Query().Get("failure") == "1" {
			state = "DELETE_FAILED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{
			"Deployment": deploymentJSON("dpl-state", state),
			"RequestId":  "request-state",
		}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	info, err := client.Deployments().Get(context.Background(), "dpl-state")
	if err != nil || info.State != DeploymentState("FUTURE_STATE") {
		t.Fatalf("unknown state=%q err=%v", info.State, err)
	}

	failedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{
			"Deployment": deploymentJSON("dpl-state", "DELETE_FAILED"),
			"RequestId":  "request-failed",
		}})
	}))
	defer failedServer.Close()
	failedClient, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(failedServer.URL), WithHTTPClient(failedServer.Client()))
	err = failedClient.Deployments().WaitDeleted(context.Background(), "dpl-state")
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != Conflict || failure.ResourceID != "dpl-state" {
		t.Fatalf("delete failure=%#v", err)
	}
}

func TestDeploymentConnectRejectsInitialAffinityWhenDisabled(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("X-TC-Action")
		if action == "AcquireDeploymentToken" {
			tokenCalls.Add(1)
		}
		deployment := deploymentJSON("dpl-no-affinity", "ACTIVE")
		delete(deployment, "AffinityConfiguration")
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"Deployment": deployment, "RequestId": "request"}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	_, err := client.Deployments().Connect(context.Background(), "dpl-no-affinity", DeploymentConnectOptions{Port: 8080, InitialAffinityID: "session-private"})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != InvalidArgument || tokenCalls.Load() != 0 {
		t.Fatalf("Connect error=%#v tokenCalls=%d", err, tokenCalls.Load())
	}
}

func TestDeploymentMalformedResponseAndDataPlaneDomainValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"RequestId": "r"}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	_, err := client.Deployments().Create(context.Background(), CreateDeploymentOptions{Name: "valid-name"})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != Protocol {
		t.Fatalf("error=%#v", err)
	}
	for _, value := range []string{"", "https://example.test", "example.test/path", "example.test:443", "-bad.example"} {
		if _, err = NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithDataPlaneDomain(value)); err == nil {
			t.Fatalf("accepted domain %q", value)
		}
	}
}

func TestDeploymentOptionsAndErrorsAreRedacted(t *testing.T) {
	request := DeploymentRequest{Header: http.Header{"Authorization": []string{"Bearer secret"}}}
	if strings.Contains(request.String(), "secret") || strings.Contains(request.GoString(), "secret") {
		t.Fatal("request formatting leaked header")
	}
	opts := DeploymentConnectOptions{InitialAffinityID: "private-session"}
	if strings.Contains(opts.String(), "private-session") || strings.Contains(opts.GoString(), "private-session") {
		t.Fatal("connect options leaked affinity id")
	}
}

func deploymentJSON(id, state string) map[string]any {
	return map[string]any{
		"DeploymentId": id, "DeploymentName": "sdk-deployment", "ToolId": "sdt-a1b2c3d4", "Status": state,
		"ScalingConfiguration":   map[string]any{"MinInstanceCount": 0, "MaxInstanceCount": 2, "MaxInstanceRequestConcurrency": 10},
		"LifecycleConfiguration": map[string]any{"IdleTimeoutSeconds": 30, "IdleAction": "STOP"},
		"AffinityConfiguration":  map[string]any{"Mode": "BEST_EFFORT", "HeaderName": "X-Workspace-Session"},
		"Tags":                   []any{map[string]any{"Key": "owner", "Value": "sdk"}},
		"CreatedTime":            "2026-09-21T00:00:00Z", "UpdatedTime": "2026-09-21T00:01:00Z",
	}
}
