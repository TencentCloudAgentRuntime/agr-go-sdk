package agr

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRegistryJSONBytes = 1 << 20

// DefaultMaxRegistrySkillPackageBytes is the SDK safety limit used when a package transfer
// leaves MaxBytes unset. It is not a statement of the service-side package limit.
const DefaultMaxRegistrySkillPackageBytes int64 = 64 << 20

var registryLabelPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// RegistryApprovalMode controls whether new Record versions require an explicit decision.
type RegistryApprovalMode string

const (
	// RegistryApprovalAuto approves new versions automatically.
	RegistryApprovalAuto RegistryApprovalMode = "AUTO"
	// RegistryApprovalManual leaves new versions pending approval.
	RegistryApprovalManual RegistryApprovalMode = "MANUAL"
)

// RegistryStatus preserves service-defined Registry lifecycle values.
type RegistryStatus string

// RegistryDescriptorType selects the protocol represented by a Record.
type RegistryDescriptorType string

const (
	// RegistryDescriptorMCP represents an MCP server descriptor.
	RegistryDescriptorMCP RegistryDescriptorType = "MCP"
	// RegistryDescriptorA2A represents an A2A Agent Card.
	RegistryDescriptorA2A RegistryDescriptorType = "A2A"
	// RegistryDescriptorAGUI represents an AG-UI endpoint descriptor.
	RegistryDescriptorAGUI RegistryDescriptorType = "AGUI"
	// RegistryDescriptorCustom represents a caller-defined JSON descriptor.
	RegistryDescriptorCustom RegistryDescriptorType = "CUSTOM"
	// RegistryDescriptorAgentSkills represents an inline SKILL.md descriptor.
	RegistryDescriptorAgentSkills RegistryDescriptorType = "AGENT_SKILLS"
)

// RegistrySourceType identifies how descriptor content is supplied.
type RegistrySourceType string

const (
	// RegistrySourceManual sends descriptor content directly.
	RegistrySourceManual RegistrySourceType = "MANUAL"
	// RegistrySourceURLImport asks the service to import an explicit HTTPS endpoint.
	RegistrySourceURLImport RegistrySourceType = "URL_IMPORT"
	// RegistrySourceTarPackage creates a Version whose gzip tar content is uploaded separately.
	RegistrySourceTarPackage RegistrySourceType = "TAR_PACKAGE"
)

// RegistryRecordStatus preserves service-defined Record lifecycle values.
type RegistryRecordStatus string

// RegistryVersionStatus preserves service-defined Version states.
type RegistryVersionStatus string

// RegistryLabelOperation selects a label mutation.
type RegistryLabelOperation string

const (
	// RegistryLabelSet creates, binds, or moves a label.
	RegistryLabelSet RegistryLabelOperation = "SET"
	// RegistryLabelDelete deletes a custom label.
	RegistryLabelDelete RegistryLabelOperation = "DELETE"
)

// RegistrySyncStatus preserves service-defined remote synchronization outcomes.
type RegistrySyncStatus string

// RegistryTag is one Tencent Cloud Registry tag.
type RegistryTag struct {
	// Key is the tag key.
	Key string
	// Value is the tag value.
	Value string
}

// RegistryFilter is one reviewed server-side filter.
type RegistryFilter struct {
	// Name selects a reviewed service field.
	Name string
	// Values are ORed within the filter.
	Values []string
}

// CreateRegistryOptions creates one Registry.
type CreateRegistryOptions struct {
	// Name is unique within one account and region.
	Name string
	// Description is optional display text.
	Description string
	// ApprovalMode defaults to AUTO when empty.
	ApprovalMode RegistryApprovalMode
	// Tags are attached during creation.
	Tags []RegistryTag
}

// UpdateRegistryOptions replaces the Registry description. Empty clears it.
type UpdateRegistryOptions struct {
	// Description is the complete replacement and may be empty.
	Description string
}

// RegistryListOptions selects one Registry offset page.
type RegistryListOptions struct {
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
	// Filters are evaluated by the service.
	Filters []RegistryFilter
}

// RegistryInfo is an immutable Registry snapshot.
type RegistryInfo struct {
	// ID is the service-generated Registry identity.
	ID string
	// Name is the Registry name.
	Name string
	// Description is the current description.
	Description string
	// Region is the service region containing the Registry.
	Region string
	// ApprovalMode is fixed when the Registry is created.
	ApprovalMode RegistryApprovalMode
	// Status preserves known and future service values.
	Status RegistryStatus
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
	// RecordCount is the number of active Records.
	RecordCount int64
	// PublishedRecordCount is present when reported by the service.
	PublishedRecordCount *int64
	// AppID is present when reported by the service.
	AppID *int64
	// CreatorUIN is the creator's primary account identity.
	CreatorUIN string
	// CreatorSubUIN is the creator's sub-account identity.
	CreatorSubUIN string
	// Tags contains authoritative Cloud tags.
	Tags []RegistryTag
	// RequestID is service correlation data.
	RequestID string
}

