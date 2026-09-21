package main

import (
	"context"
	"fmt"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	fileSystemID := os.Getenv("AGR_VOLUME_CFS_FILESYSTEM_ID")
	toolType := os.Getenv("AGR_VOLUME_TOOL_TYPE")
	if fileSystemID == "" || toolType == "" {
		return fmt.Errorf("AGR_VOLUME_CFS_FILESYSTEM_ID and AGR_VOLUME_TOOL_TYPE are required")
	}
	region := os.Getenv("TENCENTCLOUD_REGION")
	if region == "" {
		region = "ap-shanghai"
	}
	client, err := agr.NewClient(agr.WithRegion(region))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	basePath := "/agr-sdk-cookbook/" + suffix
	volume, err := client.Volumes().Create(ctx, agr.CreateVolumeOptions{
		Name:        "agr-sdk-volume-" + suffix,
		AccessMode:  agr.VolumeReadWriteMany,
		StorageType: agr.VolumeStorageCFS,
		Storage: agr.VolumeStorage{CFS: &agr.VolumeCFSStorage{
			FileSystemID: fileSystemID,
			Path:         basePath + "/direct",
		}},
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
		StorageType:       agr.VolumeStorageCFS,
		Storage:           agr.VolumeTemplateStorage{CFS: &agr.VolumeTemplateCFSStorage{FileSystemID: fileSystemID, PathPattern: basePath + "/${reuse_key}"}},
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

	lifetime := 5 * time.Minute
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
