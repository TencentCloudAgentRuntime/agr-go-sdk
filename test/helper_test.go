package e2e_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

type cloudEnvironment struct {
	region     string
	toolID     string
	codeToolID string
}

type a1CloudEnvironment struct {
	cloudEnvironment
	preCacheImage        string
	preCacheRegistryType agr.ImageRegistryType
}

func requireCloudEnvironment(t *testing.T) cloudEnvironment {
	t.Helper()
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real Cloud E2E requires AGS_E2E=1; use make test-e2e")
	}
	required := []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "AGS_E2E_REGION", "AGS_E2E_TOOL_ID", "AGS_E2E_CODE_TOOL_ID"}
	for _, name := range required {
		if os.Getenv(name) == "" {
			t.Fatalf("real Cloud E2E requires %s", name)
		}
	}
	return cloudEnvironment{region: os.Getenv("AGS_E2E_REGION"), toolID: os.Getenv("AGS_E2E_TOOL_ID"), codeToolID: os.Getenv("AGS_E2E_CODE_TOOL_ID")}
}

func requireA1CloudEnvironment(t *testing.T) a1CloudEnvironment {
	t.Helper()
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real Cloud E2E requires AGS_E2E=1")
	}
	for _, name := range []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "AGS_E2E_REGION"} {
		if os.Getenv(name) == "" {
			t.Fatalf("A0+A1 E2E requires %s", name)
		}
	}
	image := os.Getenv("AGS_E2E_PRECACHE_IMAGE")
	registry := agr.ImageRegistryType(os.Getenv("AGS_E2E_PRECACHE_REGISTRY_TYPE"))
	if image == "" || (registry != agr.ImageRegistryEnterprise && registry != agr.ImageRegistryPersonal) {
		t.Fatal("A0+A1 E2E requires AGS_E2E_PRECACHE_IMAGE and AGS_E2E_PRECACHE_REGISTRY_TYPE=enterprise|personal")
	}
	return a1CloudEnvironment{
		cloudEnvironment:     cloudEnvironment{region: os.Getenv("AGS_E2E_REGION")},
		preCacheImage:        image,
		preCacheRegistryType: registry,
	}
}

func newCloudClient(t *testing.T, environment cloudEnvironment) *agr.Client {
	t.Helper()
	client, err := agr.NewClient(agr.WithRegion(environment.region))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func deleteAndConfirm(t *testing.T, manager *agr.SandboxManager, sandbox *agr.Sandbox) {
	t.Helper()
	_ = sandbox.Close()
	deleteIDAndConfirm(t, manager, sandbox.ID())
}

func cleanupAcceptedCreate(t *testing.T, manager *agr.SandboxManager, err error) {
	t.Helper()
	var failure *agr.Error
	if errors.As(err, &failure) && failure.InstanceID != "" {
		deleteIDAndConfirm(t, manager, failure.InstanceID)
	}
}

func deleteIDAndConfirm(t *testing.T, manager *agr.SandboxManager, id string) {
	t.Helper()
	cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := manager.Delete(cleanup, id); err != nil {
		t.Errorf("delete sandbox: %v", err)
		return
	}
	deadline := time.NewTicker(2 * time.Second)
	defer deadline.Stop()
	for {
		info, err := manager.Get(cleanup, id)
		if err != nil {
			var failure *agr.Error
			if errors.As(err, &failure) && failure.Code == agr.NotFound {
				return
			}
			t.Errorf("confirm sandbox cleanup: %v", err)
			return
		}
		if info.State == agr.Stopped {
			return
		}
		select {
		case <-cleanup.Done():
			t.Errorf("confirm sandbox cleanup: %v", cleanup.Err())
			return
		case <-deadline.C:
		}
	}
}

func deleteToolAndConfirm(t *testing.T, manager *agr.ToolManager, id string) {
	t.Helper()
	cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := manager.Delete(cleanup, id); err != nil {
		t.Errorf("delete tool: %v", err)
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		_, err := manager.Get(cleanup, id)
		if err != nil {
			var failure *agr.Error
			if errors.As(err, &failure) && failure.Code == agr.NotFound {
				return
			}
			t.Errorf("confirm tool cleanup: %v", err)
			return
		}
		select {
		case <-cleanup.Done():
			t.Errorf("confirm tool cleanup: %v", cleanup.Err())
			return
		case <-ticker.C:
		}
	}
}
