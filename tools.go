package agr

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ToolManager manages sandbox tool definitions bound to one Client identity.
type ToolManager struct{ client *Client }

// PreCacheImageManager manages image pre-cache tasks bound to one Client identity.
type PreCacheImageManager struct{ client *Client }

// ToolType is the service tool category. The SDK preserves unknown values returned by the service.
type ToolType string

const (
	// ToolBrowser selects the managed browser runtime.
	ToolBrowser ToolType = "browser"
	// ToolCodeInterpreter selects the managed code-interpreter runtime.
	ToolCodeInterpreter ToolType = "code-interpreter"
	// ToolMobile selects the managed mobile runtime.
	ToolMobile ToolType = "mobile"
	// ToolOSWorld selects the OSWorld runtime.
	ToolOSWorld ToolType = "osworld"
	// ToolCustom selects a caller-configured image runtime.
	ToolCustom ToolType = "custom"
	// ToolSWEbench selects the SWE-bench runtime.
	ToolSWEbench ToolType = "swebench"
	// ToolAIO selects the all-in-one runtime.
	ToolAIO ToolType = "aio"
	// ToolAndroidWorld selects the AndroidWorld runtime.
	ToolAndroidWorld ToolType = "android-world"
	// ToolWAA selects the WAA runtime.
	ToolWAA ToolType = "waa"
)

// ToolState is the service lifecycle state for a Tool.
type ToolState string

const (
	// ToolCreating means creation is in progress.
	ToolCreating ToolState = "CREATING"
	// ToolActive means the Tool is available for Sandbox creation.
	ToolActive ToolState = "ACTIVE"
	// ToolDeleting means deletion is in progress.
	ToolDeleting ToolState = "DELETING"
	// ToolFailed means provisioning failed.
	ToolFailed ToolState = "FAILED"
	// ToolUnknown is an explicit unknown state; unrecognized service values remain intact.
	ToolUnknown ToolState = "UNKNOWN"
)

// ToolNetworkMode is the network attachment mode for tool-created sandboxes.
type ToolNetworkMode string

const (
	// ToolNetworkPublic uses public networking.
	ToolNetworkPublic ToolNetworkMode = "PUBLIC"
	// ToolNetworkVPC attaches configured VPC resources.
	ToolNetworkVPC ToolNetworkMode = "VPC"
	// ToolNetworkSandbox uses service-managed sandbox networking.
	ToolNetworkSandbox ToolNetworkMode = "SANDBOX"
)

// ImageRegistryType identifies an image registry supported by the stable SDK contract.
type ImageRegistryType string

const (
	// ImageRegistryEnterprise selects Tencent Cloud Enterprise TCR.
	ImageRegistryEnterprise ImageRegistryType = "enterprise"
	// ImageRegistryPersonal selects Tencent Cloud Personal CCR.
	ImageRegistryPersonal ImageRegistryType = "personal"
)

// ToolNetworkConfiguration configures Sandbox networking for one Tool.
type ToolNetworkConfiguration struct {
	// Mode is required for creation.
	Mode ToolNetworkMode
	// VPC contains VPC-specific settings.
	VPC *ToolVPCConfiguration
}

// ToolVPCConfiguration identifies VPC attachments.
type ToolVPCConfiguration struct {
	// SubnetIDs contains subnet identities.
	SubnetIDs []string
	// SecurityGroupIDs contains security-group identities.
	SecurityGroupIDs []string
}

// ToolTag is one Cloud resource tag.
type ToolTag struct {
	// Key is the tag key.
	Key string
	// Value is the tag value.
	Value string
}

// ToolEnvironmentVariable is one Tool image environment entry.
type ToolEnvironmentVariable struct {
	// Name is the environment variable name.
	Name string
	// Value is the environment variable value and may be sensitive.
	Value string
}

// ToolPortConfiguration exposes one image port.
type ToolPortConfiguration struct {
	// Name is the logical port name.
	Name string
	// Protocol is the service protocol value.
	Protocol string
	// Port is the container port number.
	Port int64
}

// ToolResourceConfiguration requests image runtime resources.
type ToolResourceConfiguration struct {
	// CPU is the service CPU quantity string.
	CPU string
	// Memory is the service memory quantity string.
	Memory string
	// Storage is the optional ephemeral-storage quantity string.
	Storage string
}

// ToolHTTPGetAction configures an HTTP readiness probe.
type ToolHTTPGetAction struct {
	// Path is the HTTP path.
	Path string
	// Scheme is the HTTP scheme.
	Scheme string
	// Port is the probe port.
	Port int64
}

// ToolProbeConfiguration configures image readiness.
type ToolProbeConfiguration struct {
	// HTTPGet is the optional HTTP action.
	HTTPGet *ToolHTTPGetAction
	// ReadyTimeoutMS bounds total readiness waiting in milliseconds.
	ReadyTimeoutMS int64
	// ProbeTimeoutMS bounds one probe in milliseconds.
	ProbeTimeoutMS int64
	// ProbePeriodMS is the interval between probes in milliseconds.
	ProbePeriodMS int64
	// SuccessThreshold is the required consecutive-success count.
	SuccessThreshold int64
	// FailureThreshold is the consecutive-failure count.
	FailureThreshold int64
}

// ToolDNSConfiguration configures image DNS resolution.
type ToolDNSConfiguration struct {
	// Servers contains DNS server IP addresses.
	Servers []string
	// Searches contains DNS search domains.
	Searches []string
	// Options contains resolv.conf options.
	Options []string
}

// ToolCustomConfiguration configures a custom image.
type ToolCustomConfiguration struct {
	// Image is the image reference.
	Image string
	// ImageRegistryType identifies the image registry.
	ImageRegistryType ImageRegistryType
	// Command overrides the image entrypoint.
	Command []string
	// Args supplies image arguments.
	Args []string
	// Env supplies image environment variables.
	Env []ToolEnvironmentVariable
	// Ports declares exposed ports.
	Ports []ToolPortConfiguration
	// Resources supplies optional runtime resources.
	Resources *ToolResourceConfiguration
	// Probe supplies optional readiness behavior.
	Probe *ToolProbeConfiguration
	// DNS supplies optional DNS behavior.
	DNS *ToolDNSConfiguration
}

// ToolCustomConfigurationInfo is the resolved custom-image configuration returned by the service.
type ToolCustomConfigurationInfo struct {
	// ToolCustomConfiguration contains fields accepted by create and update.
	ToolCustomConfiguration
	// ImageDigest is the resolved output digest.
	ImageDigest string
}

// ToolComputerConfiguration configures desktop-environment variants.
type ToolComputerConfiguration struct {
	// WAAImageID selects a custom WAA image.
	WAAImageID string
	// OSWorldVersion selects the built-in OSWorld version.
	OSWorldVersion string
}

// ToolLogConfiguration configures Tool log collection.
type ToolLogConfiguration struct {
	// CLSTopicID is the destination CLS topic.
	CLSTopicID string
	// Files contains service-supported log file paths.
	Files []string
}

// ToolCOSStorage configures a COS-backed mount.
type ToolCOSStorage struct {
	// Endpoint is the COS endpoint.
	Endpoint string
	// BucketName is the COS bucket name.
	BucketName string
	// BucketPath is an absolute path inside the bucket and must start with "/".
	BucketPath string
}

