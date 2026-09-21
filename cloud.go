package agr

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/controlplane"
)

// controlAdapter only converts stable public models to private models. Cloud
// orchestration, signing, token acquisition, and runtime construction belong
// to internal/controlplane.Service.
type controlAdapter struct {
	service  *controlplane.Service
	endpoint string
}

func newDefaultTencentControlPlane(region string, credential CredentialProvider) *controlAdapter {
	return newTencentControlPlane(region, credential, "ags.tencentcloudapi.com", nil, 30*time.Second)
}

func newTencentControlPlane(region string, credential CredentialProvider, endpoint string, client *http.Client, timeout time.Duration) *controlAdapter {
	return newControlAdapter(region, credential, endpoint, "monitor.tencentcloudapi.com", client, timeout)
}

func newControlAdapter(region string, credential CredentialProvider, controlEndpoint, monitorEndpoint string, client *http.Client, timeout time.Duration) *controlAdapter {
	source := func(ctx context.Context) (controlplane.Credential, error) {
		value, err := retrieveCloudCredential(ctx, credential)
		return controlplane.Credential{SecretID: value.SecretID, SecretKey: value.SecretKey, Token: value.Token}, err
	}
	return &controlAdapter{service: controlplane.NewService(controlplane.ServiceConfig{Region: region, ControlEndpoint: controlEndpoint, MonitorEndpoint: monitorEndpoint, Timeout: timeout, HTTPClient: client, Credential: source}), endpoint: controlEndpoint}
}

func (c *controlAdapter) Create(ctx context.Context, opts CreateOptions) (SandboxInfo, error) {
	in := controlplane.CreateRequest{ToolID: opts.Tool.ID, ToolName: opts.Tool.Name, Timeout: opts.Timeout, ClientToken: opts.ClientToken, Metadata: opts.Metadata, Env: opts.Env, AuthMode: string(opts.AuthMode)}
	for _, mount := range opts.MountOptions {
		in.MountOptions = append(in.MountOptions, controlplane.MountOption{Name: mount.Name, MountPath: mount.MountPath, SubPath: mount.SubPath, ReadOnly: mount.ReadOnly})
	}
	out, err := c.service.Create(ctx, in)
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) Get(ctx context.Context, id string) (SandboxInfo, error) {
	out, err := c.service.Get(ctx, id)
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) Connect(ctx context.Context, id string, timeout time.Duration) (SandboxInfo, error) {
	out, err := c.service.Connect(ctx, id, timeout)
	if err != nil {
		err = mapMutationError(id, err)
	}
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) List(ctx context.Context, opts SandboxListOptions) (SandboxPage, error) {
	in, err := controlListRequest(opts)
	if err != nil {
		return SandboxPage{}, err
	}
	out, err := c.service.List(ctx, in)
	if err != nil {
		return SandboxPage{}, normalizeError("", err)
	}
	items := make([]SandboxInfo, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, sandboxInfoFrom(item))
	}
	next := opts.Offset + len(items)
	var nextPtr *int
	if next < out.TotalCount {
		nextPtr = &next
	}
	return SandboxPage{Items: items, TotalCount: out.TotalCount, NextOffset: nextPtr}, nil
}

func (c *controlAdapter) ListPage(ctx context.Context, opts SandboxPageOptions) (SandboxCursorPage, error) {
	in, err := controlCursorListRequest(opts)
	if err != nil {
		return SandboxCursorPage{}, err
	}
	out, err := c.service.ListPage(ctx, in)
	if err != nil {
		return SandboxCursorPage{}, normalizeError("Sandboxes.ListPage", err)
	}
	items := make([]SandboxInfo, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, sandboxInfoFrom(item))
	}
	page := SandboxCursorPage{Items: items, NextToken: out.NextToken}
	if opts.NeedTotalCount {
		total := out.TotalCount
		page.TotalCount = &total
	}
	return page, nil
}

func (c *controlAdapter) Pause(ctx context.Context, id string, opts PauseOptions) (SandboxInfo, error) {
	out, err := c.service.Pause(ctx, id, opts.Mode != PauseDisk)
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) Resume(ctx context.Context, id string, opts ResumeOptions) (SandboxInfo, error) {
	out, err := c.service.Resume(ctx, id, opts.Timeout)
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) Delete(ctx context.Context, id string) error {
	return normalizeError("", c.service.Delete(ctx, id))
}

func (c *controlAdapter) waitFor(ctx context.Context, id string, target SandboxState) (SandboxInfo, error) {
	out, err := c.service.WaitFor(ctx, id, controlplane.State(target))
	return sandboxInfoFrom(out), normalizeError("", err)
}

func (c *controlAdapter) dataPlane(ctx context.Context, id string) (dataPlane, error) {
	generation, err := c.service.Runtime(ctx, id)
	if err != nil {
		return nil, normalizeError("", err)
	}
	return newRuntimeDataPlane(generation), nil
}

func (c *controlAdapter) Update(ctx context.Context, id string, opts UpdateOptions) error {
	err := c.service.Update(ctx, id, opts.Timeout, opts.MetadataUpsert)
	if err == nil {
		return nil
	}
	return mapMutationError(id, err)
}

