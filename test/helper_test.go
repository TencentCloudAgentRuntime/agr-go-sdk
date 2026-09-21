package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

type cloudEnvironment struct {
	region     string
	toolID     string
	codeToolID string
}

func errorTypeChain(err error) string {
	var types []string
	for err != nil && len(types) < 8 {
		types = append(types, fmt.Sprintf("%T", err))
		err = errors.Unwrap(err)
	}
	return strings.Join(types, " -> ")
}

func cloudErrorCode(err error) string {
	var coded interface{ GetCode() string }
	if errors.As(err, &coded) {
		return coded.GetCode()
	}
	return ""
}

type a1CloudEnvironment struct {
	cloudEnvironment
	preCacheImage        string
	preCacheRegistryType agr.ImageRegistryType
}

type volumeCloudEnvironment struct {
	cloudEnvironment
	fileSystemID   string
	toolType       agr.ToolType
	toolRoleARN    string
	storageRoleARN string
}

func requireDeploymentEnvironment(t *testing.T) cloudEnvironment {
	t.Helper()
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real Deployment E2E requires AGS_E2E=1")
	}
	for _, name := range []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "AGS_E2E_REGION", "AGS_E2E_TOOL_ID"} {
		if os.Getenv(name) == "" {
			t.Fatalf("real Deployment E2E requires %s", name)
		}
	}
	if os.Getenv("AGS_E2E_REGION") != "ap-shanghai" {
		t.Fatalf("real Deployment E2E requires AGS_E2E_REGION=ap-shanghai, got %q", os.Getenv("AGS_E2E_REGION"))
	}
	return cloudEnvironment{region: os.Getenv("AGS_E2E_REGION"), toolID: os.Getenv("AGS_E2E_TOOL_ID")}
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

func requireVolumeCloudEnvironment(t *testing.T) volumeCloudEnvironment {
	t.Helper()
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real persistent Volume E2E requires AGS_E2E=1")
	}
	for _, name := range []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "AGR_VOLUME_CFS_FILESYSTEM_ID", "AGR_VOLUME_TOOL_TYPE"} {
		if os.Getenv(name) == "" {
			t.Fatalf("real persistent Volume E2E requires %s", name)
		}
	}
	return volumeCloudEnvironment{
		cloudEnvironment: cloudEnvironment{region: "ap-guangzhou"},
		fileSystemID:     os.Getenv("AGR_VOLUME_CFS_FILESYSTEM_ID"),
		toolType:         agr.ToolType(os.Getenv("AGR_VOLUME_TOOL_TYPE")),
		toolRoleARN:      os.Getenv("AGR_VOLUME_TOOL_ROLE_ARN"),
		storageRoleARN:   os.Getenv("AGR_VOLUME_STORAGE_ROLE_ARN"),
	}
}

func newCloudClient(t *testing.T, environment cloudEnvironment) *agr.Client {
	t.Helper()
	options := []agr.ClientOption{agr.WithRegion(environment.region)}
	if endpoint := os.Getenv("AGS_E2E_CONTROL_ENDPOINT"); endpoint != "" {
		options = append(options, agr.WithControlPlaneEndpoint(endpoint))
	}
	client, err := agr.NewClient(options...)
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

func deleteDeploymentAndConfirm(t *testing.T, manager *agr.DeploymentManager, id string) {
	t.Helper()
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := manager.Delete(cleanup, id); err != nil {
		t.Errorf("delete Deployment: %v", err)
		return
	}
	if err := manager.WaitDeleted(cleanup, id); err != nil {
		t.Errorf("confirm Deployment cleanup: %v", err)
	}
}