// ToolCFSStorage configures a CFS-backed mount.
type ToolCFSStorage struct {
	// FileSystemID is the CFS identity.
	FileSystemID string
	// Path is the path inside the file system.
	Path string
}

// ToolImageStorage configures an image-backed mount.
type ToolImageStorage struct {
	// Reference is the image reference.
	Reference string
	// ImageRegistryType is the registry type reported by the service.
	ImageRegistryType string
	// SubPath is the path inside the image.
	SubPath string
	// Digest pins an image digest.
	Digest string
}

// ToolAgentBucketStorage configures an AgentBucket-backed mount.
type ToolAgentBucketStorage struct {
	// LibraryID is the AgentBucket library identity.
	LibraryID string
	// SpaceID is the AgentBucket space identity.
	SpaceID string
	// AccessDomain is the AgentBucket access domain.
	AccessDomain string
}

// ToolStorageSource selects exactly one storage backend when non-nil.
type ToolStorageSource struct {
	// COS selects COS storage.
	COS *ToolCOSStorage
	// CFS selects CFS storage.
	CFS *ToolCFSStorage
	// Image selects image storage.
	Image *ToolImageStorage
	// AgentBucket selects AgentBucket storage.
	AgentBucket *ToolAgentBucketStorage
}

// ToolVolumeRef selects one existing Preview Volume by exactly one identity.
type ToolVolumeRef struct {
	// ID is the stable Volume identity.
	ID string
	// Name is the account-local Volume name.
	Name string
}

// ToolVolumeTemplateRef derives or reuses a Volume from a Preview template.
type ToolVolumeTemplateRef struct {
	// Name is the VolumeTemplate name.
	Name string
	// ReuseKey may be a literal or an unmodified ${metadata.<key>} expression.
	ReuseKey string
}

// ToolStorageMount declares one mount available to Sandboxes created from a Tool.
type ToolStorageMount struct {
	// Name is the mount configuration name.
	Name string
	// MountPath is the instance-local mount path.
	MountPath string
	// ReadOnly preserves omitted versus explicit false.
	ReadOnly *bool
	// Source is the storage backend; nil preserves the service default.
	Source *ToolStorageSource
	// Volume mounts one existing Preview Volume. It is mutually exclusive with Source and VolumeTemplate.
	Volume *ToolVolumeRef
	// VolumeTemplate derives or reuses a Preview Volume. It is mutually exclusive with Source and Volume.
	VolumeTemplate *ToolVolumeTemplateRef
}

// CreateToolOptions contains only fields accepted by CreateSandboxTool.
type CreateToolOptions struct {
	// Name is the account-unique Tool name.
	Name string
	// Description is optional user-facing text.
	Description string
	// DefaultTimeout is a Go-style duration string accepted by the service.
	DefaultTimeout string
	// ClientToken is optional Cloud idempotency material.
	ClientToken string
	// RoleARN is the role assumed by Tool-created Sandboxes.
	RoleARN string
	// Type is the required Tool category.
	Type ToolType
	// Network is required and configures Sandbox networking.
	Network ToolNetworkConfiguration
	// Tags is the initial tag set.
	Tags []ToolTag
	// StorageMounts declares available storage mounts.
	StorageMounts []ToolStorageMount
	// CustomConfiguration configures a custom image.
	CustomConfiguration *ToolCustomConfiguration
	// ComputerConfiguration configures desktop variants.
	ComputerConfiguration *ToolComputerConfiguration
	// LogConfiguration configures log collection.
	LogConfiguration *ToolLogConfiguration
	// Persistent preserves omission versus explicit false.
	Persistent *bool
}

// String returns a redacted representation because custom configuration can contain secrets.
func (CreateToolOptions) String() string { return "CreateToolOptions{REDACTED}" }

// GoString returns the same redacted representation as String.
func (o CreateToolOptions) GoString() string { return o.String() }

// UpdateToolOptions uses pointers to distinguish omission from an explicit empty value.
type UpdateToolOptions struct {
	// Description preserves omission versus explicit empty text.
	Description *string
	// Network replaces the network configuration when non-nil.
	Network *ToolNetworkConfiguration
	// Tags replaces tags when non-nil. The current typed transport rejects an empty replacement.
	Tags *[]ToolTag
	// CustomConfiguration replaces custom-image settings when non-nil.
	CustomConfiguration *ToolCustomConfiguration
	// ComputerConfiguration replaces desktop settings when non-nil.
	ComputerConfiguration *ToolComputerConfiguration
}

// ToolListFilter is one service-defined exact list filter.
type ToolListFilter struct {
	// Name is the service filter name.
	Name string
	// Values are ORed inside this filter.
	Values []string
}

// ToolListOptions selects one offset-based Tool page.
type ToolListOptions struct {
	// ToolIDs selects at most 100 exact identities.
	ToolIDs []string
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
	// Filters are ANDed by the service.
	Filters []ToolListFilter
}

// ToolPage is one service-reported offset page.
type ToolPage struct {
	// Items contains Tool descriptions.
	Items []ToolInfo
	// TotalCount is the service count for the query.
	TotalCount int
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
}

// ToolMutationResult confirms service acceptance without inventing a Tool snapshot.
type ToolMutationResult struct {
	// ToolID identifies the accepted mutation target.
	ToolID string
	// RequestID is service correlation data.
	RequestID string
}

// ToolInfo is a control-plane Tool description.
type ToolInfo struct {
	// ID is the Tool identity.
	ID string
	// Name is the account-unique Tool name.
	Name string
	// Description is the user-facing description.
	Description string
	// RoleARN is the bound role.
	RoleARN string
	// StatusReason explains the current service state when available.
	StatusReason string
	// Type is the Tool category.
	Type ToolType
	// State is the service state; unrecognized values are preserved.
	State ToolState
	// Persistent reports whether the Tool is persistent.
	Persistent bool
	// DefaultTimeoutSeconds is the service-normalized default lifetime.
	DefaultTimeoutSeconds uint64
	// Network is the current networking configuration.
	Network ToolNetworkConfiguration
	// Tags is the current tag set.
	Tags []ToolTag
	// CreatedAt is the reported creation time.
	CreatedAt time.Time
	// UpdatedAt is the reported update time.
	UpdatedAt time.Time
	// StorageMounts is the current mount configuration.
	StorageMounts []ToolStorageMount
	// CustomConfiguration is the resolved custom-image configuration.
	CustomConfiguration *ToolCustomConfigurationInfo
	// ComputerConfiguration is the current desktop configuration.
	ComputerConfiguration *ToolComputerConfiguration
	// LogConfiguration is the current log configuration.
	LogConfiguration *ToolLogConfiguration
}

// PreCacheImageRef is the complete service identity of an image pre-cache task.
type PreCacheImageRef struct {
	// Image is the image reference.
	Image string
	// Digest is the resolved image digest.
	Digest string
	// RegistryType identifies the image registry.
	RegistryType ImageRegistryType
}

// CreatePreCacheImageOptions starts image pre-caching.
type CreatePreCacheImageOptions struct {
	// Image is the image reference.
	Image string
	// RegistryType identifies the image registry.
	RegistryType ImageRegistryType
}

