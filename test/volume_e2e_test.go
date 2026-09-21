package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestPersistentVolumeCloudJourney(t *testing.T) {
	environment := requireVolumeCloudEnvironment(t)
	client := newCloudClient(t, environment.cloudEnvironment)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	beforeVolumes := volumeIDs(t, ctx, client.Volumes())
	beforeTemplates := volumeTemplateIDs(t, ctx, client.VolumeTemplates())
	owned := newVolumeE2EOwned(client)
	defer owned.cleanup(t)

	suffix := fmt.Sprintf("%d-%d", time.Now().Unix(), os.Getpid())
	path := "/agr-sdk-e2e/" + suffix
	standalone := createE2EVolume(t, ctx, client, environment, "agr-sdk-e2e-volume-"+suffix, path+"/direct")
	owned.volumes[standalone.ID] = true
	shared := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-shared-"+suffix, agr.VolumeReclaimRetain, path+"/shared/${reuse_key}")
	owned.templates[shared.ID] = true
	retained := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-retain-"+suffix, agr.VolumeReclaimRetain, path+"/retain/${reuse_key}")
	owned.templates[retained.ID] = true
	deleted := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-delete-"+suffix, agr.VolumeReclaimDelete, path+"/delete/${reuse_key}")
	owned.templates[deleted.ID] = true

	readOnly := true
	createdTool, err := client.Tools().Create(ctx, agr.CreateToolOptions{
		Name:    "agr-sdk-e2e-volume-tool-" + suffix,
		Type:    environment.toolType,
		Network: agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkPublic},
		RoleARN: environment.toolRoleARN,
		Tags:    []agr.ToolTag{{Key: "agr-sdk-e2e", Value: suffix}},
		StorageMounts: []agr.ToolStorageMount{
			{Name: "by-name", MountPath: "/mnt/by-name", Volume: &agr.ToolVolumeRef{Name: standalone.Name}},
			{Name: "by-id", MountPath: "/mnt/by-id", ReadOnly: &readOnly, Volume: &agr.ToolVolumeRef{ID: standalone.ID}},
			{Name: "shared", MountPath: "/mnt/shared", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: shared.Name, ReuseKey: "${metadata.session_id}"}},
			{Name: "retain", MountPath: "/mnt/retain", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: retained.Name, ReuseKey: "${metadata.session_id}"}},
			{Name: "delete", MountPath: "/mnt/delete", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: deleted.Name, ReuseKey: "${metadata.session_id}"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	owned.toolID = createdTool.ToolID
	tool, err := client.Tools().WaitFor(ctx, createdTool.ToolID, agr.ToolActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(tool.StorageMounts) != 5 || tool.StorageMounts[2].VolumeTemplate == nil || tool.StorageMounts[2].VolumeTemplate.ReuseKey != "${metadata.session_id}" {
		t.Fatalf("Tool Preview mounts = %+v", tool.StorageMounts)
	}

	keyA, keyB := "session-a-"+suffix, "session-b-"+suffix
	sandboxA := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyA)
	owned.sandboxes = append(owned.sandboxes, sandboxA)
	runVolumeCommand(t, ctx, sandboxA, "set -eu; printf direct > /mnt/by-name/probe; test \"$(cat /mnt/by-id/probe)\" = direct; if touch /mnt/by-id/forbidden 2>/dev/null; then exit 90; fi; printf shared-a > /mnt/shared/probe; printf retained > /mnt/retain/probe; printf deleted > /mnt/delete/probe")
	if err := client.Volumes().Delete(ctx, standalone.ID); err == nil {
		t.Fatal("deleting a mounted Volume unexpectedly succeeded")
	}

	sandboxSameKey := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyA)
	owned.sandboxes = append(owned.sandboxes, sandboxSameKey)
	runVolumeCommand(t, ctx, sandboxSameKey, "set -eu; test \"$(cat /mnt/shared/probe)\" = shared-a")
	sandboxOtherKey := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyB)
	owned.sandboxes = append(owned.sandboxes, sandboxOtherKey)
	runVolumeCommand(t, ctx, sandboxOtherKey, "set -eu; test ! -e /mnt/shared/probe; printf shared-b > /mnt/shared/probe")

	sharedA := waitDerivedVolume(t, ctx, client.Volumes(), shared.ID, keyA)
	sharedB := waitDerivedVolume(t, ctx, client.Volumes(), shared.ID, keyB)
	if sharedA.ID == sharedB.ID {
		t.Fatalf("different reuse keys resolved to one Volume: %s", sharedA.ID)
	}
	owned.volumes[sharedA.ID], owned.volumes[sharedB.ID] = true, true
	deleteB := waitDerivedVolume(t, ctx, client.Volumes(), deleted.ID, keyB)
	retainB := waitDerivedVolume(t, ctx, client.Volumes(), retained.ID, keyB)
	owned.volumes[deleteB.ID], owned.volumes[retainB.ID] = true, true

	deleteOneVolumeSandbox(t, client.Sandboxes(), sandboxOtherKey)
	owned.sandboxes = owned.sandboxes[:2]
	waitVolumeAbsent(t, ctx, client.Volumes(), deleteB.ID)
	delete(owned.volumes, deleteB.ID)
	if got, err := client.Volumes().Get(ctx, retainB.ID); err != nil || got.ID != retainB.ID {
		t.Fatalf("Retain-derived Volume after Sandbox deletion = %+v, %v", got, err)
	}

	owned.cleanup(t)
	afterVolumes := volumeIDs(t, ctx, client.Volumes())
	afterTemplates := volumeTemplateIDs(t, ctx, client.VolumeTemplates())
	if !sameStringSet(beforeVolumes, afterVolumes) || !sameStringSet(beforeTemplates, afterTemplates) {
		t.Fatalf("persistent Volume E2E left resources: volumes before=%v after=%v templates before=%v after=%v", beforeVolumes, afterVolumes, beforeTemplates, afterTemplates)
	}
}

