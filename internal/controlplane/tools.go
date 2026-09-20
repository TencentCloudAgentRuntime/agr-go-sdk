package controlplane

import (
	"context"
	"errors"
	"time"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"

	"github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/model"
)

type ToolNetwork struct {
	Mode string
	VPC  *ToolVPC
}
type ToolVPC struct{ SubnetIDs, SecurityGroupIDs []string }
type ToolTag struct{ Key, Value string }
type ToolEnv struct{ Name, Value string }
type ToolPort struct {
	Name, Protocol string
	Port           int64
}
type ToolResources struct{ CPU, Memory, Storage string }
type ToolHTTPGet struct {
	Path, Scheme string
	Port         int64
}
type ToolProbe struct {
	HTTPGet                                       *ToolHTTPGet
	ReadyTimeoutMS, ProbeTimeoutMS, ProbePeriodMS int64
	SuccessThreshold, FailureThreshold            int64
}
type ToolDNS struct{ Servers, Searches, Options []string }
type ToolCustom struct {
	Image, ImageDigest, RegistryType string
	Command, Args                    []string
	Env                              []ToolEnv
	Ports                            []ToolPort
	Resources                        *ToolResources
	Probe                            *ToolProbe
	DNS                              *ToolDNS
}
type ToolComputer struct{ WAAImageID, OSWorldVersion string }
type ToolLog struct {
	CLSTopicID string
	Files      []string
}
type ToolCOS struct{ Endpoint, BucketName, BucketPath string }
type ToolCFS struct{ FileSystemID, Path string }
type ToolImage struct{ Reference, RegistryType, SubPath, Digest string }
type ToolAgentBucket struct{ LibraryID, SpaceID, AccessDomain string }
type ToolStorageSource struct {
	COS         *ToolCOS
	CFS         *ToolCFS
	Image       *ToolImage
	AgentBucket *ToolAgentBucket
}
type ToolStorageMount struct {
	Name, MountPath string
	ReadOnly        *bool
	Source          *ToolStorageSource
}

type CreateToolRequest struct {
	Name, Type, Description, DefaultTimeout, ClientToken, RoleARN string
	Network                                                       ToolNetwork
	Tags                                                          []ToolTag
	StorageMounts                                                 []ToolStorageMount
	Custom                                                        *ToolCustom
	Computer                                                      *ToolComputer
	Log                                                           *ToolLog
	Persistent                                                    *bool
}
type UpdateToolRequest struct {
	Description *string
	Network     *ToolNetwork
	Tags        *[]ToolTag
	Custom      *ToolCustom
	Computer    *ToolComputer
}
type ToolFilter struct {
	Name   string
	Values []string
}
type ToolListRequest struct {
	ToolIDs       []string
	Offset, Limit int
	Filters       []ToolFilter
}
type ToolMutation struct{ ToolID, RequestID string }
type Tool struct {
	ID, Name, Type, Status, Description, RoleARN, StatusReason string
	Persistent                                                 bool
	DefaultTimeoutSeconds                                      uint64
	Network                                                    ToolNetwork
	Tags                                                       []ToolTag
	CreatedAt, UpdatedAt                                       time.Time
	StorageMounts                                              []ToolStorageMount
	Custom                                                     *ToolCustom
	Computer                                                   *ToolComputer
	Log                                                        *ToolLog
}
type ToolPage struct {
	Items      []Tool
	TotalCount int
}
type PreCacheImageRef struct{ Image, Digest, RegistryType string }
type PreCacheImage struct {
	Ref                        PreCacheImageRef
	Status, Message, RequestID string
}

type wireTool struct {
	ID, Name, Type, Status, Description, RoleARN, StatusReason, CreatedAt, UpdatedAt string
	Persistent                                                                       bool
	DefaultTimeoutSeconds                                                            uint64
	Network                                                                          ToolNetwork
	Tags                                                                             []ToolTag
	StorageMounts                                                                    []ToolStorageMount
	Custom                                                                           *ToolCustom
	Computer                                                                         *ToolComputer
	Log                                                                              *ToolLog
}