// PreCacheImageResult confirms task creation but does not imply successful pre-caching.
type PreCacheImageResult struct {
	// Ref is required for subsequent Get calls.
	Ref PreCacheImageRef
	// RequestID is service correlation data.
	RequestID string
}

// PreCacheImageInfo reports image pre-cache task status.
type PreCacheImageInfo struct {
	// Ref is the complete task identity.
	Ref PreCacheImageRef
	// Status is the service task state.
	Status string
	// Message is service status text, not a stable machine-readable code.
	Message string
	// RequestID is service correlation data.
	RequestID string
}

type toolControlPlane interface {
	CreateTool(context.Context, CreateToolOptions) (ToolMutationResult, error)
	GetTool(context.Context, string) (ToolInfo, error)
	ListTools(context.Context, ToolListOptions) (ToolPage, error)
	UpdateTool(context.Context, string, UpdateToolOptions) (ToolMutationResult, error)
	DeleteTool(context.Context, string) error
	CreatePreCacheImage(context.Context, CreatePreCacheImageOptions) (PreCacheImageResult, error)
	GetPreCacheImage(context.Context, PreCacheImageRef) (PreCacheImageInfo, error)
}

func (m *ToolManager) transport(operation string) (toolControlPlane, error) {
	value, ok := m.client.cfg.control.(toolControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "TOOL_CONTROL_UNAVAILABLE")
	}
	return value, nil
}

// Create submits one Tool mutation. It does not wait for ACTIVE or issue an implicit Get.
func (m *ToolManager) Create(ctx context.Context, opts CreateToolOptions) (ToolMutationResult, error) {
	opts = copyCreateToolOptions(opts)
	if err := validateCreateTool(opts); err != nil {
		return ToolMutationResult{}, err
	}
	cp, err := m.transport("Tools.Create")
	if err != nil {
		return ToolMutationResult{}, err
	}
	return cp.CreateTool(ctx, opts)
}

// Get returns the exact Tool identity or NotFound without scanning all pages.
func (m *ToolManager) Get(ctx context.Context, toolID string) (ToolInfo, error) {
	if toolID == "" {
		return ToolInfo{}, codeError(InvalidArgument, "Tools.Get", "TOOL_ID_REQUIRED")
	}
	cp, err := m.transport("Tools.Get")
	if err != nil {
		return ToolInfo{}, err
	}
	return cp.GetTool(ctx, toolID)
}

// List returns one server-filtered Tool page.
func (m *ToolManager) List(ctx context.Context, opts ToolListOptions) (ToolPage, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 || opts.Limit < 1 || opts.Limit > 100 {
		return ToolPage{}, codeError(InvalidArgument, "Tools.List", "PAGINATION_OUT_OF_RANGE")
	}
	if len(opts.ToolIDs) > 100 {
		return ToolPage{}, codeError(InvalidArgument, "Tools.List", "TOO_MANY_TOOL_IDS")
	}
	for _, id := range opts.ToolIDs {
		if id == "" {
			return ToolPage{}, codeError(InvalidArgument, "Tools.List", "TOOL_ID_REQUIRED")
		}
	}
	for _, filter := range opts.Filters {
		if filter.Name == "" || len(filter.Values) == 0 {
			return ToolPage{}, codeError(InvalidArgument, "Tools.List", "FILTER_INVALID")
		}
	}
	opts = copyToolListOptions(opts)
	cp, err := m.transport("Tools.List")
	if err != nil {
		return ToolPage{}, err
	}
	return cp.ListTools(ctx, opts)
}

// Update submits only fields represented by UpdateSandboxTool and does not issue an implicit Get.
func (m *ToolManager) Update(ctx context.Context, toolID string, opts UpdateToolOptions) (ToolMutationResult, error) {
	if toolID == "" {
		return ToolMutationResult{}, codeError(InvalidArgument, "Tools.Update", "TOOL_ID_REQUIRED")
	}
	opts = copyUpdateToolOptions(opts)
	if err := validateUpdateTool(opts); err != nil {
		return ToolMutationResult{}, err
	}
	cp, err := m.transport("Tools.Update")
	if err != nil {
		return ToolMutationResult{}, err
	}
	return cp.UpdateTool(ctx, toolID, opts)
}

// Delete requests Tool deletion. NotFound is idempotent; linked Sandboxes are never deleted.
func (m *ToolManager) Delete(ctx context.Context, toolID string) error {
	if toolID == "" {
		return codeError(InvalidArgument, "Tools.Delete", "TOOL_ID_REQUIRED")
	}
	cp, err := m.transport("Tools.Delete")
	if err != nil {
		return err
	}
	err = cp.DeleteTool(ctx, toolID)
	var failure *Error
	if errors.As(err, &failure) && failure.Code == NotFound {
		return nil
	}
	return err
}

