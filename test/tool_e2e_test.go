package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestToolCursorAndPreCacheCloudJourney(t *testing.T) {
	environment := requireA1CloudEnvironment(t)
	client := newCloudClient(t, environment.cloudEnvironment)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	name := fmt.Sprintf("agr-sdk-e2e-%d-%d", time.Now().Unix(), os.Getpid())
	created, err := client.Tools().Create(ctx, agr.CreateToolOptions{
		Name:    name,
		Type:    agr.ToolCodeInterpreter,
		Network: agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkSandbox},
		Tags:    []agr.ToolTag{{Key: "agr-sdk-e2e", Value: "owned"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ToolID == "" {
		t.Fatal("Create Tool returned an empty ToolID")
	}
	defer deleteToolAndConfirm(t, client.Tools(), created.ToolID)

	info, err := client.Tools().WaitFor(ctx, created.ToolID, agr.ToolActive)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != created.ToolID || info.Name != name {
		t.Fatalf("created Tool = %+v", info)
	}
	page, err := client.Tools().List(ctx, agr.ToolListOptions{ToolIDs: []string{created.ToolID}, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != created.ToolID {
		t.Fatalf("Tool List = %+v, %v", page, err)
	}
	description := "AGR Go SDK real-cloud A1 validation"
	if _, err = client.Tools().Update(ctx, created.ToolID, agr.UpdateToolOptions{Description: &description}); err != nil {
		t.Fatal(err)
	}
	waitToolDescription(t, ctx, client.Tools(), created.ToolID, description)

	lifetime := 5 * time.Minute
	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool:     agr.ToolRef{ID: created.ToolID},
		Timeout:  &lifetime,
		Metadata: map[string]string{"agr-sdk-e2e": "tool-sandbox"},
	})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		t.Fatal(err)
	}
	defer deleteAndConfirm(t, client.Sandboxes(), sandbox)
	result, err := sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "printf tool-sandbox-ok"}, User: agr.Root})
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "tool-sandbox-ok" {
		t.Fatalf("Tool Sandbox command = %+v, %v", result, err)
	}
	cursor, err := client.Sandboxes().ListPage(ctx, agr.SandboxPageOptions{Filter: agr.SandboxFilter{InstanceIDs: []string{sandbox.ID()}, ToolID: created.ToolID}, MaxResults: 1, NeedTotalCount: true})
	if err != nil || len(cursor.Items) != 1 || cursor.Items[0].ID != sandbox.ID() || cursor.TotalCount == nil || *cursor.TotalCount < 1 {
		t.Fatalf("Sandbox cursor page = %+v, %v", cursor, err)
	}

	preCache, err := client.PreCacheImages().Create(ctx, agr.CreatePreCacheImageOptions{Image: environment.preCacheImage, RegistryType: environment.preCacheRegistryType})
	if err != nil {
		t.Fatal(err)
	}
	if preCache.Ref.Image == "" || preCache.Ref.Digest == "" {
		t.Fatalf("PreCache create = %+v", preCache)
	}
	waitPreCache(t, ctx, client.PreCacheImages(), preCache.Ref)
}

func waitToolDescription(t *testing.T, ctx context.Context, manager *agr.ToolManager, id, want string) {
	t.Helper()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		info, err := manager.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if info.Description == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for Tool update: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitPreCache(t *testing.T, ctx context.Context, manager *agr.PreCacheImageManager, ref agr.PreCacheImageRef) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		info, err := manager.Get(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		switch info.Status {
		case "Success", "SUCCESS":
			return
		case "Failed", "FAILED":
			t.Fatalf("pre-cache failed: status=%s message=%s request=%s", info.Status, info.Message, info.RequestID)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for pre-cache: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}