func (s *Service) CreateTool(ctx context.Context, request CreateToolRequest) (ToolMutation, error) {
	out, err := s.ags.CreateTool(ctx, request)
	return out, cloudError(err, "CreateSandboxTool")
}
func (s *Service) GetTool(ctx context.Context, id string) (Tool, error) {
	out, err := s.ags.ListTools(ctx, ToolListRequest{ToolIDs: []string{id}, Limit: 1})
	if err != nil {
		return Tool{}, cloudError(err, "DescribeSandboxToolList")
	}
	if len(out.Items) != 1 || out.Items[0].ID != id {
		return Tool{}, failure(model.NotFound, "Tools.Get", "TOOL_NOT_FOUND", false, nil)
	}
	return toolFromWire(out.Items[0]), nil
}
func (s *Service) ListTools(ctx context.Context, request ToolListRequest) (ToolPage, error) {
	out, err := s.ags.ListTools(ctx, request)
	if err != nil {
		return ToolPage{}, cloudError(err, "DescribeSandboxToolList")
	}
	page := ToolPage{TotalCount: out.TotalCount, Items: make([]Tool, 0, len(out.Items))}
	for _, item := range out.Items {
		page.Items = append(page.Items, toolFromWire(item))
	}
	return page, nil
}
func (s *Service) UpdateTool(ctx context.Context, id string, request UpdateToolRequest) (ToolMutation, error) {
	out, err := s.ags.UpdateTool(ctx, id, request)
	return out, cloudError(err, "UpdateSandboxTool")
}
func (s *Service) DeleteTool(ctx context.Context, id string) error {
	return cloudError(s.ags.DeleteTool(ctx, id), "DeleteSandboxTool")
}
func (s *Service) CreatePreCacheImage(ctx context.Context, image, registryType string) (PreCacheImage, error) {
	out, err := s.ags.CreatePreCacheImage(ctx, image, registryType)
	return out, cloudError(err, "CreatePreCacheImageTask")
}
func (s *Service) GetPreCacheImage(ctx context.Context, ref PreCacheImageRef) (PreCacheImage, error) {
	out, err := s.ags.GetPreCacheImage(ctx, ref)
	return out, cloudError(err, "DescribePreCacheImageTask")
}

func toolFromWire(value wireTool) Tool {
	created, _ := time.Parse(time.RFC3339, value.CreatedAt)
	updated, _ := time.Parse(time.RFC3339, value.UpdatedAt)
	return Tool{ID: value.ID, Name: value.Name, Type: value.Type, Status: value.Status, Description: value.Description, RoleARN: value.RoleARN, StatusReason: value.StatusReason, Persistent: value.Persistent, DefaultTimeoutSeconds: value.DefaultTimeoutSeconds, Network: value.Network, Tags: value.Tags, CreatedAt: created, UpdatedAt: updated, StorageMounts: value.StorageMounts, Custom: value.Custom, Computer: value.Computer, Log: value.Log}
}

func (a *AGS) CreateTool(ctx context.Context, in CreateToolRequest) (ToolMutation, error) {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return ToolMutation{}, err
	}
	defer cancel()
	req := ags.NewCreateSandboxToolRequest()
	req.ToolName, req.ToolType = stringPtr(in.Name), stringPtr(in.Type)
	req.NetworkConfiguration = toNetwork(in.Network)
	req.Description, req.DefaultTimeout, req.ClientToken, req.RoleArn = stringPtr(in.Description), stringPtr(in.DefaultTimeout), stringPtr(in.ClientToken), stringPtr(in.RoleARN)
	req.Tags, req.StorageMounts = toTags(in.Tags), toStorageMounts(in.StorageMounts)
	req.CustomConfiguration, req.ComputerConfiguration, req.LogConfiguration = toCustom(in.Custom), toComputer(in.Computer), toLog(in.Log)
	req.Persistent = in.Persistent
	response, err := client.CreateSandboxToolWithContext(callCtx, req)
	if err != nil {
		return ToolMutation{}, wrapCall(callCtx, err)
	}
	if response == nil || response.Response == nil || response.Response.ToolId == nil {
		return ToolMutation{}, &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("CreateSandboxTool response has no ToolId")}
	}
	return ToolMutation{ToolID: valueString(response.Response.ToolId), RequestID: valueString(response.Response.RequestId)}, nil
}