type volumeE2EOwned struct {
	client    *agr.Client
	sandboxes []*agr.Sandbox
	toolID    string
	volumes   map[string]bool
	templates map[string]bool
}

func newVolumeE2EOwned(client *agr.Client) *volumeE2EOwned {
	return &volumeE2EOwned{client: client, volumes: map[string]bool{}, templates: map[string]bool{}}
}

func (o *volumeE2EOwned) cleanup(t *testing.T) {
	t.Helper()
	cleanup, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	for index := len(o.sandboxes) - 1; index >= 0; index-- {
		deleteOneVolumeSandbox(t, o.client.Sandboxes(), o.sandboxes[index])
	}
	o.sandboxes = nil
	if o.toolID != "" {
		deleteToolAndConfirm(t, o.client.Tools(), o.toolID)
		o.toolID = ""
	}
	// Retain templates can create Volumes after the first observation. Discover
	// every derived identity owned by this test before deleting any template.
	for templateID := range o.templates {
		page, err := o.client.Volumes().List(cleanup, agr.VolumeListOptions{Filters: []agr.VolumeFilter{{Name: "volume-template-id", Values: []string{templateID}}}, Limit: 100})
		if err != nil {
			t.Errorf("list derived Volumes for cleanup: %v", err)
			continue
		}
		for _, volume := range page.Items {
			o.volumes[volume.ID] = true
		}
	}
	for id := range o.volumes {
		if err := o.client.Volumes().Delete(cleanup, id); err != nil && !isNotFound(err) {
			t.Errorf("delete Volume: %v", err)
			continue
		}
		waitVolumeAbsent(t, cleanup, o.client.Volumes(), id)
		delete(o.volumes, id)
	}
	for id := range o.templates {
		if err := o.client.VolumeTemplates().Delete(cleanup, id); err != nil && !isNotFound(err) {
			t.Errorf("delete VolumeTemplate: %v", err)
			continue
		}
		waitVolumeTemplateAbsent(t, cleanup, o.client.VolumeTemplates(), id)
		delete(o.templates, id)
	}
}

func createE2EVolume(t *testing.T, ctx context.Context, client *agr.Client, environment volumeCloudEnvironment, name, path string) agr.VolumeInfo {
	t.Helper()
	created, err := client.Volumes().Create(ctx, agr.CreateVolumeOptions{Name: name, AccessMode: agr.VolumeReadWriteMany, StorageType: agr.VolumeStorageCFS, Storage: agr.VolumeStorage{CFS: &agr.VolumeCFSStorage{FileSystemID: environment.fileSystemID, Path: path}}, StorageRoleARN: environment.storageRoleARN, Tags: []agr.VolumeTag{{Key: "agr-sdk-e2e", Value: name}}})
	if err != nil {
		t.Fatal(err)
	}
	return waitVolumeActive(t, ctx, client.Volumes(), created.ID)
}

