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
	toolFixture := resolveVolumeToolFixture(t, ctx, client, environment)
	if environment.storageRoleARN == "" {
		environment.storageRoleARN = toolFixture.roleARN
	}

	beforeVolumes := volumeIDs(t, ctx, client.Volumes())
	beforeTemplates := volumeTemplateIDs(t, ctx, client.VolumeTemplates())
	owned := newVolumeE2EOwned(client)
	defer owned.cleanup(t)

	suffix := fmt.Sprintf("%d-%d", time.Now().Unix(), os.Getpid())
	path := "agr-sdk-e2e/" + suffix
	keyA, keyB := "session-a-"+suffix, "session-b-"+suffix
	bootstrapTool, err := client.Tools().Create(ctx, toolFixture.createOptions(
		"agr-sdk-e2e-volume-bootstrap-"+suffix,
		[]agr.ToolTag{{Key: "agr-sdk-e2e", Value: suffix}},
		[]agr.ToolStorageMount{environment.bootstrapMount(path)},
	))
	if err != nil {
		t.Fatal(err)
	}
	owned.toolIDs = append(owned.toolIDs, bootstrapTool.ToolID)
	if _, err = client.Tools().WaitFor(ctx, bootstrapTool.ToolID, agr.ToolActive); err != nil {
		t.Fatal(err)
	}
	owned.bootstrap = createE2EVolumeSandbox(t, ctx, client, bootstrapTool.ToolID, "bootstrap-"+suffix)
	fixturePath := environment.fixtureMountPath(path)
	owned.bootstrapCleanupCommand = environment.fixtureCleanupCommand(path)
	initialize := fmt.Sprintf(
		"mkdir -p -- %[1]s/direct %[1]s/shared/%[2]s %[1]s/shared/%[3]s %[1]s/retain/%[2]s %[1]s/retain/%[3]s %[1]s/delete/%[2]s %[1]s/delete/%[3]s",
		fixturePath, keyA, keyB,
	)
	if environment.storageType == agr.VolumeStorageCFS {
		initialize += "; chmod -R 0777 -- " + fixturePath
	}
	runVolumeCommand(t, ctx, owned.bootstrap, initialize)
	standalone := createE2EVolume(t, ctx, client, environment, "agr-sdk-e2e-volume-"+suffix, path+"/direct")
	owned.volumes[standalone.ID] = true
	shared := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-shared-"+suffix, agr.VolumeReclaimRetain, path+"/shared/${reuse_key}")
	owned.templates[shared.ID] = true
	retained := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-retain-"+suffix, agr.VolumeReclaimRetain, path+"/retain/${reuse_key}")
	owned.templates[retained.ID] = true
	deleted := createE2ETemplate(t, ctx, client, environment, "agr-sdk-e2e-delete-"+suffix, agr.VolumeReclaimDelete, path+"/delete/${reuse_key}")
	owned.templates[deleted.ID] = true

	readOnly := true
	createdTool, err := client.Tools().Create(ctx, toolFixture.createOptions(
		"agr-sdk-e2e-volume-tool-"+suffix,
		[]agr.ToolTag{{Key: "agr-sdk-e2e", Value: suffix}},
		[]agr.ToolStorageMount{
			{Name: "by-name", MountPath: "/mnt/by-name", Volume: &agr.ToolVolumeRef{Name: standalone.Name}},
			{Name: "by-id", MountPath: "/mnt/by-id", ReadOnly: &readOnly, Volume: &agr.ToolVolumeRef{ID: standalone.ID}},
			{Name: "shared", MountPath: "/mnt/shared", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: shared.Name, ReuseKey: "${metadata.session_id}"}},
			{Name: "retain", MountPath: "/mnt/retain", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: retained.Name, ReuseKey: "${metadata.session_id}"}},
			{Name: "delete", MountPath: "/mnt/delete", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: deleted.Name, ReuseKey: "${metadata.session_id}"}},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	owned.toolIDs = append(owned.toolIDs, createdTool.ToolID)
	tool, err := client.Tools().WaitFor(ctx, createdTool.ToolID, agr.ToolActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(tool.StorageMounts) != 5 || tool.StorageMounts[2].VolumeTemplate == nil || tool.StorageMounts[2].VolumeTemplate.ReuseKey != "${metadata.session_id}" {
		t.Fatalf("Tool Preview mounts = %+v", tool.StorageMounts)
	}

	sandboxA := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyA)
	owned.sandboxes = append(owned.sandboxes, sandboxA)
	runVolumeCommand(t, ctx, sandboxA, "set -eu; printf direct > /mnt/by-name/probe; test \"$(cat /mnt/by-id/probe)\" = direct; if touch /mnt/by-id/forbidden 2>/dev/null; then exit 90; fi; printf shared-a > /mnt/shared/probe; printf retained > /mnt/retain/probe; printf deleted > /mnt/delete/probe")
	sharedA := waitDerivedVolume(t, ctx, client.Volumes(), shared.ID, keyA)
	owned.volumes[sharedA.ID] = true
	if err := client.Volumes().Delete(ctx, standalone.ID); err == nil {
		t.Fatal("deleting a mounted Volume unexpectedly succeeded")
	}

	sandboxSameKey := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyA)
	owned.sandboxes = append(owned.sandboxes, sandboxSameKey)
	runVolumeCommand(t, ctx, sandboxSameKey, "set -eu; test \"$(cat /mnt/shared/probe)\" = shared-a")
	sharedSameKey := waitDerivedVolume(t, ctx, client.Volumes(), shared.ID, keyA)
	if sharedSameKey.ID != sharedA.ID {
		t.Fatalf("same reuse key resolved to different Volumes: %s and %s", sharedA.ID, sharedSameKey.ID)
	}
	sandboxOtherKey := createE2EVolumeSandbox(t, ctx, client, createdTool.ToolID, keyB)
	owned.sandboxes = append(owned.sandboxes, sandboxOtherKey)
	runVolumeCommand(t, ctx, sandboxOtherKey, "set -eu; test ! -e /mnt/shared/probe; printf shared-b > /mnt/shared/probe")

	sharedB := waitDerivedVolume(t, ctx, client.Volumes(), shared.ID, keyB)
	if sharedA.ID == sharedB.ID {
		t.Fatalf("different reuse keys resolved to one Volume: %s", sharedA.ID)
	}
	owned.volumes[sharedB.ID] = true
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
	client                  *agr.Client
	sandboxes               []*agr.Sandbox
	bootstrap               *agr.Sandbox
	bootstrapCleanupCommand string
	toolIDs                 []string
	volumes                 map[string]bool
	templates               map[string]bool
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
	if o.bootstrap != nil {
		if o.bootstrapCleanupCommand != "" {
			result, err := o.bootstrap.Commands().Run(cleanup, "sh", agr.CommandOptions{Args: []string{"-lc", o.bootstrapCleanupCommand}, User: agr.Root})
			if err != nil || result.ExitCode != 0 {
				t.Errorf("remove persistent Volume fixture path: exit=%d err=%v stderr=%s", result.ExitCode, err, result.Stderr)
			} else {
				o.bootstrapCleanupCommand = ""
			}
		}
		deleteOneVolumeSandbox(t, o.client.Sandboxes(), o.bootstrap)
		o.bootstrap = nil
	}
	for index := len(o.toolIDs) - 1; index >= 0; index-- {
		deleteToolAndConfirm(t, o.client.Tools(), o.toolIDs[index])
	}
	o.toolIDs = nil
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
	created, err := client.Volumes().Create(ctx, agr.CreateVolumeOptions{Name: name, AccessMode: agr.VolumeReadWriteMany, StorageType: environment.storageType, Storage: environment.volumeStorage(path), StorageRoleARN: environment.storageRoleARN, Tags: []agr.VolumeTag{{Key: "agr-sdk-e2e", Value: name}}})
	if err != nil {
		t.Fatal(err)
	}
	return waitVolumeActive(t, ctx, client.Volumes(), created.ID)
}