func (c *controlAdapter) CreateTool(ctx context.Context, opts CreateToolOptions) (ToolMutationResult, error) {
	out, err := c.service.CreateTool(ctx, controlCreateTool(opts))
	if err != nil {
		return ToolMutationResult{}, normalizeError("Tools.Create", err)
	}
	return ToolMutationResult{ToolID: out.ToolID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) GetTool(ctx context.Context, id string) (ToolInfo, error) {
	out, err := c.service.GetTool(ctx, id)
	if err != nil {
		return ToolInfo{}, withResourceError(normalizeError("Tools.Get", err), "tool", id)
	}
	return toolInfoFrom(out), nil
}

func (c *controlAdapter) ListTools(ctx context.Context, opts ToolListOptions) (ToolPage, error) {
	in := controlplane.ToolListRequest{ToolIDs: opts.ToolIDs, Offset: opts.Offset, Limit: opts.Limit}
	for _, filter := range opts.Filters {
		in.Filters = append(in.Filters, controlplane.ToolFilter{Name: filter.Name, Values: filter.Values})
	}
	out, err := c.service.ListTools(ctx, in)
	if err != nil {
		return ToolPage{}, normalizeError("Tools.List", err)
	}
	page := ToolPage{TotalCount: out.TotalCount, Items: make([]ToolInfo, 0, len(out.Items))}
	for _, item := range out.Items {
		page.Items = append(page.Items, toolInfoFrom(item))
	}
	next := opts.Offset + len(page.Items)
	if next < out.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateTool(ctx context.Context, id string, opts UpdateToolOptions) (ToolMutationResult, error) {
	out, err := c.service.UpdateTool(ctx, id, controlUpdateTool(opts))
	if err != nil {
		return ToolMutationResult{}, withResourceError(normalizeError("Tools.Update", err), "tool", id)
	}
	return ToolMutationResult{ToolID: out.ToolID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DeleteTool(ctx context.Context, id string) error {
	return withResourceError(normalizeError("Tools.Delete", c.service.DeleteTool(ctx, id)), "tool", id)
}

func (c *controlAdapter) CreatePreCacheImage(ctx context.Context, opts CreatePreCacheImageOptions) (PreCacheImageResult, error) {
	out, err := c.service.CreatePreCacheImage(ctx, opts.Image, string(opts.RegistryType))
	if err != nil {
		return PreCacheImageResult{}, normalizeError("PreCacheImages.Create", err)
	}
	return PreCacheImageResult{Ref: preCacheRefFrom(out.Ref), RequestID: out.RequestID}, nil
}

func (c *controlAdapter) GetPreCacheImage(ctx context.Context, ref PreCacheImageRef) (PreCacheImageInfo, error) {
	out, err := c.service.GetPreCacheImage(ctx, controlplane.PreCacheImageRef{Image: ref.Image, Digest: ref.Digest, RegistryType: string(ref.RegistryType)})
	if err != nil {
		return PreCacheImageInfo{}, normalizeError("PreCacheImages.Get", err)
	}
	return PreCacheImageInfo{Ref: preCacheRefFrom(out.Ref), Status: out.Status, Message: out.Message, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) CreateDeployment(ctx context.Context, opts CreateDeploymentOptions) (DeploymentInfo, error) {
	out, err := c.service.CreateDeployment(ctx, controlCreateDeployment(opts))
	if err != nil {
		return DeploymentInfo{}, normalizeError("Deployments.Create", err)
	}
	return deploymentInfoFrom(out), nil
}

func (c *controlAdapter) GetDeployment(ctx context.Context, id string) (DeploymentInfo, error) {
	out, err := c.service.GetDeployment(ctx, id)
	if err != nil {
		return DeploymentInfo{}, withResourceError(normalizeError("Deployments.Get", err), "deployment", id)
	}
	return deploymentInfoFrom(out), nil
}

func (c *controlAdapter) ListDeployments(ctx context.Context, opts DeploymentListOptions) (DeploymentPage, error) {
	in := controlplane.DeploymentListRequest{Offset: opts.Offset, Limit: opts.Limit}
	for _, filter := range opts.Filters {
		in.Filters = append(in.Filters, controlplane.DeploymentFilter{Name: filter.Name, Values: filter.Values})
	}
	out, err := c.service.ListDeployments(ctx, in)
	if err != nil {
		return DeploymentPage{}, normalizeError("Deployments.List", err)
	}
	page := DeploymentPage{TotalCount: out.TotalCount, RequestID: out.RequestID, Items: make([]DeploymentInfo, 0, len(out.Items))}
	for _, item := range out.Items {
		page.Items = append(page.Items, deploymentInfoFrom(controlplane.DeploymentResult{Deployment: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateDeployment(ctx context.Context, id string, opts UpdateDeploymentOptions) (DeploymentInfo, error) {
	out, err := c.service.UpdateDeployment(ctx, id, controlUpdateDeployment(opts))
	if err != nil {
		return DeploymentInfo{}, withResourceError(normalizeError("Deployments.Update", err), "deployment", id)
	}
	return deploymentInfoFrom(out), nil
}

func (c *controlAdapter) DeleteDeployment(ctx context.Context, id string) error {
	return withResourceError(normalizeError("Deployments.Delete", c.service.DeleteDeployment(ctx, id)), "deployment", id)
}

func (c *controlAdapter) CreateVolume(ctx context.Context, opts CreateVolumeOptions) (VolumeInfo, error) {
	out, err := c.service.CreateVolume(ctx, controlCreateVolume(opts))
	if err != nil {
		return VolumeInfo{}, normalizeError("Volumes.Create", err)
	}
	return volumeInfoFrom(out), nil
}

func (c *controlAdapter) GetVolume(ctx context.Context, id string) (VolumeInfo, error) {
	out, err := c.service.GetVolume(ctx, id)
	if err != nil {
		return VolumeInfo{}, withResourceError(normalizeError("Volumes.Get", err), "volume", id)
	}
	return volumeInfoFrom(out), nil
}

func (c *controlAdapter) ListVolumes(ctx context.Context, opts VolumeListOptions) (VolumePage, error) {
	out, err := c.service.ListVolumes(ctx, controlVolumeList(opts))
	if err != nil {
		return VolumePage{}, normalizeError("Volumes.List", err)
	}
	page := VolumePage{TotalCount: out.TotalCount, RequestID: out.RequestID, Items: make([]VolumeInfo, 0, len(out.Items))}
	for _, item := range out.Items {
		page.Items = append(page.Items, volumeInfoFrom(controlplane.VolumeResult{Volume: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateVolume(ctx context.Context, id string, opts UpdateVolumeOptions) (VolumeMutationResult, error) {
	out, err := c.service.UpdateVolume(ctx, id, controlUpdateVolume(opts))
	if err != nil {
		return VolumeMutationResult{}, withResourceError(normalizeError("Volumes.Update", err), "volume", id)
	}
	return VolumeMutationResult{VolumeID: out.ResourceID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DeleteVolume(ctx context.Context, id string) error {
	return withResourceError(normalizeError("Volumes.Delete", c.service.DeleteVolume(ctx, id)), "volume", id)
}

func (c *controlAdapter) CreateVolumeTemplate(ctx context.Context, opts CreateVolumeTemplateOptions) (VolumeTemplateInfo, error) {
	out, err := c.service.CreateVolumeTemplate(ctx, controlCreateVolumeTemplate(opts))
	if err != nil {
		return VolumeTemplateInfo{}, normalizeError("VolumeTemplates.Create", err)
	}
	return volumeTemplateInfoFrom(out), nil
}

func (c *controlAdapter) GetVolumeTemplate(ctx context.Context, id string) (VolumeTemplateInfo, error) {
	out, err := c.service.GetVolumeTemplate(ctx, id)
	if err != nil {
		return VolumeTemplateInfo{}, withResourceError(normalizeError("VolumeTemplates.Get", err), "volume_template", id)
	}
	return volumeTemplateInfoFrom(out), nil
}

func (c *controlAdapter) ListVolumeTemplates(ctx context.Context, opts VolumeTemplateListOptions) (VolumeTemplatePage, error) {
	out, err := c.service.ListVolumeTemplates(ctx, controlVolumeTemplateList(opts))
	if err != nil {
		return VolumeTemplatePage{}, normalizeError("VolumeTemplates.List", err)
	}
	page := VolumeTemplatePage{TotalCount: out.TotalCount, RequestID: out.RequestID, Items: make([]VolumeTemplateInfo, 0, len(out.Items))}
	for _, item := range out.Items {
		page.Items = append(page.Items, volumeTemplateInfoFrom(controlplane.VolumeTemplateResult{Template: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateVolumeTemplate(ctx context.Context, id string, opts UpdateVolumeTemplateOptions) (VolumeTemplateMutationResult, error) {
	out, err := c.service.UpdateVolumeTemplate(ctx, id, controlUpdateVolumeTemplate(opts))
	if err != nil {
		return VolumeTemplateMutationResult{}, withResourceError(normalizeError("VolumeTemplates.Update", err), "volume_template", id)
	}
	return VolumeTemplateMutationResult{VolumeTemplateID: out.ResourceID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DeleteVolumeTemplate(ctx context.Context, id string) error {
	return withResourceError(normalizeError("VolumeTemplates.Delete", c.service.DeleteVolumeTemplate(ctx, id)), "volume_template", id)
}

func (c *controlAdapter) acquireDeploymentToken(ctx context.Context, id string) (deploymentAccess, error) {
	out, err := c.service.AcquireDeploymentToken(ctx, id)
	if err != nil {
		return deploymentAccess{}, withResourceError(normalizeError("Deployments.Connect", err), "deployment", id)
	}
	return deploymentAccess{token: out.Token, expiresAt: out.ExpiresAt}, nil
}

func controlCreateDeployment(value CreateDeploymentOptions) controlplane.CreateDeploymentRequest {
	out := controlplane.CreateDeploymentRequest{Name: value.Name, ToolID: value.ToolID}
	if value.Scaling != nil {
		out.Scaling = &controlplane.DeploymentScalingInput{MinInstances: value.Scaling.MinInstances, MaxInstances: value.Scaling.MaxInstances, MaxConcurrency: value.Scaling.MaxConcurrency}
	}
	if value.Lifecycle != nil {
		out.Lifecycle = &controlplane.DeploymentLifecycleInput{IdleTimeoutSeconds: value.Lifecycle.IdleTimeoutSeconds}
		if value.Lifecycle.IdleAction != nil {
			action := string(*value.Lifecycle.IdleAction)
			out.Lifecycle.IdleAction = &action
		}
	}
	if value.Affinity != nil {
		out.Affinity = &controlplane.DeploymentAffinityInput{Mode: string(value.Affinity.Mode), HeaderName: value.Affinity.HeaderName}
	}
	for _, tag := range value.Tags {
		out.Tags = append(out.Tags, controlplane.DeploymentTag{Key: tag.Key, Value: tag.Value})
	}
	return out
}

func controlUpdateDeployment(value UpdateDeploymentOptions) controlplane.UpdateDeploymentRequest {
	out := controlplane.UpdateDeploymentRequest{}
	if value.Scaling != nil {
		min, max, concurrency := value.Scaling.MinInstances, value.Scaling.MaxInstances, value.Scaling.MaxConcurrency
		out.Scaling = &controlplane.DeploymentScalingInput{MinInstances: &min, MaxInstances: &max, MaxConcurrency: &concurrency}
	}
	if value.Lifecycle != nil {
		timeout, action := value.Lifecycle.IdleTimeoutSeconds, string(value.Lifecycle.IdleAction)
		out.Lifecycle = &controlplane.DeploymentLifecycleInput{IdleTimeoutSeconds: &timeout, IdleAction: &action}
	}
	if value.Tags != nil {
		tags := make([]controlplane.DeploymentTag, 0, len(*value.Tags))
		for _, tag := range *value.Tags {
			tags = append(tags, controlplane.DeploymentTag{Key: tag.Key, Value: tag.Value})
		}
		out.Tags = &tags
	}
	return out
}

func deploymentInfoFrom(value controlplane.DeploymentResult) DeploymentInfo {
	in := value.Deployment
	out := DeploymentInfo{ID: in.ID, Name: in.Name, ToolID: in.ToolID, State: DeploymentState(in.State), StatusReason: in.StatusReason, Scaling: DeploymentScaling{MinInstances: in.Scaling.MinInstances, MaxInstances: in.Scaling.MaxInstances, MaxConcurrency: in.Scaling.MaxConcurrency}, Lifecycle: DeploymentLifecycle{IdleTimeoutSeconds: in.Lifecycle.IdleTimeoutSeconds, IdleAction: DeploymentIdleAction(in.Lifecycle.IdleAction)}, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, RequestID: value.RequestID}
	if in.Affinity != nil {
		out.Affinity = &DeploymentAffinity{Mode: DeploymentAffinityMode(in.Affinity.Mode), HeaderName: in.Affinity.HeaderName}
	}
	for _, tag := range in.Tags {
		out.Tags = append(out.Tags, DeploymentTag{Key: tag.Key, Value: tag.Value})
	}
	return out
}

func controlVolumeTags(values []VolumeTag) []controlplane.VolumeTag {
	out := make([]controlplane.VolumeTag, 0, len(values))
	for _, value := range values {
		out = append(out, controlplane.VolumeTag{Key: value.Key, Value: value.Value})
	}
	return out
}

func controlVolumeStorage(value VolumeStorage) controlplane.VolumeStorage {
	out := controlplane.VolumeStorage{}
	if value.COS != nil {
		out.COS = &controlplane.VolumeCOS{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPath: value.COS.BucketPath}
	}
	if value.CFS != nil {
		out.CFS = &controlplane.VolumeCFS{FileSystemID: value.CFS.FileSystemID, Path: value.CFS.Path}
	}
	return out
}

func controlVolumeTemplateStorage(value VolumeTemplateStorage) controlplane.VolumeStorageSpec {
	out := controlplane.VolumeStorageSpec{}
	if value.COS != nil {
		out.COS = &controlplane.VolumeCOSSpec{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPathPattern: value.COS.BucketPathPattern}
	}
	if value.CFS != nil {
		out.CFS = &controlplane.VolumeCFSSpec{FileSystemID: value.CFS.FileSystemID, PathPattern: value.CFS.PathPattern}
	}
	return out
}

func controlCreateVolume(value CreateVolumeOptions) controlplane.CreateVolumeRequest {
	return controlplane.CreateVolumeRequest{Name: value.Name, AccessMode: string(value.AccessMode), StorageType: string(value.StorageType), Storage: controlVolumeStorage(value.Storage), Tags: controlVolumeTags(value.Tags), ClientToken: value.ClientToken, StorageRoleARN: value.StorageRoleARN}
}

func controlCreateVolumeTemplate(value CreateVolumeTemplateOptions) controlplane.CreateVolumeTemplateRequest {
	return controlplane.CreateVolumeTemplateRequest{Name: value.Name, AccessMode: string(value.AccessMode), ReclaimPolicy: string(value.ReclaimPolicy), StorageType: string(value.StorageType), StorageSpec: controlVolumeTemplateStorage(value.Storage), VolumeNamePattern: value.VolumeNamePattern, DefaultCapacity: value.DefaultCapacity, StorageRoleARN: value.StorageRoleARN, Tags: controlVolumeTags(value.Tags), ClientToken: value.ClientToken}
}

func controlVolumeList(value VolumeListOptions) controlplane.VolumeListRequest {
	out := controlplane.VolumeListRequest{IDs: value.IDs, Names: value.Names, Offset: value.Offset, Limit: value.Limit}
	for _, filter := range value.Filters {
		out.Filters = append(out.Filters, controlplane.VolumeFilter{Name: filter.Name, Values: filter.Values})
	}
	return out
}

func controlVolumeTemplateList(value VolumeTemplateListOptions) controlplane.VolumeListRequest {
	return controlVolumeList(VolumeListOptions{IDs: value.IDs, Names: value.Names, Filters: value.Filters, Offset: value.Offset, Limit: value.Limit})
}

func controlUpdateVolume(value UpdateVolumeOptions) controlplane.UpdateVolumeRequest {
	out := controlplane.UpdateVolumeRequest{}
	if value.Tags != nil {
		tags := controlVolumeTags(*value.Tags)
		out.Tags = &tags
	}
	return out
}

func controlUpdateVolumeTemplate(value UpdateVolumeTemplateOptions) controlplane.UpdateVolumeTemplateRequest {
	out := controlplane.UpdateVolumeTemplateRequest{DefaultCapacity: value.DefaultCapacity}
	if value.Tags != nil {
		tags := controlVolumeTags(*value.Tags)
		out.Tags = &tags
	}
	return out
}

func volumeInfoFrom(value controlplane.VolumeResult) VolumeInfo {
	in := value.Volume
	out := VolumeInfo{ID: in.ID, Name: in.Name, AccessMode: VolumeAccessMode(in.AccessMode), ReclaimPolicy: VolumeReclaimPolicy(in.ReclaimPolicy), State: VolumeState(in.State), StorageType: VolumeStorageType(in.StorageType), TemplateID: in.TemplateID, ReuseKey: in.ReuseKey, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, StorageRoleARN: in.StorageRoleARN, MountedInstanceID: in.MountedInstanceID, RequestID: value.RequestID}
	out.Storage = volumeStorageFromControl(in.Storage)
	for _, tag := range in.Tags {
		out.Tags = append(out.Tags, VolumeTag{Key: tag.Key, Value: tag.Value})
	}
	return out
}

func volumeStorageFromControl(value controlplane.VolumeStorage) VolumeStorage {
	out := VolumeStorage{}
	if value.COS != nil {
		out.COS = &VolumeCOSStorage{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPath: value.COS.BucketPath}
	}
	if value.CFS != nil {
		out.CFS = &VolumeCFSStorage{FileSystemID: value.CFS.FileSystemID, Path: value.CFS.Path}
	}
	return out
}

func volumeTemplateInfoFrom(value controlplane.VolumeTemplateResult) VolumeTemplateInfo {
	in := value.Template
	out := VolumeTemplateInfo{ID: in.ID, Name: in.Name, AccessMode: VolumeAccessMode(in.AccessMode), ReclaimPolicy: VolumeReclaimPolicy(in.ReclaimPolicy), State: VolumeState(in.State), StorageType: VolumeStorageType(in.StorageType), VolumeNamePattern: in.VolumeNamePattern, DefaultCapacity: in.DefaultCapacity, StorageRoleARN: in.StorageRoleARN, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, RequestID: value.RequestID}
	if in.StorageSpec.COS != nil {
		out.Storage.COS = &VolumeTemplateCOSStorage{Endpoint: in.StorageSpec.COS.Endpoint, BucketName: in.StorageSpec.COS.BucketName, BucketPathPattern: in.StorageSpec.COS.BucketPathPattern}
	}
	if in.StorageSpec.CFS != nil {
		out.Storage.CFS = &VolumeTemplateCFSStorage{FileSystemID: in.StorageSpec.CFS.FileSystemID, PathPattern: in.StorageSpec.CFS.PathPattern}
	}
	for _, tag := range in.Tags {
		out.Tags = append(out.Tags, VolumeTag{Key: tag.Key, Value: tag.Value})
	}
	return out
}

func controlCreateTool(value CreateToolOptions) controlplane.CreateToolRequest {
	out := controlplane.CreateToolRequest{Name: value.Name, Type: string(value.Type), Description: value.Description, DefaultTimeout: value.DefaultTimeout, ClientToken: value.ClientToken, RoleARN: value.RoleARN, Network: controlToolNetwork(value.Network), Persistent: value.Persistent}
	for _, tag := range value.Tags {
		out.Tags = append(out.Tags, controlplane.ToolTag{Key: tag.Key, Value: tag.Value})
	}
	out.StorageMounts = controlToolMounts(value.StorageMounts)
	out.Custom, out.Computer, out.Log = controlToolCustom(value.CustomConfiguration), controlToolComputer(value.ComputerConfiguration), controlToolLog(value.LogConfiguration)
	return out
}

func controlUpdateTool(value UpdateToolOptions) controlplane.UpdateToolRequest {
	out := controlplane.UpdateToolRequest{Description: value.Description, Custom: controlToolCustom(value.CustomConfiguration), Computer: controlToolComputer(value.ComputerConfiguration)}
	if value.Network != nil {
		network := controlToolNetwork(*value.Network)
		out.Network = &network
	}
	if value.Tags != nil {
		tags := make([]controlplane.ToolTag, 0, len(*value.Tags))
		for _, tag := range *value.Tags {
			tags = append(tags, controlplane.ToolTag{Key: tag.Key, Value: tag.Value})
		}
		out.Tags = &tags
	}
	return out
}

func controlToolNetwork(value ToolNetworkConfiguration) controlplane.ToolNetwork {
	out := controlplane.ToolNetwork{Mode: string(value.Mode)}
	if value.VPC != nil {
		out.VPC = &controlplane.ToolVPC{SubnetIDs: value.VPC.SubnetIDs, SecurityGroupIDs: value.VPC.SecurityGroupIDs}
	}
	return out
}
func controlToolCustom(value *ToolCustomConfiguration) *controlplane.ToolCustom {
	if value == nil {
		return nil
	}
	out := &controlplane.ToolCustom{Image: value.Image, RegistryType: string(value.ImageRegistryType), Command: value.Command, Args: value.Args}
	for _, v := range value.Env {
		out.Env = append(out.Env, controlplane.ToolEnv{Name: v.Name, Value: v.Value})
	}
	for _, v := range value.Ports {
		out.Ports = append(out.Ports, controlplane.ToolPort{Name: v.Name, Protocol: v.Protocol, Port: v.Port})
	}
	if value.Resources != nil {
		out.Resources = &controlplane.ToolResources{CPU: value.Resources.CPU, Memory: value.Resources.Memory, Storage: value.Resources.Storage}
	}
	if value.Probe != nil {
		out.Probe = &controlplane.ToolProbe{ReadyTimeoutMS: value.Probe.ReadyTimeoutMS, ProbeTimeoutMS: value.Probe.ProbeTimeoutMS, ProbePeriodMS: value.Probe.ProbePeriodMS, SuccessThreshold: value.Probe.SuccessThreshold, FailureThreshold: value.Probe.FailureThreshold}
		if value.Probe.HTTPGet != nil {
			out.Probe.HTTPGet = &controlplane.ToolHTTPGet{Path: value.Probe.HTTPGet.Path, Scheme: value.Probe.HTTPGet.Scheme, Port: value.Probe.HTTPGet.Port}
		}
	}
	if value.DNS != nil {
		out.DNS = &controlplane.ToolDNS{Servers: value.DNS.Servers, Searches: value.DNS.Searches, Options: value.DNS.Options}
	}
	return out
}
func controlToolComputer(value *ToolComputerConfiguration) *controlplane.ToolComputer {
	if value == nil {
		return nil
	}
	return &controlplane.ToolComputer{WAAImageID: value.WAAImageID, OSWorldVersion: value.OSWorldVersion}
}
func controlToolLog(value *ToolLogConfiguration) *controlplane.ToolLog {
	if value == nil {
		return nil
	}
	return &controlplane.ToolLog{CLSTopicID: value.CLSTopicID, Files: value.Files}
}
func controlToolMounts(values []ToolStorageMount) []controlplane.ToolStorageMount {
	out := make([]controlplane.ToolStorageMount, 0, len(values))
	for _, v := range values {
		item := controlplane.ToolStorageMount{Name: v.Name, MountPath: v.MountPath, ReadOnly: v.ReadOnly}
		if v.Source != nil {
			source := &controlplane.ToolStorageSource{}
			if v.Source.COS != nil {
				source.COS = &controlplane.ToolCOS{Endpoint: v.Source.COS.Endpoint, BucketName: v.Source.COS.BucketName, BucketPath: v.Source.COS.BucketPath}
			}
			if v.Source.CFS != nil {
				source.CFS = &controlplane.ToolCFS{FileSystemID: v.Source.CFS.FileSystemID, Path: v.Source.CFS.Path}
			}
			if v.Source.Image != nil {
				source.Image = &controlplane.ToolImage{Reference: v.Source.Image.Reference, RegistryType: v.Source.Image.ImageRegistryType, SubPath: v.Source.Image.SubPath, Digest: v.Source.Image.Digest}
			}
			if v.Source.AgentBucket != nil {
				source.AgentBucket = &controlplane.ToolAgentBucket{LibraryID: v.Source.AgentBucket.LibraryID, SpaceID: v.Source.AgentBucket.SpaceID, AccessDomain: v.Source.AgentBucket.AccessDomain}
			}
			item.Source = source
		}
		if v.Volume != nil {
			item.Volume = &controlplane.ToolVolumeRef{ID: v.Volume.ID, Name: v.Volume.Name}
		}
		if v.VolumeTemplate != nil {
			item.VolumeTemplate = &controlplane.ToolVolumeTemplateRef{Name: v.VolumeTemplate.Name, ReuseKey: v.VolumeTemplate.ReuseKey}
		}
		out = append(out, item)
	}
	return out
}

func toolInfoFrom(value controlplane.Tool) ToolInfo {
	out := ToolInfo{ID: value.ID, Name: value.Name, Type: ToolType(value.Type), State: ToolState(value.Status), Description: value.Description, Persistent: value.Persistent, DefaultTimeoutSeconds: value.DefaultTimeoutSeconds, Network: toolNetworkFrom(value.Network), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RoleARN: value.RoleARN, StatusReason: value.StatusReason, CustomConfiguration: toolCustomFrom(value.Custom), ComputerConfiguration: toolComputerFrom(value.Computer), LogConfiguration: toolLogFrom(value.Log)}
	for _, tag := range value.Tags {
		out.Tags = append(out.Tags, ToolTag{Key: tag.Key, Value: tag.Value})
	}
	out.StorageMounts = toolMountsFrom(value.StorageMounts)
	return out
}
func toolNetworkFrom(value controlplane.ToolNetwork) ToolNetworkConfiguration {
	out := ToolNetworkConfiguration{Mode: ToolNetworkMode(value.Mode)}
	if value.VPC != nil {
		out.VPC = &ToolVPCConfiguration{SubnetIDs: append([]string(nil), value.VPC.SubnetIDs...), SecurityGroupIDs: append([]string(nil), value.VPC.SecurityGroupIDs...)}
	}
	return out
}
func toolCustomFrom(value *controlplane.ToolCustom) *ToolCustomConfigurationInfo {
	if value == nil {
		return nil
	}
	out := &ToolCustomConfigurationInfo{ToolCustomConfiguration: ToolCustomConfiguration{Image: value.Image, ImageRegistryType: ImageRegistryType(value.RegistryType), Command: append([]string(nil), value.Command...), Args: append([]string(nil), value.Args...)}, ImageDigest: value.ImageDigest}
	for _, v := range value.Env {
		out.Env = append(out.Env, ToolEnvironmentVariable{Name: v.Name, Value: v.Value})
	}
	for _, v := range value.Ports {
		out.Ports = append(out.Ports, ToolPortConfiguration{Name: v.Name, Protocol: v.Protocol, Port: v.Port})
	}
	if value.Resources != nil {
		out.Resources = &ToolResourceConfiguration{CPU: value.Resources.CPU, Memory: value.Resources.Memory, Storage: value.Resources.Storage}
	}
	if value.Probe != nil {
		out.Probe = &ToolProbeConfiguration{ReadyTimeoutMS: value.Probe.ReadyTimeoutMS, ProbeTimeoutMS: value.Probe.ProbeTimeoutMS, ProbePeriodMS: value.Probe.ProbePeriodMS, SuccessThreshold: value.Probe.SuccessThreshold, FailureThreshold: value.Probe.FailureThreshold}
		if value.Probe.HTTPGet != nil {
			out.Probe.HTTPGet = &ToolHTTPGetAction{Path: value.Probe.HTTPGet.Path, Scheme: value.Probe.HTTPGet.Scheme, Port: value.Probe.HTTPGet.Port}
		}
	}
	if value.DNS != nil {
		out.DNS = &ToolDNSConfiguration{Servers: append([]string(nil), value.DNS.Servers...), Searches: append([]string(nil), value.DNS.Searches...), Options: append([]string(nil), value.DNS.Options...)}
	}
	return out
}
func toolComputerFrom(value *controlplane.ToolComputer) *ToolComputerConfiguration {
	if value == nil {
		return nil
	}
	return &ToolComputerConfiguration{WAAImageID: value.WAAImageID, OSWorldVersion: value.OSWorldVersion}
}
func toolLogFrom(value *controlplane.ToolLog) *ToolLogConfiguration {
	if value == nil {
		return nil
	}
	return &ToolLogConfiguration{CLSTopicID: value.CLSTopicID, Files: append([]string(nil), value.Files...)}
}
func toolMountsFrom(values []controlplane.ToolStorageMount) []ToolStorageMount {
	out := make([]ToolStorageMount, 0, len(values))
	for _, v := range values {
		item := ToolStorageMount{Name: v.Name, MountPath: v.MountPath}
		if v.ReadOnly != nil {
			b := *v.ReadOnly
			item.ReadOnly = &b
		}
		if v.Source != nil {
			source := &ToolStorageSource{}
			if v.Source.COS != nil {
				source.COS = &ToolCOSStorage{Endpoint: v.Source.COS.Endpoint, BucketName: v.Source.COS.BucketName, BucketPath: v.Source.COS.BucketPath}
			}
			if v.Source.CFS != nil {
				source.CFS = &ToolCFSStorage{FileSystemID: v.Source.CFS.FileSystemID, Path: v.Source.CFS.Path}
			}
			if v.Source.Image != nil {
				source.Image = &ToolImageStorage{Reference: v.Source.Image.Reference, ImageRegistryType: v.Source.Image.RegistryType, SubPath: v.Source.Image.SubPath, Digest: v.Source.Image.Digest}
			}
			if v.Source.AgentBucket != nil {
				source.AgentBucket = &ToolAgentBucketStorage{LibraryID: v.Source.AgentBucket.LibraryID, SpaceID: v.Source.AgentBucket.SpaceID, AccessDomain: v.Source.AgentBucket.AccessDomain}
			}
			item.Source = source
		}
		if v.Volume != nil {
			item.Volume = &ToolVolumeRef{ID: v.Volume.ID, Name: v.Volume.Name}
		}
		if v.VolumeTemplate != nil {
			item.VolumeTemplate = &ToolVolumeTemplateRef{Name: v.VolumeTemplate.Name, ReuseKey: v.VolumeTemplate.ReuseKey}
		}
		out = append(out, item)
	}
	return out
}
func preCacheRefFrom(value controlplane.PreCacheImageRef) PreCacheImageRef {
	return PreCacheImageRef{Image: value.Image, Digest: value.Digest, RegistryType: ImageRegistryType(value.RegistryType)}
}

func mapMutationError(id string, err error) error {
	var mutation *controlplane.MutationError
	if !errors.As(err, &mutation) {
		return updateFailure(id, MutationUnknown, MutationSubmit, normalizeError("Sandbox.Update", err))
	}
	return updateFailure(id, MutationState(mutation.State), MutationPhase(mutation.Phase), normalizeError("Sandbox.Update", mutation.Cause))
}

func (c *controlAdapter) Query(ctx context.Context, q monitorRequest) (monitorResponse, error) {
	out, err := c.service.QueryMetric(ctx, controlplane.MetricRequest{Name: string(q.Metric), InstanceID: q.InstanceID, ToolID: q.ToolID, Start: q.Start, End: q.End, Period: q.Period})
	if err != nil {
		return monitorResponse{}, normalizeError("", err)
	}
	points := make([]MetricPoint, 0, len(out.Points))
	for _, point := range out.Points {
		points = append(points, MetricPoint{Timestamp: point.Timestamp, Value: point.Value})
	}
	resolution := q.Period
	if out.Resolution > 0 {
		resolution = out.Resolution
	}
	truncated := len(points) >= 1440 && q.End.Sub(q.Start) > resolution*time.Duration(len(points))
	return monitorResponse{Points: points, Resolution: resolution, Truncated: truncated, RequestID: out.RequestID}, nil
}

func sandboxInfoFrom(value controlplane.Sandbox) SandboxInfo {
	return SandboxInfo{ID: value.ID, ToolID: value.ToolID, ToolName: value.ToolName, State: SandboxState(value.State), CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt, Metadata: value.Metadata}
}

func instanceFrom(value controlplane.Instance) SandboxInfo {
	return sandboxInfoFrom(controlplane.SandboxForTest(value))
}

type monitorAdapter struct{ *controlAdapter }

func newTencentMonitor(region string, credential CredentialProvider, client *http.Client) *monitorAdapter {
	return newTencentMonitorWithConfig(region, credential, "monitor.tencentcloudapi.com", client, 30*time.Second)
}

func newTencentMonitorWithConfig(region string, credential CredentialProvider, endpoint string, client *http.Client, timeout time.Duration) *monitorAdapter {
	return &monitorAdapter{newControlAdapter(region, credential, "ags.tencentcloudapi.com", endpoint, client, timeout)}
}

func (m *monitorAdapter) Query(ctx context.Context, q monitorRequest) (monitorResponse, error) {
	return m.controlAdapter.Query(ctx, q)
}

func copyListOptions(opts SandboxListOptions) SandboxListOptions {
	opts.States = append([]SandboxState(nil), opts.States...)
	metadata := make(map[string][]string, len(opts.Metadata))
	for key, values := range opts.Metadata {
		metadata[key] = append([]string(nil), values...)
	}
	opts.Metadata = metadata
	return opts
}

func copyPageOptions(opts SandboxPageOptions) SandboxPageOptions {
	opts.Filter.InstanceIDs = append([]string(nil), opts.Filter.InstanceIDs...)
	opts.Filter.States = append([]SandboxState(nil), opts.Filter.States...)
	metadata := make(map[string][]string, len(opts.Filter.Metadata))
	for key, values := range opts.Filter.Metadata {
		metadata[key] = append([]string(nil), values...)
	}
	opts.Filter.Metadata = metadata
	return opts
}

func controlCursorListRequest(opts SandboxPageOptions) (controlplane.CursorListRequest, error) {
	legacy := SandboxListOptions{ToolID: opts.Filter.ToolID, States: opts.Filter.States, Metadata: opts.Filter.Metadata}
	filters, err := controlListRequest(legacy)
	if err != nil {
		if typed, ok := err.(*Error); ok {
			typed.Operation = "Sandboxes.ListPage"
		}
		return controlplane.CursorListRequest{}, err
	}
	return controlplane.CursorListRequest{InstanceIDs: opts.Filter.InstanceIDs, ToolID: filters.ToolID, MaxResults: opts.MaxResults, NextToken: opts.NextToken, NeedTotalCount: opts.NeedTotalCount, States: filters.States, Metadata: filters.Metadata}, nil
}

func controlListRequest(opts SandboxListOptions) (controlplane.ListRequest, error) {
	in := controlplane.ListRequest{ToolID: opts.ToolID, Offset: opts.Offset, Limit: opts.Limit, Metadata: opts.Metadata}
	for _, state := range opts.States {
		var internal controlplane.State
		switch state {
		case Creating:
			internal = controlplane.StateCreating
		case Running, Pausing, Paused, Stopping, Stopped, Failed:
			internal = controlplane.State(state)
		case Resuming:
			return controlplane.ListRequest{}, codeError(Unsupported, "Sandboxes.List", "LIST_STATE_UNAVAILABLE")
		default:
			return controlplane.ListRequest{}, codeError(InvalidArgument, "Sandboxes.List", "LIST_STATE_INVALID")
		}
		in.States = append(in.States, internal)
	}
	if len(opts.Metadata) > 5 {
		return controlplane.ListRequest{}, codeError(InvalidArgument, "Sandboxes.List", "LIST_METADATA_INVALID")
	}
	for key, values := range opts.Metadata {
		if key == "" || strings.TrimSpace(key) != key || strings.ContainsRune(key, 0) || len(values) == 0 {
			return controlplane.ListRequest{}, codeError(InvalidArgument, "Sandboxes.List", "LIST_METADATA_INVALID")
		}
	}
	for _, values := range opts.Metadata {
		if len(values) > 1 {
			return controlplane.ListRequest{}, codeError(Unsupported, "Sandboxes.List", "LIST_METADATA_OR_UNAVAILABLE")
		}
	}
	return in, nil
}
