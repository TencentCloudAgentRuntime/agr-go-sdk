package main

import (
	"context"
	"fmt"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	toolType := os.Getenv("AGR_VOLUME_TOOL_TYPE")
	if toolType == "" {
		return fmt.Errorf("AGR_VOLUME_TOOL_TYPE is required")
	}
	fixture, err := volumeFixtureFromEnv()
	if err != nil {
		return err
	}
	region := os.Getenv("TENCENTCLOUD_REGION")
	if region == "" {
		region = "ap-guangzhou"
	}
	client, err := agr.NewClient(agr.WithRegion(region))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	basePath := "agr-sdk-cookbook/" + suffix
	bootstrapTool, err := client.Tools().Create(ctx, agr.CreateToolOptions{
		Name:          "agr-sdk-volume-bootstrap-" + suffix,
		Type:          agr.ToolType(toolType),
		Network:       agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkPublic},
		RoleARN:       os.Getenv("AGR_VOLUME_TOOL_ROLE_ARN"),
		StorageMounts: []agr.ToolStorageMount{fixture.bootstrapMount(basePath)},
	})
	if err != nil {
		return err
	}
	defer deleteTool(client.Tools(), bootstrapTool.ToolID)
	if _, err = client.Tools().WaitFor(ctx, bootstrapTool.ToolID, agr.ToolActive); err != nil {
		return err
	}
	lifetime := 5 * time.Minute
	bootstrap, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool: agr.ToolRef{ID: bootstrapTool.ToolID}, Timeout: &lifetime,
		Metadata: map[string]string{"session_id": "bootstrap-" + suffix},
	})
	if err != nil {
		return err
	}
	defer bootstrap.Close()
	defer deleteSandbox(bootstrap)
	fixturePath := fixture.mountPath(basePath)
	defer removeFixtureData(bootstrap, fixture.cleanupCommand(basePath))
	initialize := "mkdir -p -- " + fixturePath + "/direct " + fixturePath + "/" + suffix
	if fixture.storageType == agr.VolumeStorageCFS {
		initialize += "; chmod -R 0777 -- " + fixturePath
	}
	initialized, err := bootstrap.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", initialize}, User: agr.Root})
	if err != nil || initialized.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("initialize storage fixture: exit %d", initialized.ExitCode)
		}
		return err
	}

	volume, err := client.Volumes().Create(ctx, agr.CreateVolumeOptions{
		Name:           "agr-sdk-volume-" + suffix,
		AccessMode:     agr.VolumeReadWriteMany,
		StorageType:    fixture.storageType,
		Storage:        fixture.volumeStorage(basePath + "/direct"),
		StorageRoleARN: os.Getenv("AGR_VOLUME_STORAGE_ROLE_ARN"),
		Tags:           []agr.VolumeTag{{Key: "agr-sdk-example", Value: "owned"}},
	})
	if err != nil {
		return err
	}
	defer deleteVolume(client.Volumes(), volume.ID)

	template, err := client.VolumeTemplates().Create(ctx, agr.CreateVolumeTemplateOptions{
		Name:              "agr-sdk-template-" + suffix,
		AccessMode:        agr.VolumeReadWriteMany,
		ReclaimPolicy:     agr.VolumeReclaimRetain,
		StorageType:       fixture.storageType,
		Storage:           fixture.volumeTemplateStorage(basePath + "/${reuse_key}"),
		VolumeNamePattern: "agr-sdk-derived-${reuse_key}",
		StorageRoleARN:    os.Getenv("AGR_VOLUME_STORAGE_ROLE_ARN"),
		Tags:              []agr.VolumeTag{{Key: "agr-sdk-example", Value: "owned"}},
	})
	if err != nil {
		return err
	}
	defer deleteTemplateAndDerivedVolumes(client, template.ID)

	readOnly := true
	tool, err := client.Tools().Create(ctx, agr.CreateToolOptions{
		Name:    "agr-sdk-volume-tool-" + suffix,
		Type:    agr.ToolType(toolType),
		Network: agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkPublic},
		RoleARN: os.Getenv("AGR_VOLUME_TOOL_ROLE_ARN"),
		StorageMounts: []agr.ToolStorageMount{
			{Name: "direct", MountPath: "/mnt/direct", Volume: &agr.ToolVolumeRef{ID: volume.ID}},
			{Name: "direct-read-only", MountPath: "/mnt/direct-ro", ReadOnly: &readOnly, Volume: &agr.ToolVolumeRef{Name: volume.Name}},
			{Name: "session", MountPath: "/mnt/session", VolumeTemplate: &agr.ToolVolumeTemplateRef{Name: template.Name, ReuseKey: "${metadata.session_id}"}},
		},
	})
	if err != nil {
		return err
	}
	defer deleteTool(client.Tools(), tool.ToolID)
	if _, err = client.Tools().WaitFor(ctx, tool.ToolID, agr.ToolActive); err != nil {
		return err
	}

	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool:     agr.ToolRef{ID: tool.ToolID},
		Timeout:  &lifetime,
		Metadata: map[string]string{"session_id": suffix},
	})
	if err != nil {
		return err
	}
	defer sandbox.Close()
	defer deleteSandbox(sandbox)
	result, err := sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "set -eu; printf direct > /mnt/direct/probe; test \"$(cat /mnt/direct-ro/probe)\" = direct; printf session > /mnt/session/probe; printf persistent-volume-ok"}, User: agr.Root})
	if err != nil {
		return err
	}
	fmt.Println(string(result.Stdout))
	return nil
}