func (a *AGS) ListTools(ctx context.Context, in ToolListRequest) (struct {
	Items      []wireTool
	TotalCount int
}, error) {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return struct {
			Items      []wireTool
			TotalCount int
		}{}, err
	}
	defer cancel()
	req := ags.NewDescribeSandboxToolListRequest()
	for _, id := range in.ToolIDs {
		id := id
		req.ToolIds = append(req.ToolIds, &id)
	}
	offset, limit := int64(in.Offset), int64(in.Limit)
	req.Offset, req.Limit = &offset, &limit
	for _, filter := range in.Filters {
		name := filter.Name
		item := &ags.Filter{Name: &name}
		for _, value := range filter.Values {
			value := value
			item.Values = append(item.Values, &value)
		}
		req.Filters = append(req.Filters, item)
	}
	response, err := client.DescribeSandboxToolListWithContext(callCtx, req)
	if err != nil {
		return struct {
			Items      []wireTool
			TotalCount int
		}{}, wrapCall(callCtx, err)
	}
	if response == nil || response.Response == nil {
		return struct {
			Items      []wireTool
			TotalCount int
		}{}, &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("DescribeSandboxToolList response is empty")}
	}
	out := struct {
		Items      []wireTool
		TotalCount int
	}{TotalCount: int(valueInt64(response.Response.TotalCount))}
	for _, value := range response.Response.SandboxToolSet {
		if value != nil {
			out.Items = append(out.Items, fromTool(value))
		}
	}
	return out, nil
}

func (a *AGS) UpdateTool(ctx context.Context, id string, in UpdateToolRequest) (ToolMutation, error) {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return ToolMutation{}, err
	}
	defer cancel()
	req := ags.NewUpdateSandboxToolRequest()
	req.ToolId, req.Description = &id, in.Description
	if in.Network != nil {
		req.NetworkConfiguration = toNetwork(*in.Network)
	}
	if in.Tags != nil {
		req.Tags = toTags(*in.Tags)
	}
	req.CustomConfiguration, req.ComputerConfiguration = toCustom(in.Custom), toComputer(in.Computer)
	response, err := client.UpdateSandboxToolWithContext(callCtx, req)
	if err != nil {
		return ToolMutation{}, wrapCall(callCtx, err)
	}
	if response == nil || response.Response == nil {
		return ToolMutation{}, &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("UpdateSandboxTool response is empty")}
	}
	return ToolMutation{ToolID: id, RequestID: valueString(response.Response.RequestId)}, nil
}
func (a *AGS) DeleteTool(ctx context.Context, id string) error {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	req := ags.NewDeleteSandboxToolRequest()
	req.ToolId = &id
	_, err = client.DeleteSandboxToolWithContext(callCtx, req)
	return wrapCall(callCtx, err)
}
func (a *AGS) CreatePreCacheImage(ctx context.Context, image, registryType string) (PreCacheImage, error) {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return PreCacheImage{}, err
	}
	defer cancel()
	req := ags.NewCreatePreCacheImageTaskRequest()
	req.Image, req.ImageRegistryType = &image, &registryType
	response, err := client.CreatePreCacheImageTaskWithContext(callCtx, req)
	if err != nil {
		return PreCacheImage{}, wrapCall(callCtx, err)
	}
	if response == nil || response.Response == nil {
		return PreCacheImage{}, &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("CreatePreCacheImageTask response is empty")}
	}
	return PreCacheImage{Ref: PreCacheImageRef{Image: valueString(response.Response.Image), Digest: valueString(response.Response.ImageDigest), RegistryType: valueString(response.Response.ImageRegistryType)}, RequestID: valueString(response.Response.RequestId)}, nil
}
func (a *AGS) GetPreCacheImage(ctx context.Context, ref PreCacheImageRef) (PreCacheImage, error) {
	client, callCtx, cancel, err := a.client(ctx)
	if err != nil {
		return PreCacheImage{}, err
	}
	defer cancel()
	req := ags.NewDescribePreCacheImageTaskRequest()
	req.Image, req.ImageDigest, req.ImageRegistryType = &ref.Image, &ref.Digest, &ref.RegistryType
	response, err := client.DescribePreCacheImageTaskWithContext(callCtx, req)
	if err != nil {
		return PreCacheImage{}, wrapCall(callCtx, err)
	}
	if response == nil || response.Response == nil {
		return PreCacheImage{}, &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("DescribePreCacheImageTask response is empty")}
	}
	return PreCacheImage{Ref: PreCacheImageRef{Image: valueString(response.Response.Image), Digest: valueString(response.Response.ImageDigest), RegistryType: valueString(response.Response.ImageRegistryType)}, Status: valueString(response.Response.Status), Message: valueString(response.Response.Message), RequestID: valueString(response.Response.RequestId)}, nil
}