func createE2ETemplate(t *testing.T, ctx context.Context, client *agr.Client, environment volumeCloudEnvironment, name string, reclaim agr.VolumeReclaimPolicy, pattern string) agr.VolumeTemplateInfo {
	t.Helper()
	created, err := client.VolumeTemplates().Create(ctx, agr.CreateVolumeTemplateOptions{Name: name, AccessMode: agr.VolumeReadWriteMany, ReclaimPolicy: reclaim, StorageType: agr.VolumeStorageCFS, Storage: agr.VolumeTemplateStorage{CFS: &agr.VolumeTemplateCFSStorage{FileSystemID: environment.fileSystemID, PathPattern: pattern}}, VolumeNamePattern: name + "-${reuse_key}", StorageRoleARN: environment.storageRoleARN, Tags: []agr.VolumeTag{{Key: "agr-sdk-e2e", Value: name}}})
	if err != nil {
		t.Fatal(err)
	}
	return waitTemplateActive(t, ctx, client.VolumeTemplates(), created.ID)
}

func createE2EVolumeSandbox(t *testing.T, ctx context.Context, client *agr.Client, toolID, key string) *agr.Sandbox {
	t.Helper()
	lifetime := 5 * time.Minute
	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{Tool: agr.ToolRef{ID: toolID}, Timeout: &lifetime, Metadata: map[string]string{"session_id": key, "agr-sdk-e2e": "persistent-volume"}})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		t.Fatal(err)
	}
	return sandbox
}

func runVolumeCommand(t *testing.T, ctx context.Context, sandbox *agr.Sandbox, script string) {
	t.Helper()
	result, err := sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", script}, User: agr.Root})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("persistent Volume command exit=%d err=%v stderr=%s", result.ExitCode, err, result.Stderr)
	}
}

func deleteOneVolumeSandbox(t *testing.T, manager *agr.SandboxManager, sandbox *agr.Sandbox) {
	t.Helper()
	deleteAndConfirm(t, manager, sandbox)
}

func waitVolumeActive(t *testing.T, ctx context.Context, manager *agr.VolumeManager, id string) agr.VolumeInfo {
	t.Helper()
	for {
		value, err := manager.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if value.State == agr.VolumeActive {
			return value
		}
		if value.State == agr.VolumeFailed {
			t.Fatalf("Volume %s entered %s", id, value.State)
		}
		waitVolumePoll(t, ctx)
	}
}

func waitTemplateActive(t *testing.T, ctx context.Context, manager *agr.VolumeTemplateManager, id string) agr.VolumeTemplateInfo {
	t.Helper()
	for {
		value, err := manager.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if value.State == agr.VolumeActive {
			return value
		}
		if value.State == agr.VolumeFailed {
			t.Fatalf("VolumeTemplate %s entered %s", id, value.State)
		}
		waitVolumePoll(t, ctx)
	}
}

func waitDerivedVolume(t *testing.T, ctx context.Context, manager *agr.VolumeManager, templateID, reuseKey string) agr.VolumeInfo {
	t.Helper()
	for {
		page, err := manager.List(ctx, agr.VolumeListOptions{Filters: []agr.VolumeFilter{{Name: "volume-template-id", Values: []string{templateID}}}, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range page.Items {
			if value.TemplateID == templateID && value.ReuseKey == reuseKey && value.State == agr.VolumeActive {
				return value
			}
		}
		waitVolumePoll(t, ctx)
	}
}

func waitVolumeAbsent(t *testing.T, ctx context.Context, manager *agr.VolumeManager, id string) {
	t.Helper()
	for {
		_, err := manager.Get(ctx, id)
		if isNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("confirm Volume cleanup: %v", err)
			return
		}
		waitVolumePoll(t, ctx)
	}
}

func waitVolumeTemplateAbsent(t *testing.T, ctx context.Context, manager *agr.VolumeTemplateManager, id string) {
	t.Helper()
	for {
		_, err := manager.Get(ctx, id)
		if isNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("confirm VolumeTemplate cleanup: %v", err)
			return
		}
		waitVolumePoll(t, ctx)
	}
}

func waitVolumePoll(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(2 * time.Second):
	}
}

func volumeIDs(t *testing.T, ctx context.Context, manager *agr.VolumeManager) []string {
	t.Helper()
	page, err := manager.List(ctx, agr.VolumeListOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, value := range page.Items {
		ids = append(ids, value.ID)
	}
	return ids
}

func volumeTemplateIDs(t *testing.T, ctx context.Context, manager *agr.VolumeTemplateManager) []string {
	t.Helper()
	page, err := manager.List(ctx, agr.VolumeTemplateListOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, value := range page.Items {
		ids = append(ids, value.ID)
	}
	return ids
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]int, len(left))
	for _, value := range left {
		values[value]++
	}
	for _, value := range right {
		values[value]--
	}
	for _, count := range values {
		if count != 0 {
			return false
		}
	}
	return true
}

func isNotFound(err error) bool {
	var failure *agr.Error
	return errors.As(err, &failure) && failure.Code == agr.NotFound
}