func removeFixtureData(sandbox *agr.Sandbox, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_, _ = sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", command}, User: agr.Root})
}

type volumeFixture struct {
	storageType  agr.VolumeStorageType
	cosEndpoint  string
	cosBucket    string
	fileSystemID string
}

func volumeFixtureFromEnv() (volumeFixture, error) {
	fixture := volumeFixture{
		cosEndpoint:  os.Getenv("AGR_VOLUME_COS_ENDPOINT"),
		cosBucket:    os.Getenv("AGR_VOLUME_COS_BUCKET"),
		fileSystemID: os.Getenv("AGR_VOLUME_CFS_FILESYSTEM_ID"),
	}
	if (fixture.cosEndpoint == "") != (fixture.cosBucket == "") {
		return volumeFixture{}, fmt.Errorf("AGR_VOLUME_COS_ENDPOINT and AGR_VOLUME_COS_BUCKET must be set together")
	}
	hasCOS, hasCFS := fixture.cosEndpoint != "", fixture.fileSystemID != ""
	if hasCOS == hasCFS {
		return volumeFixture{}, fmt.Errorf("exactly one COS or CFS fixture is required")
	}
	fixture.storageType = agr.VolumeStorageCFS
	if hasCOS {
		fixture.storageType = agr.VolumeStorageCOS
	}
	return fixture, nil
}

func (f volumeFixture) bootstrapMount(path string) agr.ToolStorageMount {
	source := &agr.ToolStorageSource{}
	if f.storageType == agr.VolumeStorageCOS {
		source.COS = &agr.ToolCOSStorage{Endpoint: f.cosEndpoint, BucketName: f.cosBucket, BucketPath: path}
	} else {
		source.CFS = &agr.ToolCFSStorage{FileSystemID: f.fileSystemID, Path: "/"}
	}
	return agr.ToolStorageMount{Name: "bootstrap", MountPath: "/mnt/bootstrap", Source: source}
}

func (f volumeFixture) mountPath(path string) string {
	if f.storageType == agr.VolumeStorageCOS {
		return "/mnt/bootstrap"
	}
	return "/mnt/bootstrap/" + path
}

func (f volumeFixture) cleanupCommand(path string) string {
	if f.storageType == agr.VolumeStorageCOS {
		return "rm -rf -- /mnt/bootstrap/* /mnt/bootstrap/.[!.]* /mnt/bootstrap/..?*; test -z \"$(ls -A /mnt/bootstrap)\""
	}
	return "rm -rf -- /mnt/bootstrap/" + path + "; test ! -e /mnt/bootstrap/" + path
}

func (f volumeFixture) volumeStorage(path string) agr.VolumeStorage {
	if f.storageType == agr.VolumeStorageCOS {
		return agr.VolumeStorage{COS: &agr.VolumeCOSStorage{Endpoint: f.cosEndpoint, BucketName: f.cosBucket, BucketPath: path}}
	}
	return agr.VolumeStorage{CFS: &agr.VolumeCFSStorage{FileSystemID: f.fileSystemID, Path: "/" + path}}
}

func (f volumeFixture) volumeTemplateStorage(pattern string) agr.VolumeTemplateStorage {
	if f.storageType == agr.VolumeStorageCOS {
		return agr.VolumeTemplateStorage{COS: &agr.VolumeTemplateCOSStorage{Endpoint: f.cosEndpoint, BucketName: f.cosBucket, BucketPathPattern: pattern}}
	}
	return agr.VolumeTemplateStorage{CFS: &agr.VolumeTemplateCFSStorage{FileSystemID: f.fileSystemID, PathPattern: "/" + pattern}}
}

func deleteSandbox(sandbox *agr.Sandbox) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = sandbox.Delete(ctx)
}

func deleteTool(manager *agr.ToolManager, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = manager.Delete(ctx, id)
}

func deleteVolume(manager *agr.VolumeManager, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = manager.Delete(ctx, id)
}

func deleteTemplateAndDerivedVolumes(client *agr.Client, templateID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	page, err := client.Volumes().List(ctx, agr.VolumeListOptions{Filters: []agr.VolumeFilter{{Name: "volume-template-id", Values: []string{templateID}}}, Limit: 100})
	if err == nil {
		for _, volume := range page.Items {
			_ = client.Volumes().Delete(ctx, volume.ID)
		}
	}
	_ = client.VolumeTemplates().Delete(ctx, templateID)
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