func toNetwork(value ToolNetwork) *ags.NetworkConfiguration {
	out := &ags.NetworkConfiguration{NetworkMode: stringPtr(value.Mode)}
	if value.VPC != nil {
		out.VpcConfig = &ags.VPCConfig{SubnetIds: stringPtrs(value.VPC.SubnetIDs), SecurityGroupIds: stringPtrs(value.VPC.SecurityGroupIDs)}
	}
	return out
}
func toTags(values []ToolTag) []*ags.Tag {
	out := make([]*ags.Tag, 0, len(values))
	for _, value := range values {
		key, item := value.Key, value.Value
		out = append(out, &ags.Tag{Key: &key, Value: &item})
	}
	return out
}
func toStorageMounts(values []ToolStorageMount) []*ags.StorageMount {
	out := make([]*ags.StorageMount, 0, len(values))
	for _, value := range values {
		out = append(out, &ags.StorageMount{Name: stringPtr(value.Name), MountPath: stringPtr(value.MountPath), ReadOnly: value.ReadOnly, StorageSource: toStorageSource(value.Source)})
	}
	return out
}
func toStorageSource(value *ToolStorageSource) *ags.StorageSource {
	if value == nil {
		return nil
	}
	out := &ags.StorageSource{}
	if value.COS != nil {
		out.Cos = &ags.CosStorageSource{Endpoint: stringPtr(value.COS.Endpoint), BucketName: stringPtr(value.COS.BucketName), BucketPath: stringPtr(value.COS.BucketPath)}
	}
	if value.CFS != nil {
		out.Cfs = &ags.CfsStorageSource{FileSystemId: stringPtr(value.CFS.FileSystemID), Path: stringPtr(value.CFS.Path)}
	}
	if value.Image != nil {
		out.Image = &ags.ImageStorageSource{Reference: stringPtr(value.Image.Reference), ImageRegistryType: stringPtr(value.Image.RegistryType), SubPath: stringPtr(value.Image.SubPath), Digest: stringPtr(value.Image.Digest)}
	}
	if value.AgentBucket != nil {
		out.AgentBucket = &ags.AgentBucketStorageSource{LibraryId: stringPtr(value.AgentBucket.LibraryID), SpaceId: stringPtr(value.AgentBucket.SpaceID), AccessDomain: stringPtr(value.AgentBucket.AccessDomain)}
	}
	return out
}
func toCustom(value *ToolCustom) *ags.CustomConfiguration {
	if value == nil {
		return nil
	}
	out := &ags.CustomConfiguration{Image: stringPtr(value.Image), ImageRegistryType: stringPtr(value.RegistryType), Command: stringPtrs(value.Command), Args: stringPtrs(value.Args), Resources: toResources(value.Resources), Probe: toProbe(value.Probe), DNSConfig: toDNS(value.DNS)}
	for _, v := range value.Env {
		name, item := v.Name, v.Value
		out.Env = append(out.Env, &ags.EnvVar{Name: &name, Value: &item})
	}
	for _, v := range value.Ports {
		name, protocol, port := v.Name, v.Protocol, v.Port
		out.Ports = append(out.Ports, &ags.PortConfiguration{Name: &name, Protocol: &protocol, Port: &port})
	}
	return out
}
func toResources(value *ToolResources) *ags.ResourceConfiguration {
	if value == nil {
		return nil
	}
	return &ags.ResourceConfiguration{CPU: stringPtr(value.CPU), Memory: stringPtr(value.Memory), Storage: stringPtr(value.Storage)}
}
func toProbe(value *ToolProbe) *ags.ProbeConfiguration {
	if value == nil {
		return nil
	}
	out := &ags.ProbeConfiguration{ReadyTimeoutMs: &value.ReadyTimeoutMS, ProbeTimeoutMs: &value.ProbeTimeoutMS, ProbePeriodMs: &value.ProbePeriodMS, SuccessThreshold: &value.SuccessThreshold, FailureThreshold: &value.FailureThreshold}
	if value.HTTPGet != nil {
		out.HttpGet = &ags.HttpGetAction{Path: stringPtr(value.HTTPGet.Path), Port: &value.HTTPGet.Port, Scheme: stringPtr(value.HTTPGet.Scheme)}
	}
	return out
}
func toDNS(value *ToolDNS) *ags.DNSConfig {
	if value == nil {
		return nil
	}
	return &ags.DNSConfig{Servers: stringPtrs(value.Servers), Searches: stringPtrs(value.Searches), Options: stringPtrs(value.Options)}
}
func toComputer(value *ToolComputer) *ags.ComputerConfiguration {
	if value == nil {
		return nil
	}
	out := &ags.ComputerConfiguration{}
	if value.WAAImageID != "" {
		out.WAAConfiguration = &ags.WAAConfiguration{ImageId: &value.WAAImageID}
	}
	if value.OSWorldVersion != "" {
		out.OSWorldConfiguration = &ags.OSWorldConfiguration{Version: &value.OSWorldVersion}
	}
	return out
}
func toLog(value *ToolLog) *ags.LogConfiguration {
	if value == nil {
		return nil
	}
	out := &ags.LogConfiguration{}
	if value.CLSTopicID != "" {
		out.CLSConfig = &ags.CLSConfig{TopicId: &value.CLSTopicID}
	}
	if value.Files != nil {
		out.LogSources = &ags.LogSources{Files: stringPtrs(value.Files)}
	}
	return out
}
func stringPtrs(values []string) []*string {
	out := make([]*string, 0, len(values))
	for _, value := range values {
		value := value
		out = append(out, &value)
	}
	return out
}