// WaitFor polls Get until target, a conflicting FAILED state, or context cancellation.
func (m *ToolManager) WaitFor(ctx context.Context, toolID string, target ToolState) (ToolInfo, error) {
	if toolID == "" {
		return ToolInfo{}, codeError(InvalidArgument, "Tools.WaitFor", "TOOL_ID_REQUIRED")
	}
	switch target {
	case ToolCreating, ToolActive, ToolDeleting, ToolFailed:
	default:
		return ToolInfo{}, codeError(InvalidArgument, "Tools.WaitFor", "TOOL_STATE_INVALID")
	}
	for {
		info, err := m.Get(ctx, toolID)
		if err != nil {
			return ToolInfo{}, err
		}
		if info.State == target {
			return info, nil
		}
		if info.State == ToolFailed {
			return ToolInfo{}, withResourceError(codeError(Conflict, "Tools.WaitFor", "TOOL_FAILED"), "tool", toolID)
		}
		select {
		case <-ctx.Done():
			return ToolInfo{}, normalizeError("Tools.WaitFor", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (m *PreCacheImageManager) transport(operation string) (toolControlPlane, error) {
	value, ok := m.client.cfg.control.(toolControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "PRE_CACHE_CONTROL_UNAVAILABLE")
	}
	return value, nil
}

// Create starts pre-caching and returns the complete reference needed by Get.
func (m *PreCacheImageManager) Create(ctx context.Context, opts CreatePreCacheImageOptions) (PreCacheImageResult, error) {
	if err := validateImage(opts.Image, opts.RegistryType, "PreCacheImages.Create"); err != nil {
		return PreCacheImageResult{}, err
	}
	cp, err := m.transport("PreCacheImages.Create")
	if err != nil {
		return PreCacheImageResult{}, err
	}
	return cp.CreatePreCacheImage(ctx, opts)
}

// Get reads pre-cache status using image, digest, and registry type; there is no task ID.
func (m *PreCacheImageManager) Get(ctx context.Context, ref PreCacheImageRef) (PreCacheImageInfo, error) {
	if ref.Digest == "" {
		return PreCacheImageInfo{}, codeError(InvalidArgument, "PreCacheImages.Get", "IMAGE_DIGEST_REQUIRED")
	}
	if err := validateImage(ref.Image, ref.RegistryType, "PreCacheImages.Get"); err != nil {
		return PreCacheImageInfo{}, err
	}
	cp, err := m.transport("PreCacheImages.Get")
	if err != nil {
		return PreCacheImageInfo{}, err
	}
	return cp.GetPreCacheImage(ctx, ref)
}

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)

func validateCreateTool(opts CreateToolOptions) error {
	if !toolNamePattern.MatchString(opts.Name) {
		return codeError(InvalidArgument, "Tools.Create", "TOOL_NAME_INVALID")
	}
	if opts.Type == "" {
		return codeError(InvalidArgument, "Tools.Create", "TOOL_TYPE_REQUIRED")
	}
	if err := validateToolNetwork(opts.Network, "Tools.Create"); err != nil {
		return err
	}
	if utf8.RuneCountInString(opts.Description) > 200 {
		return codeError(InvalidArgument, "Tools.Create", "DESCRIPTION_TOO_LONG")
	}
	if len(opts.ClientToken) > 64 {
		return codeError(InvalidArgument, "Tools.Create", "CLIENT_TOKEN_TOO_LONG")
	}
	if opts.DefaultTimeout != "" {
		value, err := time.ParseDuration(opts.DefaultTimeout)
		if err != nil || value <= 0 || value > 24*time.Hour {
			return codeError(InvalidArgument, "Tools.Create", "DEFAULT_TIMEOUT_INVALID")
		}
	}
	if opts.CustomConfiguration != nil {
		if err := validateImageOptional(opts.CustomConfiguration.Image, opts.CustomConfiguration.ImageRegistryType, "Tools.Create"); err != nil {
			return err
		}
	}
	return validateToolMounts(opts.StorageMounts, "Tools.Create")
}
func validateUpdateTool(opts UpdateToolOptions) error {
	if opts.Description != nil && utf8.RuneCountInString(*opts.Description) > 200 {
		return codeError(InvalidArgument, "Tools.Update", "DESCRIPTION_TOO_LONG")
	}
	if opts.Network != nil {
		if err := validateToolNetwork(*opts.Network, "Tools.Update"); err != nil {
			return err
		}
	}
	if opts.Tags != nil && len(*opts.Tags) == 0 {
		return codeError(Unsupported, "Tools.Update", "EMPTY_TAG_REPLACEMENT_UNAVAILABLE")
	}
	if opts.CustomConfiguration != nil {
		return validateImageOptional(opts.CustomConfiguration.Image, opts.CustomConfiguration.ImageRegistryType, "Tools.Update")
	}
	return nil
}
func validateToolNetwork(value ToolNetworkConfiguration, operation string) error {
	switch value.Mode {
	case ToolNetworkPublic, ToolNetworkVPC, ToolNetworkSandbox:
		return nil
	default:
		return codeError(InvalidArgument, operation, "NETWORK_MODE_INVALID")
	}
}
func validateImage(image string, registry ImageRegistryType, operation string) error {
	if image == "" {
		return codeError(InvalidArgument, operation, "IMAGE_REQUIRED")
	}
	switch registry {
	case ImageRegistryEnterprise, ImageRegistryPersonal:
		return nil
	default:
		return codeError(InvalidArgument, operation, "IMAGE_REGISTRY_TYPE_INVALID")
	}
}
func validateImageOptional(image string, registry ImageRegistryType, operation string) error {
	if image == "" && registry == "" {
		return nil
	}
	return validateImage(image, registry, operation)
}
func validateToolMounts(values []ToolStorageMount, operation string) error {
	for _, mount := range values {
		if mount.Name == "" {
			return codeError(InvalidArgument, operation, "STORAGE_MOUNT_NAME_REQUIRED")
		}
		sourceCount := 0
		if mount.Source != nil {
			sourceCount++
		}
		if mount.Volume != nil {
			sourceCount++
		}
		if mount.VolumeTemplate != nil {
			sourceCount++
		}
		if sourceCount > 1 {
			return codeError(InvalidArgument, operation, "STORAGE_MOUNT_SOURCE_CONFLICT")
		}
		if mount.Source != nil {
			count := 0
			if mount.Source.COS != nil {
				count++
				if mount.Source.COS.Endpoint == "" || mount.Source.COS.BucketName == "" || !strings.HasPrefix(mount.Source.COS.BucketPath, "/") {
					return codeError(InvalidArgument, operation, "COS_STORAGE_SOURCE_INVALID")
				}
			}
			if mount.Source.CFS != nil {
				count++
				if mount.Source.CFS.FileSystemID == "" || mount.Source.CFS.Path == "" {
					return codeError(InvalidArgument, operation, "CFS_STORAGE_SOURCE_INVALID")
				}
			}
			if mount.Source.Image != nil {
				count++
			}
			if mount.Source.AgentBucket != nil {
				count++
			}
			if count != 1 {
				return codeError(InvalidArgument, operation, "STORAGE_SOURCE_INVALID")
			}
		}
		if mount.Volume != nil {
			if (mount.Volume.ID == "") == (mount.Volume.Name == "") {
				return codeError(InvalidArgument, operation, "VOLUME_REF_INVALID")
			}
		}
		if mount.VolumeTemplate != nil && (mount.VolumeTemplate.Name == "" || mount.VolumeTemplate.ReuseKey == "") {
			return codeError(InvalidArgument, operation, "VOLUME_TEMPLATE_REF_INVALID")
		}
	}
	return nil
}

func copyCreateToolOptions(value CreateToolOptions) CreateToolOptions {
	value.Tags = append([]ToolTag(nil), value.Tags...)
	value.StorageMounts = copyToolMounts(value.StorageMounts)
	value.Network = copyToolNetwork(value.Network)
	value.CustomConfiguration = copyToolCustom(value.CustomConfiguration)
	value.ComputerConfiguration = copyToolComputer(value.ComputerConfiguration)
	value.LogConfiguration = copyToolLog(value.LogConfiguration)
	if value.Persistent != nil {
		copied := *value.Persistent
		value.Persistent = &copied
	}
	return value
}
func copyUpdateToolOptions(value UpdateToolOptions) UpdateToolOptions {
	if value.Description != nil {
		copied := *value.Description
		value.Description = &copied
	}
	if value.Network != nil {
		copied := copyToolNetwork(*value.Network)
		value.Network = &copied
	}
	if value.Tags != nil {
		copied := append([]ToolTag(nil), (*value.Tags)...)
		value.Tags = &copied
	}
	value.CustomConfiguration = copyToolCustom(value.CustomConfiguration)
	value.ComputerConfiguration = copyToolComputer(value.ComputerConfiguration)
	return value
}
func copyToolListOptions(value ToolListOptions) ToolListOptions {
	value.ToolIDs = append([]string(nil), value.ToolIDs...)
	value.Filters = append([]ToolListFilter(nil), value.Filters...)
	for i := range value.Filters {
		value.Filters[i].Values = append([]string(nil), value.Filters[i].Values...)
	}
	return value
}
func copyToolNetwork(value ToolNetworkConfiguration) ToolNetworkConfiguration {
	if value.VPC != nil {
		copied := *value.VPC
		copied.SubnetIDs = append([]string(nil), copied.SubnetIDs...)
		copied.SecurityGroupIDs = append([]string(nil), copied.SecurityGroupIDs...)
		value.VPC = &copied
	}
	return value
}
func copyToolCustom(value *ToolCustomConfiguration) *ToolCustomConfiguration {
	if value == nil {
		return nil
	}
	copied := *value
	copied.Command = append([]string(nil), value.Command...)
	copied.Args = append([]string(nil), value.Args...)
	copied.Env = append([]ToolEnvironmentVariable(nil), value.Env...)
	copied.Ports = append([]ToolPortConfiguration(nil), value.Ports...)
	if value.Resources != nil {
		v := *value.Resources
		copied.Resources = &v
	}
	if value.Probe != nil {
		v := *value.Probe
		if v.HTTPGet != nil {
			h := *v.HTTPGet
			v.HTTPGet = &h
		}
		copied.Probe = &v
	}
	if value.DNS != nil {
		v := *value.DNS
		v.Servers = append([]string(nil), v.Servers...)
		v.Searches = append([]string(nil), v.Searches...)
		v.Options = append([]string(nil), v.Options...)
		copied.DNS = &v
	}
	return &copied
}
func copyToolComputer(value *ToolComputerConfiguration) *ToolComputerConfiguration {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func copyToolLog(value *ToolLogConfiguration) *ToolLogConfiguration {
	if value == nil {
		return nil
	}
	copied := *value
	copied.Files = append([]string(nil), value.Files...)
	return &copied
}
func copyToolMounts(values []ToolStorageMount) []ToolStorageMount {
	out := append([]ToolStorageMount(nil), values...)
	for i := range out {
		if out[i].ReadOnly != nil {
			v := *out[i].ReadOnly
			out[i].ReadOnly = &v
		}
		if out[i].Source != nil {
			source := *out[i].Source
			if source.COS != nil {
				v := *source.COS
				source.COS = &v
			}
			if source.CFS != nil {
				v := *source.CFS
				source.CFS = &v
			}
			if source.Image != nil {
				v := *source.Image
				source.Image = &v
			}
			if source.AgentBucket != nil {
				v := *source.AgentBucket
				source.AgentBucket = &v
			}
			out[i].Source = &source
		}
		if out[i].Volume != nil {
			volume := *out[i].Volume
			out[i].Volume = &volume
		}
		if out[i].VolumeTemplate != nil {
			template := *out[i].VolumeTemplate
			out[i].VolumeTemplate = &template
		}
	}
	return out
}

func withResourceError(err error, resourceType, resourceID string) error {
	var failure *Error
	if errors.As(err, &failure) {
		copied := *failure
		copied.ResourceType, copied.ResourceID = resourceType, resourceID
		return &copied
	}
	return err
}

// VolumeAccessMode controls how many Sandbox instances may mount a Volume and
// whether those mounts are writable.
type VolumeAccessMode string

const (
	// VolumeReadWriteOnce allows one writable mount.
	VolumeReadWriteOnce VolumeAccessMode = "ReadWriteOnce"
	// VolumeReadWriteMany allows multiple writable mounts.
	VolumeReadWriteMany VolumeAccessMode = "ReadWriteMany"
	// VolumeReadOnlyMany allows multiple read-only mounts.
	VolumeReadOnlyMany VolumeAccessMode = "ReadOnlyMany"
)

// VolumeStorageType selects the backing storage contract. Create accepts only
// the constants below. List preserves unknown service values without exposing
// an unreviewed backing-storage shape.
type VolumeStorageType string

const (
	// VolumeStorageCOS selects a COS prefix.
	VolumeStorageCOS VolumeStorageType = "Cos"
	// VolumeStorageCFS selects a CFS path.
	VolumeStorageCFS VolumeStorageType = "Cfs"
)

// VolumeReclaimPolicy controls a template-derived Volume after its owning
// Sandbox is deleted.
type VolumeReclaimPolicy string

const (
	// VolumeReclaimRetain keeps a derived Volume after Sandbox deletion.
	VolumeReclaimRetain VolumeReclaimPolicy = "Retain"
	// VolumeReclaimDelete deletes a derived Volume after Sandbox deletion.
	VolumeReclaimDelete VolumeReclaimPolicy = "Delete"
)

// VolumeState preserves the service state verbatim, including future values.
type VolumeState string

const (
	// VolumeCreating means the resource is being provisioned.
	VolumeCreating VolumeState = "CREATING"
	// VolumeActive means the resource is ready for mounting.
	VolumeActive VolumeState = "ACTIVE"
	// VolumeDeleting means deletion is in progress.
	VolumeDeleting VolumeState = "DELETING"
	// VolumeDeleteFailed means the service could not finish deletion.
	VolumeDeleteFailed VolumeState = "DELETE_FAILED"
	// VolumeFailed means provisioning or reconciliation failed.
	VolumeFailed VolumeState = "FAILED"
)

// VolumeTag is one Cloud resource tag.
type VolumeTag struct {
	// Key is the tag key.
	Key string
	// Value is the tag value.
	Value string
}

// VolumeFilter is one service-defined exact list filter.
type VolumeFilter struct {
	// Name is the service filter name.
	Name string
	// Values are ORed inside this filter.
	Values []string
}

// VolumeCOSStorage identifies one concrete COS prefix.
type VolumeCOSStorage struct {
	// Endpoint is the COS regional endpoint.
	Endpoint string
	// BucketName is the COS bucket name.
	BucketName string
	// BucketPath is the concrete prefix inside the bucket.
	BucketPath string
}

// VolumeCFSStorage identifies one concrete CFS path.
type VolumeCFSStorage struct {
	// FileSystemID identifies the existing CFS file system.
	FileSystemID string
	// Path is the concrete directory in that file system.
	Path string
}

// VolumeStorage selects exactly one concrete COS or CFS backing store.
type VolumeStorage struct {
	// COS selects COS storage.
	COS *VolumeCOSStorage
	// CFS selects CFS storage.
	CFS *VolumeCFSStorage
}

// String returns a redacted storage representation.
func (VolumeStorage) String() string { return "VolumeStorage{REDACTED}" }

// GoString returns the same redacted representation as String.
func (v VolumeStorage) GoString() string { return v.String() }

// CreateVolumeOptions creates a standalone AGR Volume. Deleting the Volume
// record does not claim to delete the referenced COS or CFS storage.
type CreateVolumeOptions struct {
	// Name is unique within the account and region.
	Name string
	// AccessMode controls concurrent and writable mounts.
	AccessMode VolumeAccessMode
	// StorageType must match the selected Storage backend.
	StorageType VolumeStorageType
	// Storage identifies one existing backing location.
	Storage VolumeStorage
	// Tags is the initial complete tag set.
	Tags []VolumeTag
	// ClientToken is optional idempotency material.
	ClientToken string
	// StorageRoleARN is the role used to access backing storage.
	StorageRoleARN string
}

// String returns a redacted representation.
func (CreateVolumeOptions) String() string { return "CreateVolumeOptions{REDACTED}" }

// GoString returns the same redacted representation as String.
func (v CreateVolumeOptions) GoString() string { return v.String() }

// VolumeListOptions selects one offset-based Volume page.
type VolumeListOptions struct {
	// IDs selects exact Volume identities.
	IDs []string
	// Names selects exact Volume names.
	Names []string
	// Filters are ANDed by the service.
	Filters []VolumeFilter
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
}

// UpdateVolumeOptions contains mutable Volume fields.
type UpdateVolumeOptions struct {
	// Tags replaces the complete tag set. A pointer to an empty slice clears it.
	Tags *[]VolumeTag
}

// VolumeInfo is an immutable service snapshot.
type VolumeInfo struct {
	// ID is the stable Volume identity.
	ID string
	// Name is the account-local Volume name.
	Name string
	// AccessMode controls concurrent and writable mounts.
	AccessMode VolumeAccessMode
	// ReclaimPolicy reports template-owned cleanup behavior.
	ReclaimPolicy VolumeReclaimPolicy
	// State preserves the service value, including unknown future values.
	State VolumeState
	// StorageType identifies the backing storage kind.
	StorageType VolumeStorageType
	// Storage contains a reviewed backing location and is redacted when formatted.
	// It is empty when StorageType is not modeled by this SDK version.
	Storage VolumeStorage
	// TemplateID identifies the source template for a derived Volume.
	TemplateID string
	// ReuseKey identifies the template reuse partition.
	ReuseKey string
	// Tags is the current complete tag set.
	Tags []VolumeTag
	// CreatedAt is the service creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the service update timestamp.
	UpdatedAt time.Time
	// StorageRoleARN is the backing-storage access role.
	StorageRoleARN string
	// MountedInstanceID reports the current mounted Sandbox when available.
	MountedInstanceID string
	// RequestID is service correlation data.
	RequestID string
}

// String returns a representation that omits backing storage details.
func (v VolumeInfo) String() string {
	return "VolumeInfo{ID:" + v.ID + ",Name:" + v.Name + ",State:" + string(v.State) + "}"
}

// GoString returns the same redacted representation as String.
func (v VolumeInfo) GoString() string { return v.String() }

// VolumeMutationResult confirms an accepted mutation.
type VolumeMutationResult struct {
	// VolumeID identifies the mutation target.
	VolumeID string
	// RequestID is service correlation data.
	RequestID string
}

// VolumePage is one service-reported offset page.
type VolumePage struct {
	// Items contains Volume snapshots.
	Items []VolumeInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

type volumeControlPlane interface {
	CreateVolume(context.Context, CreateVolumeOptions) (VolumeInfo, error)
	GetVolume(context.Context, string) (VolumeInfo, error)
	ListVolumes(context.Context, VolumeListOptions) (VolumePage, error)
	UpdateVolume(context.Context, string, UpdateVolumeOptions) (VolumeMutationResult, error)
	DeleteVolume(context.Context, string) error
}

// VolumeManager manages Preview Volumes for one Client identity and region.
type VolumeManager struct{ client *Client }

func (m *VolumeManager) transport(operation string) (volumeControlPlane, error) {
	value, ok := m.client.cfg.control.(volumeControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "VOLUME_CONTROL_UNAVAILABLE")
	}
	return value, nil
}

// Create creates one standalone Volume without creating its backing storage.
func (m *VolumeManager) Create(ctx context.Context, opts CreateVolumeOptions) (VolumeInfo, error) {
	opts = copyCreateVolumeOptions(opts)
	if err := validateCreateVolume(opts); err != nil {
		return VolumeInfo{}, err
	}
	cp, err := m.transport("Volumes.Create")
	if err != nil {
		return VolumeInfo{}, err
	}
	return cp.CreateVolume(ctx, opts)
}

// Get returns the exact Volume ID or NotFound.
func (m *VolumeManager) Get(ctx context.Context, id string) (VolumeInfo, error) {
	if strings.TrimSpace(id) == "" {
		return VolumeInfo{}, codeError(InvalidArgument, "Volumes.Get", "VOLUME_ID_REQUIRED")
	}
	cp, err := m.transport("Volumes.Get")
	if err != nil {
		return VolumeInfo{}, err
	}
	return cp.GetVolume(ctx, id)
}

// List returns one server-filtered offset page.
func (m *VolumeManager) List(ctx context.Context, opts VolumeListOptions) (VolumePage, error) {
	opts = copyVolumeListOptions(opts)
	if err := validateVolumeList(opts, "Volumes.List"); err != nil {
		return VolumePage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport("Volumes.List")
	if err != nil {
		return VolumePage{}, err
	}
	return cp.ListVolumes(ctx, opts)
}

// Update replaces mutable fields on one exact Volume ID.
func (m *VolumeManager) Update(ctx context.Context, id string, opts UpdateVolumeOptions) (VolumeMutationResult, error) {
	if strings.TrimSpace(id) == "" {
		return VolumeMutationResult{}, codeError(InvalidArgument, "Volumes.Update", "VOLUME_ID_REQUIRED")
	}
	opts = copyUpdateVolumeOptions(opts)
	if opts.Tags == nil {
		return VolumeMutationResult{}, codeError(InvalidArgument, "Volumes.Update", "UPDATE_REQUIRED")
	}
	cp, err := m.transport("Volumes.Update")
	if err != nil {
		return VolumeMutationResult{}, err
	}
	return cp.UpdateVolume(ctx, id, opts)
}

// Delete removes the Volume record and does not claim to delete backing storage.
func (m *VolumeManager) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return codeError(InvalidArgument, "Volumes.Delete", "VOLUME_ID_REQUIRED")
	}
	cp, err := m.transport("Volumes.Delete")
	if err != nil {
		return err
	}
	return cp.DeleteVolume(ctx, id)
}

func validateCreateVolume(value CreateVolumeOptions) error {
	if strings.TrimSpace(value.Name) == "" {
		return codeError(InvalidArgument, "Volumes.Create", "VOLUME_NAME_REQUIRED")
	}
	if !validVolumeAccessMode(value.AccessMode) {
		return codeError(InvalidArgument, "Volumes.Create", "ACCESS_MODE_INVALID")
	}
	if err := validateVolumeStorage(value.StorageType, value.Storage, "Volumes.Create"); err != nil {
		return err
	}
	if len(value.ClientToken) > 64 {
		return codeError(InvalidArgument, "Volumes.Create", "CLIENT_TOKEN_INVALID")
	}
	return nil
}

func validateVolumeStorage(kind VolumeStorageType, value VolumeStorage, operation string) error {
	count := 0
	if value.COS != nil {
		count++
		if value.COS.Endpoint == "" || value.COS.BucketName == "" || value.COS.BucketPath == "" {
			return codeError(InvalidArgument, operation, "STORAGE_INVALID")
		}
	}
	if value.CFS != nil {
		count++
		if value.CFS.FileSystemID == "" || value.CFS.Path == "" {
			return codeError(InvalidArgument, operation, "STORAGE_INVALID")
		}
	}
	if count != 1 || (kind == VolumeStorageCOS) != (value.COS != nil) || (kind == VolumeStorageCFS) != (value.CFS != nil) {
		return codeError(InvalidArgument, operation, "STORAGE_TYPE_MISMATCH")
	}
	return nil
}

func validVolumeAccessMode(value VolumeAccessMode) bool {
	return value == VolumeReadWriteOnce || value == VolumeReadWriteMany || value == VolumeReadOnlyMany
}

func validateVolumeList(value VolumeListOptions, operation string) error {
	if value.Offset < 0 || value.Limit < 0 || value.Limit > 100 {
		return codeError(InvalidArgument, operation, "PAGINATION_INVALID")
	}
	if len(value.IDs) > 100 {
		return codeError(InvalidArgument, operation, "TOO_MANY_VOLUME_IDS")
	}
	for _, id := range value.IDs {
		if strings.TrimSpace(id) == "" {
			return codeError(InvalidArgument, operation, "VOLUME_ID_REQUIRED")
		}
	}
	for _, name := range value.Names {
		if strings.TrimSpace(name) == "" {
			return codeError(InvalidArgument, operation, "VOLUME_NAME_REQUIRED")
		}
	}
	for _, filter := range value.Filters {
		if filter.Name == "" || len(filter.Values) == 0 {
			return codeError(InvalidArgument, operation, "FILTER_INVALID")
		}
		for _, item := range filter.Values {
			if item == "" {
				return codeError(InvalidArgument, operation, "FILTER_INVALID")
			}
		}
	}
	return nil
}

func copyCreateVolumeOptions(value CreateVolumeOptions) CreateVolumeOptions {
	value.Tags = append([]VolumeTag(nil), value.Tags...)
	value.Storage = copyVolumeStorage(value.Storage)
	return value
}

func copyVolumeStorage(value VolumeStorage) VolumeStorage {
	out := VolumeStorage{}
	if value.COS != nil {
		copy := *value.COS
		out.COS = &copy
	}
	if value.CFS != nil {
		copy := *value.CFS
		out.CFS = &copy
	}
	return out
}

func copyVolumeListOptions(value VolumeListOptions) VolumeListOptions {
	value.IDs = append([]string(nil), value.IDs...)
	value.Names = append([]string(nil), value.Names...)
	value.Filters = append([]VolumeFilter(nil), value.Filters...)
	for i := range value.Filters {
		value.Filters[i].Values = append([]string(nil), value.Filters[i].Values...)
	}
	return value
}

func copyUpdateVolumeOptions(value UpdateVolumeOptions) UpdateVolumeOptions {
	if value.Tags != nil {
		tags := append([]VolumeTag(nil), (*value.Tags)...)
		value.Tags = &tags
	}
	return value
}

// VolumeTemplateCOSStorage identifies a COS prefix pattern for derived Volumes.
type VolumeTemplateCOSStorage struct {
	// Endpoint is the COS regional endpoint.
	Endpoint string
	// BucketName is the COS bucket name.
	BucketName string
	// BucketPathPattern may contain ${reuse_key}.
	BucketPathPattern string
}

// VolumeTemplateCFSStorage identifies a CFS path pattern for derived Volumes.
type VolumeTemplateCFSStorage struct {
	// FileSystemID identifies the existing CFS file system.
	FileSystemID string
	// PathPattern may contain ${reuse_key}.
	PathPattern string
}

// VolumeTemplateStorage selects exactly one COS or CFS pattern.
type VolumeTemplateStorage struct {
	// COS selects a COS prefix pattern.
	COS *VolumeTemplateCOSStorage
	// CFS selects a CFS path pattern.
	CFS *VolumeTemplateCFSStorage
}

// String returns a redacted storage representation.
func (VolumeTemplateStorage) String() string { return "VolumeTemplateStorage{REDACTED}" }

// GoString returns the same redacted representation as String.
func (v VolumeTemplateStorage) GoString() string { return v.String() }

// CreateVolumeTemplateOptions creates one derivation and reuse policy.
type CreateVolumeTemplateOptions struct {
	// Name is unique within the account and region.
	Name string
	// AccessMode controls concurrent and writable mounts.
	AccessMode VolumeAccessMode
	// ReclaimPolicy controls a derived Volume after Sandbox deletion.
	ReclaimPolicy VolumeReclaimPolicy
	// StorageType must match the selected Storage backend.
	StorageType VolumeStorageType
	// Storage identifies one backing location pattern.
	Storage VolumeTemplateStorage
	// VolumeNamePattern may contain ${reuse_key}.
	VolumeNamePattern string
	// DefaultCapacity is a service-defined capacity value.
	DefaultCapacity string
	// StorageRoleARN is the role used to access backing storage.
	StorageRoleARN string
	// Tags is the initial complete tag set.
	Tags []VolumeTag
	// ClientToken is optional idempotency material.
	ClientToken string
}

// String returns a redacted representation.
func (CreateVolumeTemplateOptions) String() string { return "CreateVolumeTemplateOptions{REDACTED}" }

// GoString returns the same redacted representation as String.
func (v CreateVolumeTemplateOptions) GoString() string { return v.String() }

// VolumeTemplateListOptions selects one offset-based template page.
type VolumeTemplateListOptions struct {
	// IDs selects exact VolumeTemplate identities.
	IDs []string
	// Names selects exact VolumeTemplate names.
	Names []string
	// Filters are ANDed by the service.
	Filters []VolumeFilter
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
}

// UpdateVolumeTemplateOptions contains mutable template fields.
type UpdateVolumeTemplateOptions struct {
	// Tags replaces the complete tag set; an empty slice clears it.
	Tags *[]VolumeTag
	// DefaultCapacity preserves omission versus explicit empty text.
	DefaultCapacity *string
}

// VolumeTemplateInfo is an immutable service snapshot.
type VolumeTemplateInfo struct {
	// ID is the stable VolumeTemplate identity.
	ID string
	// Name is the account-local VolumeTemplate name.
	Name string
	// AccessMode controls concurrent and writable mounts.
	AccessMode VolumeAccessMode
	// ReclaimPolicy controls derived Volume cleanup.
	ReclaimPolicy VolumeReclaimPolicy
	// State preserves the service value, including unknown future values.
	State VolumeState
	// StorageType identifies the backing storage kind.
	StorageType VolumeStorageType
	// Storage contains a reviewed backing pattern and is redacted when formatted.
	// It is empty when StorageType is not modeled by this SDK version.
	Storage VolumeTemplateStorage
	// VolumeNamePattern is applied to derived Volumes.
	VolumeNamePattern string
	// DefaultCapacity is the service-defined default capacity.
	DefaultCapacity string
	// StorageRoleARN is the backing-storage access role.
	StorageRoleARN string
	// Tags is the current complete tag set.
	Tags []VolumeTag
	// CreatedAt is the service creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the service update timestamp.
	UpdatedAt time.Time
	// RequestID is service correlation data.
	RequestID string
}

// String returns a representation that omits backing storage details.
func (v VolumeTemplateInfo) String() string {
	return "VolumeTemplateInfo{ID:" + v.ID + ",Name:" + v.Name + ",State:" + string(v.State) + "}"
}

// GoString returns the same redacted representation as String.
func (v VolumeTemplateInfo) GoString() string { return v.String() }

// VolumeTemplateMutationResult confirms an accepted mutation.
type VolumeTemplateMutationResult struct {
	// VolumeTemplateID identifies the mutation target.
	VolumeTemplateID string
	// RequestID is service correlation data.
	RequestID string
}

// VolumeTemplatePage is one service-reported offset page.
type VolumeTemplatePage struct {
	// Items contains VolumeTemplate snapshots.
	Items []VolumeTemplateInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

type volumeTemplateControlPlane interface {
	CreateVolumeTemplate(context.Context, CreateVolumeTemplateOptions) (VolumeTemplateInfo, error)
	GetVolumeTemplate(context.Context, string) (VolumeTemplateInfo, error)
	ListVolumeTemplates(context.Context, VolumeTemplateListOptions) (VolumeTemplatePage, error)
	UpdateVolumeTemplate(context.Context, string, UpdateVolumeTemplateOptions) (VolumeTemplateMutationResult, error)
	DeleteVolumeTemplate(context.Context, string) error
}

// VolumeTemplateManager manages Preview templates for one Client identity and region.
type VolumeTemplateManager struct{ client *Client }

func (m *VolumeTemplateManager) transport(operation string) (volumeTemplateControlPlane, error) {
	value, ok := m.client.cfg.control.(volumeTemplateControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "VOLUME_TEMPLATE_CONTROL_UNAVAILABLE")
	}
	return value, nil
}

// Create creates one VolumeTemplate without creating backing storage.
func (m *VolumeTemplateManager) Create(ctx context.Context, opts CreateVolumeTemplateOptions) (VolumeTemplateInfo, error) {
	opts = copyCreateVolumeTemplateOptions(opts)
	if err := validateCreateVolumeTemplate(opts); err != nil {
		return VolumeTemplateInfo{}, err
	}
	cp, err := m.transport("VolumeTemplates.Create")
	if err != nil {
		return VolumeTemplateInfo{}, err
	}
	return cp.CreateVolumeTemplate(ctx, opts)
}

// Get returns the exact VolumeTemplate ID or NotFound.
func (m *VolumeTemplateManager) Get(ctx context.Context, id string) (VolumeTemplateInfo, error) {
	if strings.TrimSpace(id) == "" {
		return VolumeTemplateInfo{}, codeError(InvalidArgument, "VolumeTemplates.Get", "VOLUME_TEMPLATE_ID_REQUIRED")
	}
	cp, err := m.transport("VolumeTemplates.Get")
	if err != nil {
		return VolumeTemplateInfo{}, err
	}
	return cp.GetVolumeTemplate(ctx, id)
}

// List returns one server-filtered offset page.
func (m *VolumeTemplateManager) List(ctx context.Context, opts VolumeTemplateListOptions) (VolumeTemplatePage, error) {
	opts = copyVolumeTemplateListOptions(opts)
	if err := validateVolumeTemplateList(opts); err != nil {
		return VolumeTemplatePage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport("VolumeTemplates.List")
	if err != nil {
		return VolumeTemplatePage{}, err
	}
	return cp.ListVolumeTemplates(ctx, opts)
}

// Update replaces mutable fields on one exact VolumeTemplate ID.
func (m *VolumeTemplateManager) Update(ctx context.Context, id string, opts UpdateVolumeTemplateOptions) (VolumeTemplateMutationResult, error) {
	if strings.TrimSpace(id) == "" {
		return VolumeTemplateMutationResult{}, codeError(InvalidArgument, "VolumeTemplates.Update", "VOLUME_TEMPLATE_ID_REQUIRED")
	}
	opts = copyUpdateVolumeTemplateOptions(opts)
	if opts.Tags == nil && opts.DefaultCapacity == nil {
		return VolumeTemplateMutationResult{}, codeError(InvalidArgument, "VolumeTemplates.Update", "UPDATE_REQUIRED")
	}
	cp, err := m.transport("VolumeTemplates.Update")
	if err != nil {
		return VolumeTemplateMutationResult{}, err
	}
	return cp.UpdateVolumeTemplate(ctx, id, opts)
}

// Delete removes one exact VolumeTemplate ID after dependent Volumes are gone.
func (m *VolumeTemplateManager) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return codeError(InvalidArgument, "VolumeTemplates.Delete", "VOLUME_TEMPLATE_ID_REQUIRED")
	}
	cp, err := m.transport("VolumeTemplates.Delete")
	if err != nil {
		return err
	}
	return cp.DeleteVolumeTemplate(ctx, id)
}

func validateCreateVolumeTemplate(value CreateVolumeTemplateOptions) error {
	if strings.TrimSpace(value.Name) == "" {
		return codeError(InvalidArgument, "VolumeTemplates.Create", "VOLUME_TEMPLATE_NAME_REQUIRED")
	}
	if !validVolumeAccessMode(value.AccessMode) {
		return codeError(InvalidArgument, "VolumeTemplates.Create", "ACCESS_MODE_INVALID")
	}
	if value.ReclaimPolicy != VolumeReclaimRetain && value.ReclaimPolicy != VolumeReclaimDelete {
		return codeError(InvalidArgument, "VolumeTemplates.Create", "RECLAIM_POLICY_INVALID")
	}
	if err := validateVolumeTemplateStorage(value.StorageType, value.Storage); err != nil {
		return err
	}
	if len(value.ClientToken) > 64 {
		return codeError(InvalidArgument, "VolumeTemplates.Create", "CLIENT_TOKEN_INVALID")
	}
	return nil
}

func validateVolumeTemplateStorage(kind VolumeStorageType, value VolumeTemplateStorage) error {
	count := 0
	if value.COS != nil {
		count++
		if value.COS.Endpoint == "" || value.COS.BucketName == "" || value.COS.BucketPathPattern == "" {
			return codeError(InvalidArgument, "VolumeTemplates.Create", "STORAGE_INVALID")
		}
	}
	if value.CFS != nil {
		count++
		if value.CFS.FileSystemID == "" || value.CFS.PathPattern == "" {
			return codeError(InvalidArgument, "VolumeTemplates.Create", "STORAGE_INVALID")
		}
	}
	if count != 1 || (kind == VolumeStorageCOS) != (value.COS != nil) || (kind == VolumeStorageCFS) != (value.CFS != nil) {
		return codeError(InvalidArgument, "VolumeTemplates.Create", "STORAGE_TYPE_MISMATCH")
	}
	return nil
}

func validateVolumeTemplateList(value VolumeTemplateListOptions) error {
	return validateVolumeList(VolumeListOptions{IDs: value.IDs, Names: value.Names, Filters: value.Filters, Offset: value.Offset, Limit: value.Limit}, "VolumeTemplates.List")
}

func copyCreateVolumeTemplateOptions(value CreateVolumeTemplateOptions) CreateVolumeTemplateOptions {
	value.Tags = append([]VolumeTag(nil), value.Tags...)
	value.Storage = copyVolumeTemplateStorage(value.Storage)
	return value
}

func copyVolumeTemplateStorage(value VolumeTemplateStorage) VolumeTemplateStorage {
	out := VolumeTemplateStorage{}
	if value.COS != nil {
		copy := *value.COS
		out.COS = &copy
	}
	if value.CFS != nil {
		copy := *value.CFS
		out.CFS = &copy
	}
	return out
}

func copyVolumeTemplateListOptions(value VolumeTemplateListOptions) VolumeTemplateListOptions {
	shared := copyVolumeListOptions(VolumeListOptions{IDs: value.IDs, Names: value.Names, Filters: value.Filters, Offset: value.Offset, Limit: value.Limit})
	return VolumeTemplateListOptions{IDs: shared.IDs, Names: shared.Names, Filters: shared.Filters, Offset: shared.Offset, Limit: shared.Limit}
}

func copyUpdateVolumeTemplateOptions(value UpdateVolumeTemplateOptions) UpdateVolumeTemplateOptions {
	if value.Tags != nil {
		tags := append([]VolumeTag(nil), (*value.Tags)...)
		value.Tags = &tags
	}
	if value.DefaultCapacity != nil {
		capacity := *value.DefaultCapacity
		value.DefaultCapacity = &capacity
	}
	return value
}