func createE2ETemplate(t *testing.T, ctx context.Context, client *agr.Client, environment volumeCloudEnvironment, name string, reclaim agr.VolumeReclaimPolicy, pattern string) agr.VolumeTemplateInfo {
	t.Helper()
	created, err := client.VolumeTemplates().Create(ctx, agr.CreateVolumeTemplateOptions{Name: name, AccessMode: agr.VolumeReadWriteMany, ReclaimPolicy: reclaim, StorageType: environment.storageType, Storage: environment.volumeTemplateStorage(pattern), VolumeNamePattern: name + "-${reuse_key}", StorageRoleARN: environment.storageRoleARN, Tags: []agr.VolumeTag{{Key: "agr-sdk-e2e", Value: name}}})
	if err != nil {
		t.Fatal(err)
	}
	return waitTemplateActive(t, ctx, client.VolumeTemplates(), created.ID)
}

func (e volumeCloudEnvironment) bootstrapMount(path string) agr.ToolStorageMount {
	source := &agr.ToolStorageSource{}
	if e.storageType == agr.VolumeStorageCOS {
		source.COS = &agr.ToolCOSStorage{Endpoint: e.cosEndpoint, BucketName: e.cosBucket, BucketPath: path}
	} else {
		source.CFS = &agr.ToolCFSStorage{FileSystemID: e.fileSystemID, Path: "/"}
	}
	return agr.ToolStorageMount{Name: "bootstrap", MountPath: "/mnt/bootstrap", Source: source}
}