func fromTool(value *ags.SandboxTool) wireTool {
	out := wireTool{ID: valueString(value.ToolId), Name: valueString(value.ToolName), Type: valueString(value.ToolType), Status: valueString(value.Status), Description: valueString(value.Description), Persistent: value.Persistent != nil && *value.Persistent, DefaultTimeoutSeconds: valueUint64(value.DefaultTimeoutSeconds), Network: fromNetwork(value.NetworkConfiguration), Tags: fromTags(value.Tags), CreatedAt: valueString(value.CreateTime), UpdatedAt: valueString(value.UpdateTime), RoleARN: valueString(value.RoleArn), StorageMounts: fromStorageMounts(value.StorageMounts), Custom: fromCustom(value.CustomConfiguration), Computer: fromComputer(value.ComputerConfiguration), Log: fromLog(value.LogConfiguration), StatusReason: valueString(value.StatusReason)}
	return out
}
func fromNetwork(value *ags.NetworkConfiguration) ToolNetwork {
	if value == nil {
		return ToolNetwork{}
	}
	out := ToolNetwork{Mode: valueString(value.NetworkMode)}
	if value.VpcConfig != nil {
		out.VPC = &ToolVPC{SubnetIDs: fromStringPtrs(value.VpcConfig.SubnetIds), SecurityGroupIDs: fromStringPtrs(value.VpcConfig.SecurityGroupIds)}
	}
	return out
}
func fromTags(values []*ags.Tag) []ToolTag {
	out := make([]ToolTag, 0, len(values))
	for _, value := range values {
		if value != nil {
			out = append(out, ToolTag{Key: valueString(value.Key), Value: valueString(value.Value)})
		}
	}
	return out
}
func fromStorageMounts(values []*ags.StorageMount) []ToolStorageMount {
	out := make([]ToolStorageMount, 0, len(values))
	for _, value := range values {
		if value != nil {
			out = append(out, ToolStorageMount{Name: valueString(value.Name), MountPath: valueString(value.MountPath), ReadOnly: value.ReadOnly, Source: fromStorageSource(value.StorageSource)})
		}
	}
	return out
}
func fromStorageSource(value *ags.StorageSource) *ToolStorageSource {
	if value == nil {
		return nil
	}
	out := &ToolStorageSource{}
	if value.Cos != nil {
		out.COS = &ToolCOS{Endpoint: valueString(value.Cos.Endpoint), BucketName: valueString(value.Cos.BucketName), BucketPath: valueString(value.Cos.BucketPath)}
	}
	if value.Cfs != nil {
		out.CFS = &ToolCFS{FileSystemID: valueString(value.Cfs.FileSystemId), Path: valueString(value.Cfs.Path)}
	}
	if value.Image != nil {
		out.Image = &ToolImage{Reference: valueString(value.Image.Reference), RegistryType: valueString(value.Image.ImageRegistryType), SubPath: valueString(value.Image.SubPath), Digest: valueString(value.Image.Digest)}
	}
	if value.AgentBucket != nil {
		out.AgentBucket = &ToolAgentBucket{LibraryID: valueString(value.AgentBucket.LibraryId), SpaceID: valueString(value.AgentBucket.SpaceId), AccessDomain: valueString(value.AgentBucket.AccessDomain)}
	}
	return out
}
func fromCustom(value *ags.CustomConfigurationDetail) *ToolCustom {
	if value == nil {
		return nil
	}
	out := &ToolCustom{Image: valueString(value.Image), ImageDigest: valueString(value.ImageDigest), RegistryType: valueString(value.ImageRegistryType), Command: fromStringPtrs(value.Command), Args: fromStringPtrs(value.Args), Resources: fromResources(value.Resources), Probe: fromProbe(value.Probe), DNS: fromDNS(value.DNSConfig)}
	for _, v := range value.Env {
		if v != nil {
			out.Env = append(out.Env, ToolEnv{Name: valueString(v.Name), Value: valueString(v.Value)})
		}
	}
	for _, v := range value.Ports {
		if v != nil {
			out.Ports = append(out.Ports, ToolPort{Name: valueString(v.Name), Protocol: valueString(v.Protocol), Port: valueInt64(v.Port)})
		}
	}
	return out
}
func fromResources(value *ags.ResourceConfiguration) *ToolResources {
	if value == nil {
		return nil
	}
	return &ToolResources{CPU: valueString(value.CPU), Memory: valueString(value.Memory), Storage: valueString(value.Storage)}
}
func fromProbe(value *ags.ProbeConfiguration) *ToolProbe {
	if value == nil {
		return nil
	}
	out := &ToolProbe{ReadyTimeoutMS: valueInt64(value.ReadyTimeoutMs), ProbeTimeoutMS: valueInt64(value.ProbeTimeoutMs), ProbePeriodMS: valueInt64(value.ProbePeriodMs), SuccessThreshold: valueInt64(value.SuccessThreshold), FailureThreshold: valueInt64(value.FailureThreshold)}
	if value.HttpGet != nil {
		out.HTTPGet = &ToolHTTPGet{Path: valueString(value.HttpGet.Path), Port: valueInt64(value.HttpGet.Port), Scheme: valueString(value.HttpGet.Scheme)}
	}
	return out
}
func fromDNS(value *ags.DNSConfig) *ToolDNS {
	if value == nil {
		return nil
	}
	return &ToolDNS{Servers: fromStringPtrs(value.Servers), Searches: fromStringPtrs(value.Searches), Options: fromStringPtrs(value.Options)}
}
func fromComputer(value *ags.ComputerConfiguration) *ToolComputer {
	if value == nil {
		return nil
	}
	out := &ToolComputer{}
	if value.WAAConfiguration != nil {
		out.WAAImageID = valueString(value.WAAConfiguration.ImageId)
	}
	if value.OSWorldConfiguration != nil {
		out.OSWorldVersion = valueString(value.OSWorldConfiguration.Version)
	}
	return out
}
func fromLog(value *ags.LogConfiguration) *ToolLog {
	if value == nil {
		return nil
	}
	out := &ToolLog{}
	if value.CLSConfig != nil {
		out.CLSTopicID = valueString(value.CLSConfig.TopicId)
	}
	if value.LogSources != nil {
		out.Files = fromStringPtrs(value.LogSources.Files)
	}
	return out
}
func fromStringPtrs(values []*string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != nil {
			out = append(out, *value)
		}
	}
	return out
}
