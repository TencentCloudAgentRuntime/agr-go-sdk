package agr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	return newControlAdapter(region, credential, endpoint, "monitor.tencentcloudapi.com", client, timeout, region+".tencentags.com")
}

func newControlAdapter(region string, credential CredentialProvider, controlEndpoint, monitorEndpoint string, client *http.Client, timeout time.Duration, sandboxDomain string) *controlAdapter {
	source := func(ctx context.Context) (controlplane.Credential, error) {
		value, err := retrieveCloudCredential(ctx, credential)
		return controlplane.Credential{SecretID: value.SecretID, SecretKey: value.SecretKey, Token: value.Token}, err
	}
	return &controlAdapter{service: controlplane.NewService(controlplane.ServiceConfig{Region: region, ControlEndpoint: controlEndpoint, MonitorEndpoint: monitorEndpoint, SandboxDomain: sandboxDomain, Timeout: timeout, HTTPClient: client, Credential: source}), endpoint: controlEndpoint}
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

func (c *controlAdapter) UpdateVolume(ctx context.Context, target volumeTarget, opts UpdateVolumeOptions) (VolumeMutationResult, error) {
	out, err := c.service.UpdateVolume(ctx, controlVolumeTarget(target), controlUpdateVolume(opts))
	if err != nil {
		return VolumeMutationResult{}, withResourceError(normalizeError("Volumes.Update", err), "volume", volumeTargetLabel(target))
	}
	return VolumeMutationResult{VolumeID: out.ResourceID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DeleteVolume(ctx context.Context, target volumeTarget) error {
	return withResourceError(normalizeError("Volumes.Delete", c.service.DeleteVolume(ctx, controlVolumeTarget(target))), "volume", volumeTargetLabel(target))
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

func (c *controlAdapter) UpdateVolumeTemplate(ctx context.Context, target volumeTarget, opts UpdateVolumeTemplateOptions) (VolumeTemplateMutationResult, error) {
	out, err := c.service.UpdateVolumeTemplate(ctx, controlVolumeTarget(target), controlUpdateVolumeTemplate(opts))
	if err != nil {
		return VolumeTemplateMutationResult{}, withResourceError(normalizeError("VolumeTemplates.Update", err), "volume_template", volumeTargetLabel(target))
	}
	return VolumeTemplateMutationResult{VolumeTemplateID: out.ResourceID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DeleteVolumeTemplate(ctx context.Context, target volumeTarget) error {
	return withResourceError(normalizeError("VolumeTemplates.Delete", c.service.DeleteVolumeTemplate(ctx, controlVolumeTarget(target))), "volume_template", volumeTargetLabel(target))
}

func (c *controlAdapter) CreateSessionSpace(ctx context.Context, opts CreateSessionSpaceOptions) (SessionSpaceInfo, error) {
	tags := make([]controlplane.VolumeTag, len(opts.Tags))
	for i, tag := range opts.Tags {
		tags[i] = controlplane.VolumeTag{Key: tag.Key, Value: tag.Value}
	}
	out, err := c.service.CreateSessionSpace(ctx, controlplane.CreateSessionSpaceRequest{Name: opts.Name, Description: opts.Description, Tags: tags})
	if err != nil {
		return SessionSpaceInfo{}, normalizeError("SessionSpaces.Create", err)
	}
	return sessionSpaceInfoFrom(out), nil
}

func (c *controlAdapter) GetSessionSpace(ctx context.Context, id string) (SessionSpaceInfo, error) {
	out, err := c.service.GetSessionSpace(ctx, id)
	if err != nil {
		return SessionSpaceInfo{}, withResourceError(normalizeError("SessionSpaces.Get", err), "session_space", id)
	}
	return sessionSpaceInfoFrom(out), nil
}

func (c *controlAdapter) ListSessionSpaces(ctx context.Context, opts SessionSpaceListOptions) (SessionSpacePage, error) {
	in := controlplane.SessionSpaceListRequest{Offset: opts.Offset, Limit: opts.Limit, Filters: controlSessionFilters(opts.Filters)}
	out, err := c.service.ListSessionSpaces(ctx, in)
	if err != nil {
		return SessionSpacePage{}, normalizeError("SessionSpaces.List", err)
	}
	page := SessionSpacePage{Items: make([]SessionSpaceInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, sessionSpaceInfoFrom(controlplane.SessionSpaceResult{Space: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateSessionSpace(ctx context.Context, id string, opts UpdateSessionSpaceOptions) (SessionSpaceInfo, error) {
	var description *string
	if opts.Description != nil {
		value := *opts.Description
		description = &value
	}
	out, err := c.service.UpdateSessionSpace(ctx, id, controlplane.UpdateSessionSpaceRequest{Name: opts.Name, Description: description})
	if err != nil {
		return SessionSpaceInfo{}, withResourceError(normalizeError("SessionSpaces.Update", err), "session_space", id)
	}
	return sessionSpaceInfoFrom(out), nil
}

func (c *controlAdapter) DeleteSessionSpace(ctx context.Context, id string) error {
	return withResourceError(normalizeError("SessionSpaces.Delete", c.service.DeleteSessionSpace(ctx, id)), "session_space", id)
}

func (c *controlAdapter) CreateSession(ctx context.Context, opts CreateSessionOptions) (SessionInfo, error) {
	out, err := c.service.CreateSession(ctx, controlplane.CreateSessionRequest{Ref: controlSessionRef(opts.Ref), Title: opts.Title, State: controlSessionState(opts.State), Metadata: controlSessionMetadata(opts.Metadata)})
	if err != nil {
		return SessionInfo{}, normalizeError("Sessions.Create", err)
	}
	return sessionInfoFrom(out), nil
}

func (c *controlAdapter) GetSession(ctx context.Context, ref SessionRef) (SessionInfo, error) {
	out, err := c.service.GetSession(ctx, controlSessionRef(ref))
	if err != nil {
		return SessionInfo{}, withResourceError(normalizeError("Sessions.Get", err), "session", ref.SessionID)
	}
	return sessionInfoFrom(out), nil
}

func (c *controlAdapter) ListSessions(ctx context.Context, opts SessionListOptions) (SessionPage, error) {
	in := controlplane.SessionListRequest{SpaceID: opts.SpaceID, UserIDs: append([]string(nil), opts.UserIDs...), SessionIDs: append([]string(nil), opts.SessionIDs...), Filters: controlSessionFilters(opts.Filters), Offset: opts.Offset, Limit: opts.Limit}
	out, err := c.service.ListSessions(ctx, in)
	if err != nil {
		return SessionPage{}, normalizeError("Sessions.List", err)
	}
	page := SessionPage{Items: make([]SessionInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, sessionInfoFrom(controlplane.SessionResult{Session: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateSession(ctx context.Context, ref SessionRef, opts UpdateSessionOptions) (SessionInfo, error) {
	in := controlplane.UpdateSessionRequest{Title: opts.Title}
	if opts.Metadata != nil {
		values := controlSessionMetadata(*opts.Metadata)
		in.Metadata = &values
	}
	out, err := c.service.UpdateSession(ctx, controlSessionRef(ref), in)
	if err != nil {
		return SessionInfo{}, withResourceError(normalizeError("Sessions.Update", err), "session", ref.SessionID)
	}
	return sessionInfoFrom(out), nil
}

func (c *controlAdapter) DeleteSession(ctx context.Context, ref SessionRef) error {
	return withResourceError(normalizeError("Sessions.Delete", c.service.DeleteSession(ctx, controlSessionRef(ref))), "session", ref.SessionID)
}

func (c *controlAdapter) AppendEvent(ctx context.Context, ref SessionRef, event EventInput) (EventInfo, error) {
	out, err := c.service.AppendEvent(ctx, controlSessionRef(ref), controlEvent(event))
	if err != nil {
		return EventInfo{}, withResourceError(normalizeError("Sessions.AppendEvent", err), "session", ref.SessionID)
	}
	return eventInfoFrom(out), nil
}

func (c *controlAdapter) ListEvents(ctx context.Context, ref SessionRef, opts EventListOptions) (EventPage, error) {
	in := controlplane.EventListRequest{Ref: controlSessionRef(ref), Author: opts.Author, AfterTimestamp: opts.AfterTimestamp, Offset: opts.Offset, Limit: opts.Limit}
	out, err := c.service.ListEvents(ctx, in)
	if err != nil {
		return EventPage{}, withResourceError(normalizeError("Sessions.ListEvents", err), "session", ref.SessionID)
	}
	page := EventPage{Items: make([]EventInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, eventInfoFrom(controlplane.EventResult{Event: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func controlSessionFilters(values []SessionFilter) []controlplane.SessionFilter {
	out := make([]controlplane.SessionFilter, len(values))
	for i, value := range values {
		out[i] = controlplane.SessionFilter{Name: value.Name, Values: append([]string(nil), value.Values...)}
	}
	return out
}
func controlSessionMetadata(values []SessionMetadata) []controlplane.SessionMetadata {
	out := make([]controlplane.SessionMetadata, len(values))
	for i, value := range values {
		out[i] = controlplane.SessionMetadata{Name: value.Name, Value: value.Value}
	}
	return out
}
func controlSessionRef(value SessionRef) controlplane.SessionRef {
	return controlplane.SessionRef{SpaceID: value.SpaceID, UserID: value.UserID, SessionID: value.SessionID}
}
func controlSessionState(value SessionState) controlplane.SessionState {
	return controlplane.SessionState{CustomState: append(json.RawMessage(nil), value.CustomState...)}
}
func sessionSpaceInfoFrom(value controlplane.SessionSpaceResult) SessionSpaceInfo {
	return SessionSpaceInfo{ID: value.Space.ID, Name: value.Space.Name, Description: value.Space.Description, Status: SessionSpaceStatus(value.Space.Status), Default: value.Space.Default, CreatedAt: value.Space.CreatedAt, UpdatedAt: value.Space.UpdatedAt, RequestID: value.RequestID}
}
func sessionInfoFrom(value controlplane.SessionResult) SessionInfo {
	out := SessionInfo{Ref: SessionRef{SpaceID: value.Session.Ref.SpaceID, UserID: value.Session.Ref.UserID, SessionID: value.Session.Ref.SessionID}, Title: value.Session.Title, State: SessionState{CustomState: append(json.RawMessage(nil), value.Session.State.CustomState...)}, EventCount: value.Session.EventCount, CreatedAt: value.Session.CreatedAt, UpdatedAt: value.Session.UpdatedAt, RequestID: value.RequestID, Metadata: make([]SessionMetadata, len(value.Session.Metadata))}
	for i, item := range value.Session.Metadata {
		out.Metadata[i] = SessionMetadata{Name: item.Name, Value: item.Value}
	}
	return out
}
func controlEvent(value EventInput) controlplane.Event {
	out := controlplane.Event{ID: value.ID, InvocationID: value.InvocationID, Author: value.Author, Metadata: append(json.RawMessage(nil), value.Metadata...), Extensions: append(json.RawMessage(nil), value.Extensions...), ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage}
	if value.Actions != nil {
		out.Actions = &controlplane.EventActions{StateDelta: append(json.RawMessage(nil), value.Actions.StateDelta...)}
	}
	if value.Content != nil {
		out.Content = &controlplane.EventContent{Role: value.Content.Role, Parts: make([]controlplane.EventPart, len(value.Content.Parts))}
		for i, part := range value.Content.Parts {
			out.Content.Parts[i] = controlplane.EventPart{Text: part.Text, Thought: part.Thought, FunctionCall: append(json.RawMessage(nil), part.FunctionCall...), FunctionResponse: append(json.RawMessage(nil), part.FunctionResponse...)}
			if part.InlineData != nil {
				out.Content.Parts[i].InlineData = &controlplane.InlineData{MIMEType: part.InlineData.MIMEType, Data: part.InlineData.Data}
			}
		}
	}
	return out
}
func eventInfoFrom(value controlplane.EventResult) EventInfo {
	event := EventInput{ID: value.Event.ID, InvocationID: value.Event.InvocationID, Author: value.Event.Author, Metadata: append(json.RawMessage(nil), value.Event.Metadata...), Extensions: append(json.RawMessage(nil), value.Event.Extensions...), ErrorCode: value.Event.ErrorCode, ErrorMessage: value.Event.ErrorMessage}
	if value.Event.Actions != nil {
		event.Actions = &EventActions{StateDelta: append(json.RawMessage(nil), value.Event.Actions.StateDelta...)}
	}
	if value.Event.Content != nil {
		event.Content = &EventContent{Role: value.Event.Content.Role, Parts: make([]EventPart, len(value.Event.Content.Parts))}
		for i, part := range value.Event.Content.Parts {
			event.Content.Parts[i] = EventPart{Text: part.Text, Thought: part.Thought, FunctionCall: append(json.RawMessage(nil), part.FunctionCall...), FunctionResponse: append(json.RawMessage(nil), part.FunctionResponse...)}
			if part.InlineData != nil {
				event.Content.Parts[i].InlineData = &EventInlineData{MIMEType: part.InlineData.MIMEType, Data: part.InlineData.Data}
			}
		}
	}
	return EventInfo{EventInput: event, Timestamp: value.Event.Timestamp, RequestID: value.RequestID}
}

func (c *controlAdapter) CreateRegistry(ctx context.Context, opts CreateRegistryOptions) (RegistryInfo, error) {
	out, err := c.service.CreateRegistry(ctx, controlplane.CreateRegistryRequest{Name: opts.Name, Description: opts.Description, ApprovalMode: string(opts.ApprovalMode), Tags: controlRegistryTags(opts.Tags)})
	if err != nil {
		return RegistryInfo{}, normalizeError("Registries.Create", err)
	}
	return registryInfoFrom(out), nil
}

func (c *controlAdapter) GetRegistry(ctx context.Context, id string) (RegistryInfo, error) {
	out, err := c.service.GetRegistry(ctx, id)
	if err != nil {
		return RegistryInfo{}, withResourceError(normalizeError("Registries.Get", err), "registry", id)
	}
	return registryInfoFrom(out), nil
}

func (c *controlAdapter) ListRegistries(ctx context.Context, opts RegistryListOptions) (RegistryPage, error) {
	out, err := c.service.ListRegistries(ctx, controlplane.RegistryListRequest{Offset: opts.Offset, Limit: opts.Limit, Filters: controlRegistryFilters(opts.Filters)})
	if err != nil {
		return RegistryPage{}, normalizeError("Registries.List", err)
	}
	page := RegistryPage{Items: make([]RegistryInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, registryInfoFrom(controlplane.RegistryResult{Registry: item}))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateRegistry(ctx context.Context, id string, opts UpdateRegistryOptions) (RegistryInfo, error) {
	out, err := c.service.UpdateRegistry(ctx, id, controlplane.UpdateRegistryRequest{Description: opts.Description})
	if err != nil {
		return RegistryInfo{}, withResourceError(normalizeError("Registries.Update", err), "registry", id)
	}
	return registryInfoFrom(out), nil
}

func (c *controlAdapter) DeleteRegistry(ctx context.Context, id string) error {
	return withResourceError(normalizeError("Registries.Delete", c.service.DeleteRegistry(ctx, id)), "registry", id)
}

func (c *controlAdapter) CreateRegistryRecord(ctx context.Context, registryID string, opts CreateRegistryRecordOptions) (RegistryRecordResult, error) {
	in := controlplane.CreateRegistryRecordRequest{Name: opts.Name, Description: opts.Description, VersionName: opts.VersionName, DescriptorType: string(opts.DescriptorType), MCPSource: controlRegistryMCPSource(opts.MCPSource), AgentSource: controlRegistryAgentSource(opts.AgentSource), SkillSource: controlRegistrySkillSource(opts.SkillSource), CustomDescriptors: append(json.RawMessage(nil), opts.CustomDescriptors...)}
	out, err := c.service.CreateRegistryRecord(ctx, registryID, in)
	if err != nil {
		return RegistryRecordResult{}, withResourceError(normalizeError("Registries.Records.Create", err), "registry", registryID)
	}
	return registryRecordResultFrom(out), nil
}

func (c *controlAdapter) GetRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistryVersionSelector) (RegistryRecordResult, error) {
	out, err := c.service.GetRegistryRecord(ctx, registryID, recordID, controlRegistrySelector(selector))
	if err != nil {
		return RegistryRecordResult{}, withResourceError(normalizeError("Registries.Records.Get", err), "registry_record", recordID)
	}
	return registryRecordResultFrom(out), nil
}

func (c *controlAdapter) ListRegistryRecords(ctx context.Context, registryID string, opts RegistryRecordListOptions) (RegistryRecordPage, error) {
	out, err := c.service.ListRegistryRecords(ctx, registryID, controlplane.RegistryRecordListRequest{Offset: opts.Offset, Limit: opts.Limit, Filters: controlRegistryFilters(opts.Filters)})
	if err != nil {
		return RegistryRecordPage{}, withResourceError(normalizeError("Registries.Records.List", err), "registry", registryID)
	}
	page := RegistryRecordPage{Items: make([]RegistryRecordInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, registryRecordInfoFrom(item, ""))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) UpdateRegistryRecord(ctx context.Context, registryID, recordID string, opts UpdateRegistryRecordOptions) (RegistryRecordResult, error) {
	in := controlplane.UpdateRegistryRecordRequest{Description: opts.Description, VersionName: opts.VersionName, ChangeLog: opts.ChangeLog, MCPSource: controlRegistryMCPSource(opts.MCPSource), AgentSource: controlRegistryAgentSource(opts.AgentSource), SkillSource: controlRegistrySkillSource(opts.SkillSource), CustomDescriptors: append(json.RawMessage(nil), opts.CustomDescriptors...)}
	for _, value := range opts.LabelMutations {
		in.LabelMutations = append(in.LabelMutations, controlplane.RegistryLabelMutation{Operation: string(value.Operation), Name: value.Name, VersionID: value.VersionID, Reason: value.Reason})
	}
	out, err := c.service.UpdateRegistryRecord(ctx, registryID, recordID, in)
	if err != nil {
		return RegistryRecordResult{}, withResourceError(normalizeError("Registries.Records.Update", err), "registry_record", recordID)
	}
	return registryRecordResultFrom(out), nil
}

func (c *controlAdapter) DeleteRegistryRecord(ctx context.Context, registryID, recordID string, opts DeleteRegistryRecordOptions) error {
	in := controlplane.DeleteRegistryRecordRequest{Reason: opts.Reason}
	if opts.Version != nil {
		value := opts.Version.VersionID
		in.VersionID = &value
	}
	return withResourceError(normalizeError("Registries.Records.Delete", c.service.DeleteRegistryRecord(ctx, registryID, recordID, in)), "registry_record", recordID)
}

func (c *controlAdapter) ListRegistryRecordVersions(ctx context.Context, registryID, recordID string, opts RegistryVersionListOptions) (RegistryRecordVersionPage, error) {
	out, err := c.service.ListRegistryRecordVersions(ctx, registryID, recordID, controlplane.RegistryRecordListRequest{Offset: opts.Offset, Limit: opts.Limit, Filters: controlRegistryFilters(opts.Filters)})
	if err != nil {
		return RegistryRecordVersionPage{}, withResourceError(normalizeError("Registries.Records.ListVersions", err), "registry_record", recordID)
	}
	page := RegistryRecordVersionPage{Items: make([]RegistryRecordVersionInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, registryVersionInfoFrom(item, ""))
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func (c *controlAdapter) DecideRegistryRecord(ctx context.Context, decision, registryID, recordID, versionID, comment string) (RegistryRecordVersionInfo, error) {
	out, requestID, err := c.service.DecideRegistryRecord(ctx, decision, registryID, recordID, versionID, comment)
	if err != nil {
		return RegistryRecordVersionInfo{}, withResourceError(normalizeError("Registries.Records."+decision, err), "registry_record", recordID)
	}
	return registryVersionInfoFrom(out, requestID), nil
}

func (c *controlAdapter) PreviewRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistryVersionSelector) (RegistryPreviewInfo, error) {
	out, err := c.service.PreviewRegistryRecord(ctx, registryID, recordID, controlRegistrySelector(selector))
	if err != nil {
		return RegistryPreviewInfo{}, withResourceError(normalizeError("Registries.Records.Preview", err), "registry_record", recordID)
	}
	return RegistryPreviewInfo{Result: append(json.RawMessage(nil), out.Result...), ResolvedVersionID: out.ResolvedVersionID, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) SyncRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistryVersionSelector, changeLog string) (RegistrySyncResult, error) {
	out, err := c.service.SyncRegistryRecord(ctx, registryID, recordID, controlRegistrySelector(selector), changeLog)
	if err != nil {
		return RegistrySyncResult{}, withResourceError(normalizeError("Registries.Records.Sync", err), "registry_record", recordID)
	}
	result := RegistrySyncResult{Status: RegistrySyncStatus(out.Status), ResolvedVersionID: out.ResolvedVersionID, LastSyncAt: out.LastSyncAt, ErrorCode: out.ErrorCode, ErrorMessage: out.ErrorMessage, RequestID: out.RequestID}
	if out.CreatedVersion != nil {
		value := registryVersionInfoFrom(*out.CreatedVersion, out.RequestID)
		result.CreatedVersion = &value
	}
	if out.Record != nil {
		value := registryRecordInfoFrom(*out.Record, out.RequestID)
		result.Record = &value
	}
	return result, nil
}

func (c *controlAdapter) UploadRegistrySkillPackage(ctx context.Context, registryID, recordID, versionID string, body io.Reader, size, max int64) (RegistrySkillPackageUploadResult, error) {
	out, err := c.service.UploadRegistrySkillPackage(ctx, registryID, recordID, versionID, body, size, max)
	if err != nil {
		return RegistrySkillPackageUploadResult{}, withResourceError(normalizeError("Registries.Records.UploadSkillPackage", err), "registry_record", recordID)
	}
	return RegistrySkillPackageUploadResult{Version: registryVersionInfoFrom(out.Version, out.RequestID), ContentStatus: out.ContentStatus, SizeBytes: out.SizeBytes, SHA256: out.SHA256, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) DownloadRegistrySkillPackage(ctx context.Context, registryID, recordID string, selector *RegistryVersionSelector, dst io.Writer, max int64) (RegistrySkillPackageDownloadResult, error) {
	out, err := c.service.DownloadRegistrySkillPackage(ctx, registryID, recordID, controlRegistrySelector(selector), dst, max)
	if err != nil {
		return RegistrySkillPackageDownloadResult{}, withResourceError(normalizeError("Registries.Records.DownloadSkillPackage", err), "registry_record", recordID)
	}
	return RegistrySkillPackageDownloadResult{ResolvedVersionID: out.ResolvedVersionID, SizeBytes: out.SizeBytes, SHA256: out.SHA256, RequestID: out.RequestID}, nil
}

func (c *controlAdapter) ListRegistryAuditLogs(ctx context.Context, registryID string, opts RegistryAuditLogListOptions) (RegistryAuditLogPage, error) {
	in := controlplane.RegistryAuditLogRequest{RecordID: opts.RecordID, VersionID: opts.VersionID, Action: opts.Action, Actor: opts.Actor, Start: opts.Start, End: opts.End, Offset: opts.Offset, Limit: opts.Limit}
	out, err := c.service.ListRegistryAuditLogs(ctx, registryID, in)
	if err != nil {
		return RegistryAuditLogPage{}, withResourceError(normalizeError("Registries.Records.ListAuditLogs", err), "registry", registryID)
	}
	page := RegistryAuditLogPage{Items: make([]RegistryAuditLogInfo, 0, len(out.Items)), TotalCount: out.TotalCount, RequestID: out.RequestID}
	for _, item := range out.Items {
		page.Items = append(page.Items, RegistryAuditLogInfo{ID: item.ID, RegistryID: item.RegistryID, RecordID: item.RecordID, VersionID: item.VersionID, Actor: item.Actor, Action: item.Action, Details: append(json.RawMessage(nil), item.Details...), CreatedAt: item.CreatedAt})
	}
	next := opts.Offset + len(page.Items)
	if int64(next) < page.TotalCount {
		page.NextOffset = &next
	}
	return page, nil
}

func controlRegistryTags(values []RegistryTag) []controlplane.RegistryTag {
	out := make([]controlplane.RegistryTag, len(values))
	for i, v := range values {
		out[i] = controlplane.RegistryTag{Key: v.Key, Value: v.Value}
	}
	return out
}
func controlRegistryFilters(values []RegistryFilter) []controlplane.RegistryFilter {
	out := make([]controlplane.RegistryFilter, len(values))
	for i, v := range values {
		out[i] = controlplane.RegistryFilter{Name: v.Name, Values: append([]string(nil), v.Values...)}
	}
	return out
}
func controlRegistryMCPSource(value *RegistryMCPSource) *controlplane.RegistrySource {
	if value == nil {
		return nil
	}
	return &controlplane.RegistrySource{Type: string(value.Type), Descriptors: append(json.RawMessage(nil), value.Descriptors...), EndpointURL: value.EndpointURL}
}
func controlRegistryAgentSource(value *RegistryAgentSource) *controlplane.RegistrySource {
	if value == nil {
		return nil
	}
	return &controlplane.RegistrySource{Type: string(value.Type), Descriptors: append(json.RawMessage(nil), value.Descriptors...), EndpointURL: value.EndpointURL}
}
func controlRegistrySkillSource(value *RegistrySkillSource) *controlplane.RegistrySkillSource {
	if value == nil {
		return nil
	}
	return &controlplane.RegistrySkillSource{Type: string(value.Type), SkillMD: value.SkillMD}
}
func controlRegistrySelector(value *RegistryVersionSelector) *controlplane.RegistrySelector {
	if value == nil {
		return nil
	}
	return &controlplane.RegistrySelector{VersionID: value.VersionID, Label: value.Label}
}
func registryInfoFrom(value controlplane.RegistryResult) RegistryInfo {
	in := value.Registry
	out := RegistryInfo{ID: in.ID, Name: in.Name, Description: in.Description, Region: in.Region, ApprovalMode: RegistryApprovalMode(in.ApprovalMode), Status: RegistryStatus(in.Status), CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, RecordCount: in.RecordCount, PublishedRecordCount: in.PublishedRecordCount, AppID: in.AppID, CreatorUIN: in.CreatorUIN, CreatorSubUIN: in.CreatorSubUIN, Tags: make([]RegistryTag, len(in.Tags)), RequestID: value.RequestID}
	for i, tag := range in.Tags {
		out.Tags[i] = RegistryTag{Key: tag.Key, Value: tag.Value}
	}
	return out
}
func registryRecordInfoFrom(in controlplane.RegistryRecord, requestID string) RegistryRecordInfo {
	return RegistryRecordInfo{ID: in.ID, RegistryID: in.RegistryID, Name: in.Name, Description: in.Description, DescriptorType: RegistryDescriptorType(in.DescriptorType), Status: RegistryRecordStatus(in.Status), AppID: in.AppID, CreatorUIN: in.CreatorUIN, CreatorSubUIN: in.CreatorSubUIN, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, VersionCount: in.VersionCount, Labels: append([]string(nil), in.Labels...), RequestID: requestID}
}
func registryVersionInfoFrom(in controlplane.RegistryRecordVersion, requestID string) RegistryRecordVersionInfo {
	out := RegistryRecordVersionInfo{ID: in.ID, RecordID: in.RecordID, Revision: in.Revision, Status: RegistryVersionStatus(in.Status), ApprovalMode: RegistryApprovalMode(in.ApprovalMode), AppID: in.AppID, CreatorUIN: in.CreatorUIN, CreatorSubUIN: in.CreatorSubUIN, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt, Name: in.Name, Descriptors: append(json.RawMessage(nil), in.Descriptors...), SourceConfig: append(json.RawMessage(nil), in.SourceConfig...), SourceType: RegistrySourceType(in.SourceType), ContentStatus: in.ContentStatus, ContentSHA256: in.ContentSHA256, ContentSizeBytes: in.ContentSizeBytes, ConfigSHA256: in.ConfigSHA256, ChangeLog: in.ChangeLog, ContentReadyAt: in.ContentReadyAt, Labels: append([]string(nil), in.Labels...), RequestID: requestID, ApprovalActions: make([]RegistryApprovalAction, len(in.ApprovalActions))}
	for i, item := range in.ApprovalActions {
		out.ApprovalActions[i] = RegistryApprovalAction{ID: item.ID, Type: item.Type, ActorType: item.ActorType, ActorUIN: item.ActorUIN, ActorSubUIN: item.ActorSubUIN, Comment: item.Comment, RequestID: item.RequestID, CreatedAt: item.CreatedAt}
	}
	return out
}
func registryRecordResultFrom(value controlplane.RegistryRecordResult) RegistryRecordResult {
	out := RegistryRecordResult{Record: registryRecordInfoFrom(value.Record, value.RequestID), ResolvedBy: value.ResolvedBy, ResolvedLabel: value.ResolvedLabel, RequestID: value.RequestID}
	if value.Version != nil {
		version := registryVersionInfoFrom(*value.Version, value.RequestID)
		out.Version = &version
	}
	return out
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

func controlVolumeTarget(value volumeTarget) controlplane.VolumeTarget {
	return controlplane.VolumeTarget{ID: value.id, Name: value.name}
}

func volumeTargetLabel(value volumeTarget) string {
	if value.id != "" {
		return value.id
	}
	return value.name
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
	return &monitorAdapter{newControlAdapter(region, credential, "ags.tencentcloudapi.com", endpoint, client, timeout, region+".tencentags.com")}
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