func (e volumeCloudEnvironment) fixtureMountPath(path string) string {
	if e.storageType == agr.VolumeStorageCOS {
		return "/mnt/bootstrap"
	}
	return "/mnt/bootstrap/" + path
}

func (e volumeCloudEnvironment) fixtureCleanupCommand(path string) string {
	if e.storageType == agr.VolumeStorageCOS {
		return "rm -rf -- /mnt/bootstrap/* /mnt/bootstrap/.[!.]* /mnt/bootstrap/..?*; test -z \"$(ls -A /mnt/bootstrap)\""
	}
	return "rm -rf -- /mnt/bootstrap/" + path + "; test ! -e /mnt/bootstrap/" + path
}

func (e volumeCloudEnvironment) volumeStorage(path string) agr.VolumeStorage {
	if e.storageType == agr.VolumeStorageCOS {
		return agr.VolumeStorage{COS: &agr.VolumeCOSStorage{Endpoint: e.cosEndpoint, BucketName: e.cosBucket, BucketPath: path}}
	}
	return agr.VolumeStorage{CFS: &agr.VolumeCFSStorage{FileSystemID: e.fileSystemID, Path: "/" + path}}
}

func (e volumeCloudEnvironment) volumeTemplateStorage(pattern string) agr.VolumeTemplateStorage {
	if e.storageType == agr.VolumeStorageCOS {
		return agr.VolumeTemplateStorage{COS: &agr.VolumeTemplateCOSStorage{Endpoint: e.cosEndpoint, BucketName: e.cosBucket, BucketPathPattern: pattern}}
	}
	return agr.VolumeTemplateStorage{CFS: &agr.VolumeTemplateCFSStorage{FileSystemID: e.fileSystemID, PathPattern: "/" + pattern}}
}

type volumeToolFixture struct {
	toolType              agr.ToolType
	network               agr.ToolNetworkConfiguration
	roleARN               string
	customConfiguration   *agr.ToolCustomConfiguration
	computerConfiguration *agr.ToolComputerConfiguration
	logConfiguration      *agr.ToolLogConfiguration
}

func resolveVolumeToolFixture(t *testing.T, ctx context.Context, client *agr.Client, environment volumeCloudEnvironment) volumeToolFixture {
	t.Helper()
	if environment.baseToolID == "" {
		return volumeToolFixture{
			toolType: environment.toolType,
			network:  agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkPublic},
			roleARN:  environment.toolRoleARN,
		}
	}
	base, err := client.Tools().Get(ctx, environment.baseToolID)
	if err != nil {
		t.Fatalf("read base Tool: %v", err)
	}
	if base.State != agr.ToolActive {
		t.Fatalf("base Tool %s is %s, want %s", base.ID, base.State, agr.ToolActive)
	}
	fixture := volumeToolFixture{
		toolType:              base.Type,
		network:               base.Network,
		roleARN:               base.RoleARN,
		computerConfiguration: base.ComputerConfiguration,
		logConfiguration:      base.LogConfiguration,
	}
	if environment.toolRoleARN != "" {
		fixture.roleARN = environment.toolRoleARN
	}
	if base.CustomConfiguration != nil {
		custom := base.CustomConfiguration.ToolCustomConfiguration
		fixture.customConfiguration = &custom
	}
	if fixture.toolType == agr.ToolCustom && fixture.customConfiguration == nil {
		t.Fatal("custom base Tool returned no CustomConfiguration")
	}
	return fixture
}

