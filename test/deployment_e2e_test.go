package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestDeploymentControlAndHTTPCloudJourney(t *testing.T) {
	environment := requireDeploymentEnvironment(t)
	client := newCloudClient(t, environment)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	zero, one := int64(0), int64(1)
	idle := int64(300)
	stop := agr.DeploymentIdleStop
	name := fmt.Sprintf("agr-sdk-e2e-%d-%d", time.Now().Unix(), os.Getpid())
	created, err := client.Deployments().Create(ctx, agr.CreateDeploymentOptions{
		Name:   name,
		ToolID: environment.toolID,
		Scaling: &agr.CreateDeploymentScaling{
			MinInstances:   &zero,
			MaxInstances:   &one,
			MaxConcurrency: &one,
		},
		Lifecycle: &agr.CreateDeploymentLifecycle{
			IdleTimeoutSeconds: &idle,
			IdleAction:         &stop,
		},
		Tags: []agr.DeploymentTag{{Key: "agr-sdk-e2e", Value: "deployment"}},
	})
	if err != nil {
		t.Fatalf("%v; cloud_code=%s chain=%s", err, cloudErrorCode(err), errorTypeChain(err))
	}
	if created.ID == "" {
		t.Fatal("Create Deployment returned an empty ID")
	}
	defer deleteDeploymentAndConfirm(t, client.Deployments(), created.ID)

	active, err := client.Deployments().WaitFor(ctx, created.ID, agr.DeploymentActive)
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != name || active.ToolID != environment.toolID {
		t.Fatalf("Deployment identity = %+v", active)
	}

	described, err := client.Deployments().Get(ctx, created.ID)
	if err != nil || described.ID != created.ID {
		t.Fatalf("Get Deployment = %+v, %v", described, err)
	}
	page, err := client.Deployments().List(ctx, agr.DeploymentListOptions{
		Limit: 1,
		Filters: []agr.DeploymentFilter{{
			Name:   "deployment-id",
			Values: []string{created.ID},
		}},
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != created.ID || page.TotalCount < 1 {
		t.Fatalf("List Deployment = %+v, %v", page, err)
	}

	tags := []agr.DeploymentTag{{Key: "agr-sdk-e2e", Value: "deployment-updated"}}
	updated, err := client.Deployments().Update(ctx, created.ID, agr.UpdateDeploymentOptions{Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != tags[0] {
		t.Fatalf("updated Deployment tags = %+v", updated.Tags)
	}

	connection, err := client.Deployments().Connect(ctx, created.ID, agr.DeploymentConnectOptions{Port: 49983})
	if err != nil {
		t.Fatal(err)
	}
	requestBody := bytes.NewBufferString(`{"path":"/","depth":1}`)
	response, err := connection.Do(ctx, agr.DeploymentRequest{
		Method: http.MethodPost,
		Path:   "/filesystem.Filesystem/ListDir",
		Header: http.Header{
			"Content-Type":             []string{"application/json"},
			"Connect-Protocol-Version": []string{"1"},
			"Authorization":            []string{"Basic dXNlcjo="},
		},
		Body: requestBody,
	})
	if err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		_ = connection.Close()
		t.Fatalf("read envd ListDir: read=%v close=%v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusOK {
		_ = connection.Close()
		t.Fatalf("envd ListDir status=%s body=%q", response.Status, body)
	}
	var payload struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Entries == nil {
		_ = connection.Close()
		t.Fatalf("envd ListDir payload=%q error=%v", body, err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	// Closing the local data-plane connection must not delete the remote Deployment.
	stillActive, err := client.Deployments().Get(ctx, created.ID)
	if err != nil || stillActive.State != agr.DeploymentActive {
		t.Fatalf("Deployment after connection Close = %+v, %v", stillActive, err)
	}
}