// RegistryPage is one service-reported Registry page.
type RegistryPage struct {
	// Items contains Registry snapshots.
	Items []RegistryInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// RegistryMCPSource supplies MCP descriptor content or an import endpoint.
type RegistryMCPSource struct {
	// Type selects MANUAL or URL_IMPORT.
	Type RegistrySourceType
	// Descriptors is a bounded JSON object for MANUAL.
	Descriptors json.RawMessage
	// EndpointURL is an explicit HTTPS endpoint for URL_IMPORT.
	EndpointURL string
}

// RegistryAgentSource supplies A2A or AG-UI descriptor content or an import endpoint.
type RegistryAgentSource struct {
	// Type selects MANUAL or URL_IMPORT.
	Type RegistrySourceType
	// Descriptors is a bounded JSON object for MANUAL.
	Descriptors json.RawMessage
	// EndpointURL is an explicit HTTPS endpoint for URL_IMPORT.
	EndpointURL string
}

// RegistrySkillSource supplies inline SKILL.md content or declares a gzip tar package upload.
type RegistrySkillSource struct {
	// Type selects MANUAL or TAR_PACKAGE.
	Type RegistrySourceType
	// SkillMD contains bounded inline SKILL.md text for MANUAL and must be empty for TAR_PACKAGE.
	SkillMD string
}

// CreateRegistryRecordOptions creates a Record and its first Version.
type CreateRegistryRecordOptions struct {
	// Name is unique among active Records in the Registry.
	Name string
	// Description is optional Record display text.
	Description string
	// VersionName is an optional name for revision one.
	VersionName string
	// DescriptorType selects exactly one matching source field.
	DescriptorType RegistryDescriptorType
	// MCPSource is valid only for MCP Records.
	MCPSource *RegistryMCPSource
	// AgentSource is valid only for A2A or AGUI Records.
	AgentSource *RegistryAgentSource
	// SkillSource is valid only for AGENT_SKILLS Records.
	SkillSource *RegistrySkillSource
	// CustomDescriptors is a bounded JSON object for CUSTOM Records.
	CustomDescriptors json.RawMessage
}

// RegistryVersionSelector resolves one Version. VersionID and Label are mutually exclusive.
// A nil selector means the service default stable label.
type RegistryVersionSelector struct {
	// VersionID selects one exact Version.
	VersionID string
	// Label selects the Version currently bound to one label.
	Label string
}

// RegistryRecordInfo is immutable Record metadata.
type RegistryRecordInfo struct {
	// ID is the service-generated Record identity.
	ID string
	// RegistryID identifies the owning Registry.
	RegistryID string
	// Name is the Record name.
	Name string
	// Description is the Record description.
	Description string
	// DescriptorType is fixed at creation.
	DescriptorType RegistryDescriptorType
	// Status preserves known and future service values.
	Status RegistryRecordStatus
	// AppID is the owning account identity.
	AppID int64
	// CreatorUIN is the creator's primary account identity.
	CreatorUIN string
	// CreatorSubUIN is the creator's sub-account identity.
	CreatorSubUIN string
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
	// VersionCount is present when reported by the service.
	VersionCount *int64
	// Labels contains all label names known to the Record.
	Labels []string
	// RequestID is service correlation data.
	RequestID string
}

// RegistryApprovalAction is one immutable Version decision entry.
type RegistryApprovalAction struct {
	// ID is the decision identity.
	ID string
	// Type preserves the service action type.
	Type string
	// ActorType preserves the service actor category.
	ActorType string
	// ActorUIN is the actor's primary account identity.
	ActorUIN string
	// ActorSubUIN is the actor's sub-account identity.
	ActorSubUIN string
	// Comment is the decision comment.
	Comment string
	// RequestID correlates the original decision request when reported.
	RequestID string
	// CreatedAt is the decision timestamp.
	CreatedAt time.Time
}

// RegistryRecordVersionInfo is an immutable Version snapshot.
type RegistryRecordVersionInfo struct {
	// ID is the Version identity.
	ID string
	// RecordID identifies the owning Record.
	RecordID string
	// Revision is the monotonic Record revision.
	Revision int64
	// Status preserves known and future Version states.
	Status RegistryVersionStatus
	// ApprovalMode is the mode captured when this Version was created.
	ApprovalMode RegistryApprovalMode
	// AppID is the owning account identity.
	AppID int64
	// CreatorUIN is the creator's primary account identity.
	CreatorUIN string
	// CreatorSubUIN is the creator's sub-account identity.
	CreatorSubUIN string
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
	// Name is the optional Version display name.
	Name string
	// Descriptors is a bounded JSON object.
	Descriptors json.RawMessage
	// SourceConfig is a bounded normalized source JSON object.
	SourceConfig json.RawMessage
	// SourceType preserves the service source type.
	SourceType RegistrySourceType
	// ContentStatus preserves the package content state when present.
	ContentStatus string
	// ContentSHA256 is the ready content digest when present.
	ContentSHA256 string
	// ContentSizeBytes is present when reported by the service.
	ContentSizeBytes *int64
	// ConfigSHA256 is the normalized configuration digest.
	ConfigSHA256 string
	// ChangeLog is the immutable Version change description.
	ChangeLog string
	// ContentReadyAt is the package readiness timestamp when present.
	ContentReadyAt time.Time
	// ApprovalActions contains the decision history.
	ApprovalActions []RegistryApprovalAction
	// Labels contains labels currently bound to this Version.
	Labels []string
	// RequestID is service correlation data.
	RequestID string
}

// RegistryRecordResult combines Record metadata and its resolved or newly created Version.
type RegistryRecordResult struct {
	// Record is the current Record metadata.
	Record RegistryRecordInfo
	// Version is present for reads and Version creation; metadata-only updates leave it nil.
	Version *RegistryRecordVersionInfo
	// ResolvedBy preserves the service resolution mode.
	ResolvedBy string
	// ResolvedLabel contains the selected label when applicable.
	ResolvedLabel string
	// RequestID is service correlation data.
	RequestID string
}

// RegistryRecordListOptions selects one Record offset page.
type RegistryRecordListOptions struct {
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
	// Filters are evaluated by the service.
	Filters []RegistryFilter
}

// RegistryRecordPage is one service-reported Record page.
type RegistryRecordPage struct {
	// Items contains Record snapshots.
	Items []RegistryRecordInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// RegistryLabelMutation mutates one label while preserving optional VersionID presence.
type RegistryLabelMutation struct {
	// Operation selects SET or DELETE.
	Operation RegistryLabelOperation
	// Name is the normalized label name.
	Name string
	// VersionID is optional for SET and forbidden for DELETE.
	VersionID *string
	// Reason is optional mutation context.
	Reason string
}

// UpdateRegistryRecordOptions either updates Record metadata/labels or creates a Version.
// Metadata fields and Version source fields cannot be combined in one call.
type UpdateRegistryRecordOptions struct {
	// Description preserves omission versus explicit clearing.
	Description *string
	// LabelMutations contains at most 32 unique label operations.
	LabelMutations []RegistryLabelMutation
	// VersionName names a newly created Version.
	VersionName string
	// ChangeLog describes a newly created Version.
	ChangeLog string
	// MCPSource creates a new MCP Version.
	MCPSource *RegistryMCPSource
	// AgentSource creates a new A2A or AGUI Version.
	AgentSource *RegistryAgentSource
	// SkillSource creates a new inline Skill Version.
	SkillSource *RegistrySkillSource
	// CustomDescriptors creates a new CUSTOM Version.
	CustomDescriptors json.RawMessage
}

// DeleteRegistryRecordOptions deletes the whole Record when Version is nil. A non-nil Version
// must contain VersionID only; an explicitly empty selector is rejected locally.
type DeleteRegistryRecordOptions struct {
	// Version selects a single Version; nil selects the whole Record.
	Version *RegistryVersionSelector
	// Reason is required when deleting a Version.
	Reason string
}

// RegistryVersionListOptions selects one Version offset page.
type RegistryVersionListOptions struct {
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
	// Filters are evaluated by the service.
	Filters []RegistryFilter
}

// RegistryRecordVersionPage is one service-reported Version page.
type RegistryRecordVersionPage struct {
	// Items contains Version snapshots.
	Items []RegistryRecordVersionInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// RegistryAuditLogListOptions filters Registry audit entries.
type RegistryAuditLogListOptions struct {
	// RecordID optionally narrows the query to one Record.
	RecordID string
	// VersionID optionally narrows the query to one Version.
	VersionID string
	// Action selects one exact service action.
	Action string
	// Actor selects one exact account actor.
	Actor string
	// Start is the optional inclusive window start.
	Start *time.Time
	// End is the optional window end.
	End *time.Time
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
}

// RegistryAuditLogInfo is one immutable, redacted service audit entry.
type RegistryAuditLogInfo struct {
	// ID is the audit entry identity.
	ID string
	// RegistryID identifies the owning Registry.
	RegistryID string
	// RecordID identifies the related Record when present.
	RecordID string
	// VersionID identifies the related Version when present.
	VersionID string
	// Actor is the account identity that initiated the action.
	Actor string
	// Action is the recorded service action.
	Action string
	// Details is a bounded, redacted JSON object.
	Details json.RawMessage
	// CreatedAt is the action timestamp.
	CreatedAt time.Time
}

// RegistryAuditLogPage is one service-reported audit page.
type RegistryAuditLogPage struct {
	// Items contains audit snapshots.
	Items []RegistryAuditLogInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// RegistryPreviewInfo is a bounded, read-only remote preview result.
type RegistryPreviewInfo struct {
	// Result is the bounded preview JSON object.
	Result json.RawMessage
	// ResolvedVersionID is the Version inspected by the service.
	ResolvedVersionID string
	// RequestID is service correlation data.
	RequestID string
}

// RegistrySyncResult describes an explicit remote synchronization mutation.
type RegistrySyncResult struct {
	// Status preserves the service synchronization outcome.
	Status RegistrySyncStatus
	// ResolvedVersionID is the Version used as the synchronization source.
	ResolvedVersionID string
	// CreatedVersion is present when synchronization created a Version.
	CreatedVersion *RegistryRecordVersionInfo
	// Record is present when synchronization changed the Record.
	Record *RegistryRecordInfo
	// LastSyncAt is the service synchronization timestamp when present.
	LastSyncAt time.Time
	// ErrorCode is populated for a FAILED service outcome.
	ErrorCode string
	// ErrorMessage is populated for a FAILED service outcome.
	ErrorMessage string
	// RequestID is service correlation data.
	RequestID string
}

// RegistrySkillPackageUploadOptions bounds one streaming gzip tar upload.
type RegistrySkillPackageUploadOptions struct {
	// SizeBytes is the exact number of bytes uploaded and must be positive. Bytes after this
	// declared length remain unread.
	SizeBytes int64
	// MaxBytes is a caller safety limit. Zero uses DefaultMaxRegistrySkillPackageBytes.
	MaxBytes int64
}

// RegistrySkillPackageUploadResult reports local transfer evidence without exposing the signed URL.
type RegistrySkillPackageUploadResult struct {
	// Version is the existing TAR_PACKAGE Version selected for upload.
	Version RegistryRecordVersionInfo
	// ContentStatus is the status reported when the service issued the upload attempt.
	ContentStatus string
	// SizeBytes is the number of bytes sent.
	SizeBytes int64
	// SHA256 is the lowercase digest computed while streaming the upload.
	SHA256 string
	// RequestID correlates the URL-issuance control-plane request.
	RequestID string
}

// RegistrySkillPackageDownloadOptions bounds one streaming package download.
type RegistrySkillPackageDownloadOptions struct {
	// MaxBytes is a caller safety limit. Zero uses DefaultMaxRegistrySkillPackageBytes.
	MaxBytes int64
}

// RegistrySkillPackageDownloadResult reports a verified package transfer.
type RegistrySkillPackageDownloadResult struct {
	// ResolvedVersionID is the exact Version selected by VersionID, Label, or Stable default.
	ResolvedVersionID string
	// SizeBytes is the number of bytes written.
	SizeBytes int64
	// SHA256 is the lowercase service digest verified against the downloaded bytes.
	SHA256 string
	// RequestID correlates the URL-issuance control-plane request.
	RequestID string
}

type registryControlPlane interface {
	CreateRegistry(context.Context, CreateRegistryOptions) (RegistryInfo, error)
	GetRegistry(context.Context, string) (RegistryInfo, error)
	ListRegistries(context.Context, RegistryListOptions) (RegistryPage, error)
	UpdateRegistry(context.Context, string, UpdateRegistryOptions) (RegistryInfo, error)
	DeleteRegistry(context.Context, string) error
	CreateRegistryRecord(context.Context, string, CreateRegistryRecordOptions) (RegistryRecordResult, error)
	GetRegistryRecord(context.Context, string, string, *RegistryVersionSelector) (RegistryRecordResult, error)
	ListRegistryRecords(context.Context, string, RegistryRecordListOptions) (RegistryRecordPage, error)
	UpdateRegistryRecord(context.Context, string, string, UpdateRegistryRecordOptions) (RegistryRecordResult, error)
	DeleteRegistryRecord(context.Context, string, string, DeleteRegistryRecordOptions) error
	ListRegistryRecordVersions(context.Context, string, string, RegistryVersionListOptions) (RegistryRecordVersionPage, error)
	DecideRegistryRecord(context.Context, string, string, string, string, string) (RegistryRecordVersionInfo, error)
	PreviewRegistryRecord(context.Context, string, string, *RegistryVersionSelector) (RegistryPreviewInfo, error)
	SyncRegistryRecord(context.Context, string, string, *RegistryVersionSelector, string) (RegistrySyncResult, error)
	ListRegistryAuditLogs(context.Context, string, RegistryAuditLogListOptions) (RegistryAuditLogPage, error)
	UploadRegistrySkillPackage(context.Context, string, string, string, io.Reader, int64, int64) (RegistrySkillPackageUploadResult, error)
	DownloadRegistrySkillPackage(context.Context, string, string, *RegistryVersionSelector, io.Writer, int64) (RegistrySkillPackageDownloadResult, error)
}

// RegistryManager manages Preview Registry resources.
type RegistryManager struct{ client *Client }

// RegistryRecordManager manages Records bound to exactly one Registry.
type RegistryRecordManager struct {
	client     *Client
	registryID string
}

func (m *RegistryManager) transport(operation string) (registryControlPlane, error) {
	cp, ok := m.client.cfg.control.(registryControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "REGISTRY_CONTROL_UNAVAILABLE")
	}
	return cp, nil
}

func (m *RegistryRecordManager) transport(operation string) (registryControlPlane, error) {
	if strings.TrimSpace(m.registryID) == "" {
		return nil, codeError(InvalidArgument, operation, "REGISTRY_ID_REQUIRED")
	}
	cp, ok := m.client.cfg.control.(registryControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "REGISTRY_CONTROL_UNAVAILABLE")
	}
	return cp, nil
}

// Records returns a Record manager bound to registryID. Methods reject an empty ID locally.
func (m *RegistryManager) Records(registryID string) *RegistryRecordManager {
	return &RegistryRecordManager{client: m.client, registryID: registryID}
}

// Create creates one Registry.
func (m *RegistryManager) Create(ctx context.Context, opts CreateRegistryOptions) (RegistryInfo, error) {
	op := "Registries.Create"
	opts.Tags = append([]RegistryTag(nil), opts.Tags...)
	if strings.TrimSpace(opts.Name) == "" || utf8.RuneCountInString(opts.Name) > 255 || utf8.RuneCountInString(opts.Description) > 4096 || (opts.ApprovalMode != "" && opts.ApprovalMode != RegistryApprovalAuto && opts.ApprovalMode != RegistryApprovalManual) || !validRegistryTags(opts.Tags) {
		return RegistryInfo{}, codeError(InvalidArgument, op, "OPTIONS_INVALID")
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryInfo{}, err
	}
	return cp.CreateRegistry(ctx, opts)
}

// Get returns one Registry by ID.
func (m *RegistryManager) Get(ctx context.Context, id string) (RegistryInfo, error) {
	op := "Registries.Get"
	if strings.TrimSpace(id) == "" {
		return RegistryInfo{}, codeError(InvalidArgument, op, "REGISTRY_ID_REQUIRED")
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryInfo{}, err
	}
	return cp.GetRegistry(ctx, id)
}

// List returns one server-filtered Registry page.
func (m *RegistryManager) List(ctx context.Context, opts RegistryListOptions) (RegistryPage, error) {
	op := "Registries.List"
	opts.Filters = copyRegistryFilters(opts.Filters)
	if err := validateRegistryList(opts.Offset, opts.Limit, opts.Filters, map[string]bool{"name": true, "search": true, "archived": true, "status": true, "tag-key": true}, true, op); err != nil {
		return RegistryPage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryPage{}, err
	}
	return cp.ListRegistries(ctx, opts)
}

// Update replaces one Registry description, including an explicit empty value.
func (m *RegistryManager) Update(ctx context.Context, id string, opts UpdateRegistryOptions) (RegistryInfo, error) {
	op := "Registries.Update"
	if strings.TrimSpace(id) == "" || utf8.RuneCountInString(opts.Description) > 4096 {
		return RegistryInfo{}, codeError(InvalidArgument, op, "OPTIONS_INVALID")
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryInfo{}, err
	}
	return cp.UpdateRegistry(ctx, id, opts)
}

// Delete deletes an empty Registry.
func (m *RegistryManager) Delete(ctx context.Context, id string) error {
	op := "Registries.Delete"
	if strings.TrimSpace(id) == "" {
		return codeError(InvalidArgument, op, "REGISTRY_ID_REQUIRED")
	}
	cp, err := m.transport(op)
	if err != nil {
		return err
	}
	return cp.DeleteRegistry(ctx, id)
}

// Create creates a Record and its first Version.
func (m *RegistryRecordManager) Create(ctx context.Context, opts CreateRegistryRecordOptions) (RegistryRecordResult, error) {
	op := "Registries.Records.Create"
	opts = copyCreateRegistryRecordOptions(opts)
	if err := validateCreateRegistryRecord(opts, op); err != nil {
		return RegistryRecordResult{}, err
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordResult{}, err
	}
	return cp.CreateRegistryRecord(ctx, m.registryID, opts)
}

// Get returns a Record and one resolved Version. Nil selector means stable.
func (m *RegistryRecordManager) Get(ctx context.Context, recordID string, selector *RegistryVersionSelector) (RegistryRecordResult, error) {
	op := "Registries.Records.Get"
	selector = copyRegistrySelector(selector)
	if strings.TrimSpace(recordID) == "" {
		return RegistryRecordResult{}, codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	if err := validateRegistrySelector(selector, op, true); err != nil {
		return RegistryRecordResult{}, err
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordResult{}, err
	}
	return cp.GetRegistryRecord(ctx, m.registryID, recordID, selector)
}

// List returns one server-filtered Record page within this Registry.
func (m *RegistryRecordManager) List(ctx context.Context, opts RegistryRecordListOptions) (RegistryRecordPage, error) {
	op := "Registries.Records.List"
	opts.Filters = copyRegistryFilters(opts.Filters)
	allowed := map[string]bool{"name": true, "search": true, "descriptor_type": true, "lifecycle_status": true}
	if err := validateRegistryList(opts.Offset, opts.Limit, opts.Filters, allowed, false, op); err != nil {
		return RegistryRecordPage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordPage{}, err
	}
	return cp.ListRegistryRecords(ctx, m.registryID, opts)
}

// Update changes Record metadata/labels or creates one new Version without automatic retry.
func (m *RegistryRecordManager) Update(ctx context.Context, recordID string, opts UpdateRegistryRecordOptions) (RegistryRecordResult, error) {
	op := "Registries.Records.Update"
	if strings.TrimSpace(recordID) == "" {
		return RegistryRecordResult{}, codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	opts = copyUpdateRegistryRecordOptions(opts)
	if err := validateUpdateRegistryRecord(opts, op); err != nil {
		return RegistryRecordResult{}, err
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordResult{}, err
	}
	return cp.UpdateRegistryRecord(ctx, m.registryID, recordID, opts)
}

// Delete deletes the Record when Version is nil or exactly one Version when supplied.
func (m *RegistryRecordManager) Delete(ctx context.Context, recordID string, opts DeleteRegistryRecordOptions) error {
	op := "Registries.Records.Delete"
	opts.Version = copyRegistrySelector(opts.Version)
	if strings.TrimSpace(recordID) == "" {
		return codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	if opts.Version != nil && (strings.TrimSpace(opts.Version.VersionID) == "" || opts.Version.Label != "") {
		return codeError(InvalidArgument, op, "VERSION_SELECTOR_INVALID")
	}
	if opts.Version != nil && (strings.TrimSpace(opts.Reason) == "" || utf8.RuneCountInString(opts.Reason) > 4096) {
		return codeError(InvalidArgument, op, "REASON_INVALID")
	}
	if opts.Version == nil && utf8.RuneCountInString(opts.Reason) > 4096 {
		return codeError(InvalidArgument, op, "REASON_INVALID")
	}
	cp, err := m.transport(op)
	if err != nil {
		return err
	}
	return cp.DeleteRegistryRecord(ctx, m.registryID, recordID, opts)
}

// ListVersions returns one Version page for recordID.
func (m *RegistryRecordManager) ListVersions(ctx context.Context, recordID string, opts RegistryVersionListOptions) (RegistryRecordVersionPage, error) {
	op := "Registries.Records.ListVersions"
	if strings.TrimSpace(recordID) == "" {
		return RegistryRecordVersionPage{}, codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	opts.Filters = copyRegistryFilters(opts.Filters)
	if err := validateRegistryList(opts.Offset, opts.Limit, opts.Filters, map[string]bool{"status": true, "source_type": true}, false, op); err != nil {
		return RegistryRecordVersionPage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordVersionPage{}, err
	}
	return cp.ListRegistryRecordVersions(ctx, m.registryID, recordID, opts)
}

// Approve approves one pending Version.
func (m *RegistryRecordManager) Approve(ctx context.Context, recordID, versionID, comment string) (RegistryRecordVersionInfo, error) {
	return m.decide(ctx, "approve", recordID, versionID, comment)
}

// Reject rejects one pending Version.
func (m *RegistryRecordManager) Reject(ctx context.Context, recordID, versionID, comment string) (RegistryRecordVersionInfo, error) {
	return m.decide(ctx, "reject", recordID, versionID, comment)
}

// Cancel cancels one pending Version.
func (m *RegistryRecordManager) Cancel(ctx context.Context, recordID, versionID, comment string) (RegistryRecordVersionInfo, error) {
	return m.decide(ctx, "cancel", recordID, versionID, comment)
}

func (m *RegistryRecordManager) decide(ctx context.Context, decision, recordID, versionID, comment string) (RegistryRecordVersionInfo, error) {
	op := "Registries.Records." + strings.ToUpper(decision[:1]) + decision[1:]
	if strings.TrimSpace(recordID) == "" || strings.TrimSpace(versionID) == "" || strings.TrimSpace(comment) == "" || utf8.RuneCountInString(comment) > 4096 {
		return RegistryRecordVersionInfo{}, codeError(InvalidArgument, op, "DECISION_INVALID")
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryRecordVersionInfo{}, err
	}
	return cp.DecideRegistryRecord(ctx, decision, m.registryID, recordID, versionID, comment)
}

// Preview performs one explicit read-only remote metadata preview.
func (m *RegistryRecordManager) Preview(ctx context.Context, recordID string, selector *RegistryVersionSelector) (RegistryPreviewInfo, error) {
	op := "Registries.Records.Preview"
	selector = copyRegistrySelector(selector)
	if strings.TrimSpace(recordID) == "" {
		return RegistryPreviewInfo{}, codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	if err := validateRegistrySelector(selector, op, true); err != nil {
		return RegistryPreviewInfo{}, err
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryPreviewInfo{}, err
	}
	return cp.PreviewRegistryRecord(ctx, m.registryID, recordID, selector)
}

// Sync performs one explicit remote synchronization mutation without automatic retry.
func (m *RegistryRecordManager) Sync(ctx context.Context, recordID string, selector *RegistryVersionSelector, changeLog string) (RegistrySyncResult, error) {
	op := "Registries.Records.Sync"
	selector = copyRegistrySelector(selector)
	if strings.TrimSpace(recordID) == "" {
		return RegistrySyncResult{}, codeError(InvalidArgument, op, "RECORD_ID_REQUIRED")
	}
	if err := validateRegistrySelector(selector, op, true); err != nil {
		return RegistrySyncResult{}, err
	}
	if utf8.RuneCountInString(changeLog) > 4096 {
		return RegistrySyncResult{}, codeError(InvalidArgument, op, "CHANGE_LOG_TOO_LONG")
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistrySyncResult{}, err
	}
	return cp.SyncRegistryRecord(ctx, m.registryID, recordID, selector, changeLog)
}

// UploadSkillPackage streams one gzip tar archive to an existing TAR_PACKAGE Version. The SDK
// obtains and consumes the short-lived upload URL internally and never returns it. The upload is
// not retried. Success means the bytes were accepted by object storage; call Get to observe the
// service's asynchronous ContentStatus transition to READY.
func (m *RegistryRecordManager) UploadSkillPackage(ctx context.Context, recordID, versionID string, body io.Reader, opts RegistrySkillPackageUploadOptions) (RegistrySkillPackageUploadResult, error) {
	op := "Registries.Records.UploadSkillPackage"
	if strings.TrimSpace(recordID) == "" || strings.TrimSpace(versionID) == "" || body == nil || opts.SizeBytes <= 0 {
		return RegistrySkillPackageUploadResult{}, codeError(InvalidArgument, op, "SKILL_PACKAGE_UPLOAD_INVALID")
	}
	max, err := registrySkillPackageMax(opts.MaxBytes, op)
	if err != nil {
		return RegistrySkillPackageUploadResult{}, err
	}
	if opts.SizeBytes > max {
		return RegistrySkillPackageUploadResult{}, &Error{Code: ResourceExhausted, Operation: op, Reason: "SKILL_PACKAGE_TOO_LARGE", Retryable: false}
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistrySkillPackageUploadResult{}, err
	}
	return cp.UploadRegistrySkillPackage(ctx, m.registryID, recordID, versionID, body, opts.SizeBytes, max)
}

// DownloadSkillPackage streams and verifies one READY TAR_PACKAGE Version. Nil selector uses the
// Stable service default. A failed call may have written a prefix to dst; use a temporary file when
// atomic replacement is required. The SDK does not extract the archive.
func (m *RegistryRecordManager) DownloadSkillPackage(ctx context.Context, recordID string, selector *RegistryVersionSelector, dst io.Writer, opts RegistrySkillPackageDownloadOptions) (RegistrySkillPackageDownloadResult, error) {
	op := "Registries.Records.DownloadSkillPackage"
	selector = copyRegistrySelector(selector)
	if strings.TrimSpace(recordID) == "" || dst == nil {
		return RegistrySkillPackageDownloadResult{}, codeError(InvalidArgument, op, "SKILL_PACKAGE_DOWNLOAD_INVALID")
	}
	if err := validateRegistrySelector(selector, op, true); err != nil {
		return RegistrySkillPackageDownloadResult{}, err
	}
	max, err := registrySkillPackageMax(opts.MaxBytes, op)
	if err != nil {
		return RegistrySkillPackageDownloadResult{}, err
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistrySkillPackageDownloadResult{}, err
	}
	return cp.DownloadRegistrySkillPackage(ctx, m.registryID, recordID, selector, dst, max)
}

// ListAuditLogs returns one Registry audit page.
func (m *RegistryRecordManager) ListAuditLogs(ctx context.Context, opts RegistryAuditLogListOptions) (RegistryAuditLogPage, error) {
	op := "Registries.Records.ListAuditLogs"
	if opts.Offset < 0 || opts.Limit < 0 || opts.Limit > 100 || (opts.Start != nil && opts.Start.IsZero()) || (opts.End != nil && opts.End.IsZero()) || (opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End)) {
		return RegistryAuditLogPage{}, codeError(InvalidArgument, op, "OPTIONS_INVALID")
	}
	if opts.Start != nil {
		value := *opts.Start
		opts.Start = &value
	}
	if opts.End != nil {
		value := *opts.End
		opts.End = &value
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport(op)
	if err != nil {
		return RegistryAuditLogPage{}, err
	}
	return cp.ListRegistryAuditLogs(ctx, m.registryID, opts)
}

func validateCreateRegistryRecord(opts CreateRegistryRecordOptions, op string) error {
	if strings.TrimSpace(opts.Name) == "" || utf8.RuneCountInString(opts.Name) > 255 || utf8.RuneCountInString(opts.Description) > 4096 {
		return codeError(InvalidArgument, op, "OPTIONS_INVALID")
	}
	return validateRegistrySource(opts.DescriptorType, opts.MCPSource, opts.AgentSource, opts.SkillSource, opts.CustomDescriptors, op)
}

func validateUpdateRegistryRecord(opts UpdateRegistryRecordOptions, op string) error {
	metadata := opts.Description != nil || len(opts.LabelMutations) > 0
	version := opts.MCPSource != nil || opts.AgentSource != nil || opts.SkillSource != nil || len(opts.CustomDescriptors) != 0 || opts.VersionName != "" || opts.ChangeLog != ""
	if metadata == version {
		return codeError(InvalidArgument, op, "UPDATE_MODE_INVALID")
	}
	if metadata {
		if opts.Description != nil && utf8.RuneCountInString(*opts.Description) > 4096 {
			return codeError(InvalidArgument, op, "DESCRIPTION_TOO_LONG")
		}
		if len(opts.LabelMutations) > 32 {
			return codeError(InvalidArgument, op, "LABEL_MUTATIONS_INVALID")
		}
		seen := map[string]bool{}
		for _, item := range opts.LabelMutations {
			name := strings.ToLower(item.Name)
			if item.Name != name || !registryLabelPattern.MatchString(name) || seen[name] || (item.Operation != RegistryLabelSet && item.Operation != RegistryLabelDelete) {
				return codeError(InvalidArgument, op, "LABEL_MUTATIONS_INVALID")
			}
			seen[name] = true
			if !validRegistryLabelMutation(item, name) {
				return codeError(InvalidArgument, op, "LABEL_MUTATIONS_INVALID")
			}
		}
		return nil
	}
	if utf8.RuneCountInString(opts.ChangeLog) > 4096 {
		return codeError(InvalidArgument, op, "CHANGE_LOG_TOO_LONG")
	}
	count := 0
	if opts.MCPSource != nil {
		count++
	}
	if opts.AgentSource != nil {
		count++
	}
	if opts.SkillSource != nil {
		count++
	}
	if len(opts.CustomDescriptors) != 0 {
		count++
	}
	if count != 1 {
		return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
	}
	if opts.MCPSource != nil {
		return validateRegistrySource(RegistryDescriptorMCP, opts.MCPSource, nil, nil, nil, op)
	}
	if opts.AgentSource != nil {
		return validateRegistrySource(RegistryDescriptorA2A, nil, opts.AgentSource, nil, nil, op)
	}
	if opts.SkillSource != nil {
		return validateRegistrySource(RegistryDescriptorAgentSkills, nil, nil, opts.SkillSource, nil, op)
	}
	return validateRegistryJSON(opts.CustomDescriptors, op, "CUSTOM_DESCRIPTORS_INVALID")
}

func validRegistryLabelMutation(item RegistryLabelMutation, normalizedName string) bool {
	switch item.Operation {
	case RegistryLabelDelete:
		return item.VersionID == nil && normalizedName != "stable" && normalizedName != "latest"
	case RegistryLabelSet:
		if item.VersionID != nil && strings.TrimSpace(*item.VersionID) == "" {
			return false
		}
		return normalizedName != "stable" || item.VersionID != nil
	default:
		return false
	}
}

func validateRegistrySource(kind RegistryDescriptorType, mcp *RegistryMCPSource, agent *RegistryAgentSource, skill *RegistrySkillSource, custom json.RawMessage, op string) error {
	count := 0
	if mcp != nil {
		count++
	}
	if agent != nil {
		count++
	}
	if skill != nil {
		count++
	}
	if len(custom) != 0 {
		count++
	}
	if count != 1 {
		return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
	}
	switch kind {
	case RegistryDescriptorMCP:
		if mcp == nil {
			return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
		}
		return validateStructuredRegistrySource(mcp.Type, mcp.Descriptors, mcp.EndpointURL, op)
	case RegistryDescriptorA2A, RegistryDescriptorAGUI:
		if agent == nil {
			return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
		}
		return validateStructuredRegistrySource(agent.Type, agent.Descriptors, agent.EndpointURL, op)
	case RegistryDescriptorAgentSkills:
		if skill == nil {
			return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
		}
		switch skill.Type {
		case RegistrySourceManual:
			if strings.TrimSpace(skill.SkillMD) == "" || len(skill.SkillMD) > maxRegistryJSONBytes {
				return codeError(InvalidArgument, op, "SKILL_SOURCE_INVALID")
			}
		case RegistrySourceTarPackage:
			if skill.SkillMD != "" {
				return codeError(InvalidArgument, op, "SKILL_SOURCE_INVALID")
			}
		default:
			return codeError(InvalidArgument, op, "SKILL_SOURCE_INVALID")
		}
		return nil
	case RegistryDescriptorCustom:
		if len(custom) == 0 {
			return codeError(InvalidArgument, op, "SOURCE_UNION_INVALID")
		}
		return validateRegistryJSON(custom, op, "CUSTOM_DESCRIPTORS_INVALID")
	default:
		return codeError(InvalidArgument, op, "DESCRIPTOR_TYPE_INVALID")
	}
}

func registrySkillPackageMax(value int64, op string) (int64, error) {
	if value < 0 {
		return 0, codeError(InvalidArgument, op, "MAX_BYTES_INVALID")
	}
	if value == 0 {
		return DefaultMaxRegistrySkillPackageBytes, nil
	}
	return value, nil
}

func validateStructuredRegistrySource(kind RegistrySourceType, descriptors json.RawMessage, endpoint, op string) error {
	switch kind {
	case RegistrySourceManual:
		if endpoint != "" {
			return codeError(InvalidArgument, op, "SOURCE_INVALID")
		}
		return validateRegistryJSON(descriptors, op, "DESCRIPTORS_INVALID")
	case RegistrySourceURLImport:
		if len(descriptors) != 0 {
			return codeError(InvalidArgument, op, "SOURCE_INVALID")
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return codeError(InvalidArgument, op, "ENDPOINT_URL_INVALID")
		}
		return nil
	case RegistrySourceTarPackage:
		return codeError(Unsupported, op, "SKILL_PACKAGE_TRANSFER_UNAVAILABLE")
	default:
		return codeError(InvalidArgument, op, "SOURCE_TYPE_INVALID")
	}
}

func validateRegistrySelector(selector *RegistryVersionSelector, op string, allowNil bool) error {
	if selector == nil {
		if allowNil {
			return nil
		}
		return codeError(InvalidArgument, op, "VERSION_SELECTOR_REQUIRED")
	}
	version, label := strings.TrimSpace(selector.VersionID), strings.TrimSpace(selector.Label)
	if (version == "") == (label == "") {
		return codeError(InvalidArgument, op, "VERSION_SELECTOR_INVALID")
	}
	return nil
}

func validateRegistryJSON(value json.RawMessage, op, reason string) error {
	if len(value) == 0 || len(value) > maxRegistryJSONBytes || !json.Valid(value) {
		return codeError(InvalidArgument, op, reason)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil || object == nil {
		return codeError(InvalidArgument, op, reason)
	}
	return nil
}

func validateRegistryList(offset, limit int, filters []RegistryFilter, allowed map[string]bool, tags bool, op string) error {
	if offset < 0 || limit < 0 || limit > 100 || len(filters) > 10 {
		return codeError(InvalidArgument, op, "PAGINATION_OR_FILTER_INVALID")
	}
	seen := map[string]bool{}
	tagGroups := 0
	for _, filter := range filters {
		valid := allowed[filter.Name]
		if tags && strings.HasPrefix(filter.Name, "tag:") && len(filter.Name) > 4 {
			valid = true
			tagGroups++
		}
		if !valid || seen[filter.Name] || len(filter.Values) == 0 || len(filter.Values) > 100 {
			return codeError(InvalidArgument, op, "FILTER_INVALID")
		}
		seen[filter.Name] = true
		for _, value := range filter.Values {
			if strings.TrimSpace(value) == "" {
				return codeError(InvalidArgument, op, "FILTER_INVALID")
			}
		}
	}
	if tagGroups > 6 {
		return codeError(InvalidArgument, op, "FILTER_INVALID")
	}
	return nil
}

func validRegistryTags(tags []RegistryTag) bool {
	if len(tags) > 10 {
		return false
	}
	seen := map[string]bool{}
	for _, tag := range tags {
		if strings.TrimSpace(tag.Key) == "" || seen[tag.Key] {
			return false
		}
		seen[tag.Key] = true
	}
	return true
}

func copyRegistryFilters(values []RegistryFilter) []RegistryFilter {
	out := append([]RegistryFilter(nil), values...)
	for i := range out {
		out[i].Values = append([]string(nil), out[i].Values...)
	}
	return out
}

func copyRegistrySelector(value *RegistryVersionSelector) *RegistryVersionSelector {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func copyCreateRegistryRecordOptions(value CreateRegistryRecordOptions) CreateRegistryRecordOptions {
	value.CustomDescriptors = append(json.RawMessage(nil), value.CustomDescriptors...)
	if value.MCPSource != nil {
		v := *value.MCPSource
		v.Descriptors = append(json.RawMessage(nil), v.Descriptors...)
		value.MCPSource = &v
	}
	if value.AgentSource != nil {
		v := *value.AgentSource
		v.Descriptors = append(json.RawMessage(nil), v.Descriptors...)
		value.AgentSource = &v
	}
	if value.SkillSource != nil {
		v := *value.SkillSource
		value.SkillSource = &v
	}
	return value
}

func copyUpdateRegistryRecordOptions(value UpdateRegistryRecordOptions) UpdateRegistryRecordOptions {
	value.CustomDescriptors = append(json.RawMessage(nil), value.CustomDescriptors...)
	value.LabelMutations = append([]RegistryLabelMutation(nil), value.LabelMutations...)
	for i := range value.LabelMutations {
		if value.LabelMutations[i].VersionID != nil {
			v := *value.LabelMutations[i].VersionID
			value.LabelMutations[i].VersionID = &v
		}
	}
	if value.Description != nil {
		v := *value.Description
		value.Description = &v
	}
	created := copyCreateRegistryRecordOptions(CreateRegistryRecordOptions{MCPSource: value.MCPSource, AgentSource: value.AgentSource, SkillSource: value.SkillSource})
	value.MCPSource, value.AgentSource, value.SkillSource = created.MCPSource, created.AgentSource, created.SkillSource
	return value
}