func (f volumeToolFixture) createOptions(name string, tags []agr.ToolTag, mounts []agr.ToolStorageMount) agr.CreateToolOptions {
	return agr.CreateToolOptions{
		Name:                  name,
		Type:                  f.toolType,
		Network:               f.network,
		RoleARN:               f.roleARN,
		Tags:                  tags,
		StorageMounts:         mounts,
		CustomConfiguration:   f.customConfiguration,
		ComputerConfiguration: f.computerConfiguration,
		LogConfiguration:      f.logConfiguration,
	}
}

func TestVolumeFixtureContract(t *testing.T) {
	cos := volumeCloudEnvironment{storageType: agr.VolumeStorageCOS, cosEndpoint: "cos.ap-guangzhou.myqcloud.com", cosBucket: "fixture-1250000000"}
	cosMount := cos.bootstrapMount("agr-sdk-e2e/run")
	if cosMount.Source == nil || cosMount.Source.COS == nil || cosMount.Source.CFS != nil || cosMount.Source.COS.BucketPath != "agr-sdk-e2e/run" {
		t.Fatalf("COS bootstrap mount = %+v", cosMount)
	}
	cosStorage := cos.volumeStorage("agr-sdk-e2e/run/direct")
	if cosStorage.COS == nil || cosStorage.CFS != nil || cosStorage.COS.Endpoint != cos.cosEndpoint || cosStorage.COS.BucketName != cos.cosBucket || cosStorage.COS.BucketPath != "agr-sdk-e2e/run/direct" {
		t.Fatalf("COS Volume storage = %+v", cosStorage)
	}
	if cos.volumeTemplateStorage("agr-sdk-e2e/run/${reuse_key}").COS == nil || cos.fixtureMountPath("agr-sdk-e2e/run") != "/mnt/bootstrap" {
		t.Fatal("COS fixture projection is incomplete")
	}

	cfs := volumeCloudEnvironment{storageType: agr.VolumeStorageCFS, fileSystemID: "cfs-fixture"}
	cfsMount := cfs.bootstrapMount("agr-sdk-e2e/run")
	if cfsMount.Source == nil || cfsMount.Source.CFS == nil || cfsMount.Source.COS != nil || cfsMount.Source.CFS.Path != "/" {
		t.Fatalf("CFS bootstrap mount = %+v", cfsMount)
	}
	cfsStorage := cfs.volumeStorage("agr-sdk-e2e/run/direct")
	if cfsStorage.CFS == nil || cfsStorage.COS != nil || cfsStorage.CFS.FileSystemID != cfs.fileSystemID || cfsStorage.CFS.Path != "/agr-sdk-e2e/run/direct" {
		t.Fatalf("CFS Volume storage = %+v", cfsStorage)
	}
	if cfs.volumeTemplateStorage("agr-sdk-e2e/run/${reuse_key}").CFS == nil || cfs.fixtureMountPath("agr-sdk-e2e/run") != "/mnt/bootstrap/agr-sdk-e2e/run" {
		t.Fatal("CFS fixture projection is incomplete")
	}

	custom := &agr.ToolCustomConfiguration{Image: "registry.example/fixture:latest", ImageRegistryType: agr.ImageRegistryPersonal}
	tool := volumeToolFixture{toolType: agr.ToolCustom, network: agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkPublic}, roleARN: "fixture-role", customConfiguration: custom}
	options := tool.createOptions("derived", []agr.ToolTag{{Key: "owner", Value: "e2e"}}, []agr.ToolStorageMount{{Name: "data", MountPath: "/mnt/data"}})
	if options.Type != agr.ToolCustom || options.CustomConfiguration != custom || options.RoleARN != "fixture-role" || len(options.Tags) != 1 || len(options.StorageMounts) != 1 {
		t.Fatalf("derived Tool options = %+v", options)
	}
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
