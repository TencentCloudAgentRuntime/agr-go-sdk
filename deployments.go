package agr

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// DeploymentManager provides Cloud control and managed data-plane connections for Deployments.
type DeploymentManager struct{ client *Client }

// DeploymentState preserves service states, including values added after this SDK release.
type DeploymentState string

const (
	// DeploymentActive means the stable entry point accepts data-plane traffic.
	DeploymentActive DeploymentState = "ACTIVE"
	// DeploymentDeleting means the entry point is closed while deletion is in progress.
	DeploymentDeleting DeploymentState = "DELETING"
	// DeploymentDeleteFailed means the last asynchronous deletion attempt failed.
	DeploymentDeleteFailed DeploymentState = "DELETE_FAILED"
)

// DeploymentAffinityMode selects how requests reuse managed Sandbox instances.
type DeploymentAffinityMode string

const (
	// DeploymentAffinityBestEffort permits reassignment when the previous instance is unavailable.
	DeploymentAffinityBestEffort DeploymentAffinityMode = "BEST_EFFORT"
	// DeploymentAffinityStrict rejects the request when the previous instance is unavailable.
	DeploymentAffinityStrict DeploymentAffinityMode = "STRICT"
	// DeploymentAffinityExclusive dedicates one instance to one affinity ID without migration.
	DeploymentAffinityExclusive DeploymentAffinityMode = "EXCLUSIVE"
)

// DeploymentIdleAction selects what happens to an idle managed Sandbox instance.
type DeploymentIdleAction string

const (
	// DeploymentIdleStop releases an idle managed Sandbox instance.
	DeploymentIdleStop DeploymentIdleAction = "STOP"
	// DeploymentIdlePause preserves an idle managed Sandbox instance in paused state.
	DeploymentIdlePause DeploymentIdleAction = "PAUSE"
)

// DeploymentTag is one complete Deployment tag pair.
type DeploymentTag struct {
	// Key is the tag key.
	Key string
	// Value is the tag value.
	Value string
}

// CreateDeploymentScaling preserves omitted create-time defaults.
type CreateDeploymentScaling struct {
	// MinInstances is the optional active-instance lower bound.
	MinInstances *int64
	// MaxInstances is the optional active-instance upper bound.
	MaxInstances *int64
	// MaxConcurrency is the optional per-instance request or connection lease limit.
	MaxConcurrency *int64
}

// DeploymentScaling is the complete materialized capacity configuration.
type DeploymentScaling struct {
	// MinInstances is the active-instance lower bound.
	MinInstances int64
	// MaxInstances is the active-instance upper bound.
	MaxInstances int64
	// MaxConcurrency is the per-instance request or connection lease limit.
	MaxConcurrency int64
}

// CreateDeploymentLifecycle preserves omitted create-time defaults.
type CreateDeploymentLifecycle struct {
	// IdleTimeoutSeconds is the optional delay before applying IdleAction.
	IdleTimeoutSeconds *int64
	// IdleAction is the optional action applied after the idle timeout.
	IdleAction *DeploymentIdleAction
}

// DeploymentLifecycle is the complete materialized idle policy.
type DeploymentLifecycle struct {
	// IdleTimeoutSeconds is the delay before applying IdleAction.
	IdleTimeoutSeconds int64
	// IdleAction is the action applied after the idle timeout.
	IdleAction DeploymentIdleAction
}

// DeploymentAffinity configures or reports connection affinity. Empty Mode disables affinity.
type DeploymentAffinity struct {
	// Mode selects affinity behavior; an empty value disables affinity.
	Mode DeploymentAffinityMode
	// HeaderName is the application-visible header used to carry affinity IDs.
	HeaderName string
}

// CreateDeploymentOptions configures one new stable Deployment entry point.
type CreateDeploymentOptions struct {
	// Name is a unique DNS-1123 Deployment name.
	Name string
	// ToolID selects the Sandbox Tool served by this Deployment.
	ToolID string
	// Scaling optionally overrides server scaling defaults.
	Scaling *CreateDeploymentScaling
	// Lifecycle optionally overrides server idle-policy defaults.
	Lifecycle *CreateDeploymentLifecycle
	// Affinity optionally enables managed instance affinity.
	Affinity *DeploymentAffinity
	// Tags is the initial complete tag set.
	Tags []DeploymentTag
}

// UpdateDeploymentOptions contains complete replacement sections for one Deployment.
type UpdateDeploymentOptions struct {
	// Scaling replaces all scaling fields when non-nil.
	Scaling *DeploymentScaling
	// Lifecycle replaces all lifecycle fields when non-nil.
	Lifecycle *DeploymentLifecycle
	// Tags replaces the complete set; a non-nil empty slice clears all tags.
	Tags *[]DeploymentTag
}

// DeploymentFilter is one server-side list predicate; filters are ANDed and Values are ORed.
type DeploymentFilter struct {
	// Name is one of deployment-id, deployment-name, deployment-name-like, tool-id, or status.
	Name string
	// Values contains one or more exact filter operands.
	Values []string
}

// DeploymentListOptions configures offset-based Deployment listing.
type DeploymentListOptions struct {
	// Offset is the zero-based result offset.
	Offset int
	// Limit is the page size from 1 through 200; zero selects 20.
	Limit int
	// Filters contains server-side predicates.
	Filters []DeploymentFilter
}

// DeploymentPage is one immutable page of Deployment snapshots.
type DeploymentPage struct {
	// Items contains the current page.
	Items []DeploymentInfo
	// TotalCount is the filtered count before pagination.
	TotalCount int64
	// NextOffset is present when another page may exist.
	NextOffset *int
	// RequestID correlates the list request.
	RequestID string
}

// DeploymentInfo is one immutable, fully materialized control-plane snapshot.
type DeploymentInfo struct {
	// ID is the stable Deployment identifier.
	ID string
	// Name is the immutable Deployment name.
	Name string
	// ToolID is the served Sandbox Tool identifier.
	ToolID string
	// StatusReason is the safe service summary for DELETE_FAILED.
	StatusReason string
	// State is preserved even when newer than this SDK.
	State DeploymentState
	// Scaling is the materialized capacity configuration.
	Scaling DeploymentScaling
	// Lifecycle is the materialized idle policy.
	Lifecycle DeploymentLifecycle
	// Affinity is nil when affinity is disabled.
	Affinity *DeploymentAffinity
	// Tags is the complete tag set.
	Tags []DeploymentTag
	// CreatedAt is the service creation time.
	CreatedAt time.Time
	// UpdatedAt is the latest successful configuration or state update time.
	UpdatedAt time.Time
	// RequestID correlates the Cloud response that produced this snapshot. It is empty for
	// snapshots embedded in a list; use DeploymentPage.RequestID for that request.
	RequestID string
}

type deploymentControlPlane interface {
	CreateDeployment(context.Context, CreateDeploymentOptions) (DeploymentInfo, error)
	GetDeployment(context.Context, string) (DeploymentInfo, error)
	ListDeployments(context.Context, DeploymentListOptions) (DeploymentPage, error)
	UpdateDeployment(context.Context, string, UpdateDeploymentOptions) (DeploymentInfo, error)
	DeleteDeployment(context.Context, string) error
	acquireDeploymentToken(context.Context, string) (deploymentAccess, error)
}

type deploymentAccess struct {
	token     string
	expiresAt time.Time
}

func (m *DeploymentManager) transport(operation string) (deploymentControlPlane, error) {
	value, ok := m.client.cfg.control.(deploymentControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "DEPLOYMENT_CONTROL_UNAVAILABLE")
	}
	return value, nil
}

// Create creates a Deployment and returns the service-materialized snapshot.
func (m *DeploymentManager) Create(ctx context.Context, opts CreateDeploymentOptions) (DeploymentInfo, error) {
	opts = copyCreateDeploymentOptions(opts)
	if err := validateCreateDeployment(opts); err != nil {
		return DeploymentInfo{}, err
	}
	cp, err := m.transport("Deployments.Create")
	if err != nil {
		return DeploymentInfo{}, err
	}
	return cp.CreateDeployment(ctx, opts)
}

// Get returns one Deployment by stable ID.
func (m *DeploymentManager) Get(ctx context.Context, id string) (DeploymentInfo, error) {
	if id == "" {
		return DeploymentInfo{}, codeError(InvalidArgument, "Deployments.Get", "DEPLOYMENT_ID_REQUIRED")
	}
	cp, err := m.transport("Deployments.Get")
	if err != nil {
		return DeploymentInfo{}, err
	}
	return cp.GetDeployment(ctx, id)
}

// List returns one offset-based page of Deployments.
func (m *DeploymentManager) List(ctx context.Context, opts DeploymentListOptions) (DeploymentPage, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 || opts.Limit < 1 || opts.Limit > 200 {
		return DeploymentPage{}, codeError(InvalidArgument, "Deployments.List", "PAGINATION_OUT_OF_RANGE")
	}
	allowed := map[string]bool{"deployment-id": true, "deployment-name": true, "deployment-name-like": true, "tool-id": true, "status": true}
	for _, filter := range opts.Filters {
		if !allowed[filter.Name] || len(filter.Values) == 0 {
			return DeploymentPage{}, codeError(InvalidArgument, "Deployments.List", "FILTER_INVALID")
		}
		for _, value := range filter.Values {
			if value == "" {
				return DeploymentPage{}, codeError(InvalidArgument, "Deployments.List", "FILTER_INVALID")
			}
		}
	}
	opts = copyDeploymentListOptions(opts)
	cp, err := m.transport("Deployments.List")
	if err != nil {
		return DeploymentPage{}, err
	}
	return cp.ListDeployments(ctx, opts)
}

// Update replaces the supplied configuration sections and returns the updated snapshot.
func (m *DeploymentManager) Update(ctx context.Context, id string, opts UpdateDeploymentOptions) (DeploymentInfo, error) {
	if id == "" {
		return DeploymentInfo{}, codeError(InvalidArgument, "Deployments.Update", "DEPLOYMENT_ID_REQUIRED")
	}
	opts = copyUpdateDeploymentOptions(opts)
	if err := validateUpdateDeployment(opts); err != nil {
		return DeploymentInfo{}, err
	}
	cp, err := m.transport("Deployments.Update")
	if err != nil {
		return DeploymentInfo{}, err
	}
	return cp.UpdateDeployment(ctx, id, opts)
}

// Delete submits asynchronous deletion. NotFound is idempotent; use WaitDeleted to confirm.
func (m *DeploymentManager) Delete(ctx context.Context, id string) error {
	if id == "" {
		return codeError(InvalidArgument, "Deployments.Delete", "DEPLOYMENT_ID_REQUIRED")
	}
	cp, err := m.transport("Deployments.Delete")
	if err != nil {
		return err
	}
	err = cp.DeleteDeployment(ctx, id)
	var failure *Error
	if errors.As(err, &failure) && failure.Code == NotFound {
		return nil
	}
	return err
}

// WaitFor polls until the Deployment reaches target or the context ends.
func (m *DeploymentManager) WaitFor(ctx context.Context, id string, target DeploymentState) (DeploymentInfo, error) {
	if id == "" {
		return DeploymentInfo{}, codeError(InvalidArgument, "Deployments.WaitFor", "DEPLOYMENT_ID_REQUIRED")
	}
	if target != DeploymentActive && target != DeploymentDeleting && target != DeploymentDeleteFailed {
		return DeploymentInfo{}, codeError(InvalidArgument, "Deployments.WaitFor", "DEPLOYMENT_STATE_INVALID")
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := m.Get(ctx, id)
		if err != nil {
			return DeploymentInfo{}, err
		}
		if info.State == target {
			return info, nil
		}
		if info.State == DeploymentDeleteFailed {
			return DeploymentInfo{}, withResourceError(codeError(Conflict, "Deployments.WaitFor", "DEPLOYMENT_DELETE_FAILED"), "deployment", id)
		}
		select {
		case <-ctx.Done():
			return DeploymentInfo{}, normalizeError("Deployments.WaitFor", ctx.Err())
		case <-ticker.C:
		}
	}
}

// WaitDeleted polls until Describe returns exact NotFound and rejects DELETE_FAILED.
func (m *DeploymentManager) WaitDeleted(ctx context.Context, id string) error {
	if id == "" {
		return codeError(InvalidArgument, "Deployments.WaitDeleted", "DEPLOYMENT_ID_REQUIRED")
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := m.Get(ctx, id)
		var failure *Error
		if errors.As(err, &failure) && failure.Code == NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if info.State == DeploymentDeleteFailed {
			return withResourceError(codeError(Conflict, "Deployments.WaitDeleted", "DEPLOYMENT_DELETE_FAILED"), "deployment", id)
		}
		select {
		case <-ctx.Done():
			return normalizeError("Deployments.WaitDeleted", ctx.Err())
		case <-ticker.C:
		}
	}
}

var deploymentNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$`)

func validateCreateDeployment(opts CreateDeploymentOptions) error {
	if !deploymentNamePattern.MatchString(opts.Name) {
		return codeError(InvalidArgument, "Deployments.Create", "DEPLOYMENT_NAME_INVALID")
	}
	if err := validateCreateDeploymentScaling(opts.Scaling, "Deployments.Create"); err != nil {
		return err
	}
	if err := validateCreateDeploymentLifecycle(opts.Lifecycle, "Deployments.Create"); err != nil {
		return err
	}
	return validateDeploymentAffinity(opts.Affinity, "Deployments.Create")
}

func validateCreateDeploymentScaling(value *CreateDeploymentScaling, operation string) error {
	if value == nil {
		return nil
	}
	if value.MinInstances != nil && *value.MinInstances < 0 || value.MaxInstances != nil && *value.MaxInstances < 1 || value.MaxConcurrency != nil && *value.MaxConcurrency < 1 {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_SCALING_INVALID")
	}
	if value.MinInstances != nil && value.MaxInstances != nil && *value.MaxInstances < *value.MinInstances {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_SCALING_INVALID")
	}
	return nil
}

func validateCreateDeploymentLifecycle(value *CreateDeploymentLifecycle, operation string) error {
	if value == nil {
		return nil
	}
	if value.IdleTimeoutSeconds != nil && *value.IdleTimeoutSeconds < 30 {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_LIFECYCLE_INVALID")
	}
	if value.IdleAction != nil && *value.IdleAction != DeploymentIdleStop && *value.IdleAction != DeploymentIdlePause {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_LIFECYCLE_INVALID")
	}
	return nil
}

func validateDeploymentAffinity(value *DeploymentAffinity, operation string) error {
	if value == nil {
		return nil
	}
	if value.Mode == "" {
		if value.HeaderName != "" {
			return codeError(InvalidArgument, operation, "DEPLOYMENT_AFFINITY_INVALID")
		}
		return nil
	}
	if value.Mode != DeploymentAffinityBestEffort && value.Mode != DeploymentAffinityStrict && value.Mode != DeploymentAffinityExclusive {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_AFFINITY_INVALID")
	}
	if value.HeaderName != "" && !validAffinityHeader(value.HeaderName) {
		return codeError(InvalidArgument, operation, "DEPLOYMENT_AFFINITY_HEADER_INVALID")
	}
	return nil
}

func validateUpdateDeployment(opts UpdateDeploymentOptions) error {
	if opts.Scaling == nil && opts.Lifecycle == nil && opts.Tags == nil {
		return codeError(InvalidArgument, "Deployments.Update", "DEPLOYMENT_UPDATE_EMPTY")
	}
	if opts.Scaling != nil && (opts.Scaling.MinInstances < 0 || opts.Scaling.MaxInstances < 1 || opts.Scaling.MaxInstances < opts.Scaling.MinInstances || opts.Scaling.MaxConcurrency < 1) {
		return codeError(InvalidArgument, "Deployments.Update", "DEPLOYMENT_SCALING_INVALID")
	}
	if opts.Lifecycle != nil && (opts.Lifecycle.IdleTimeoutSeconds < 30 || opts.Lifecycle.IdleAction != DeploymentIdleStop && opts.Lifecycle.IdleAction != DeploymentIdlePause) {
		return codeError(InvalidArgument, "Deployments.Update", "DEPLOYMENT_LIFECYCLE_INVALID")
	}
	return nil
}

func validAffinityHeader(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r > 127 || !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", r) {
			return false
		}
	}
	switch strings.ToLower(value) {
	case "x-access-token", "authorization", "host", "connection", "upgrade", "proxy-connection", "origin":
		return false
	}
	return true
}

func copyCreateDeploymentOptions(value CreateDeploymentOptions) CreateDeploymentOptions {
	value.Tags = append([]DeploymentTag(nil), value.Tags...)
	if value.Scaling != nil {
		copy := *value.Scaling
		copy.MinInstances, copy.MaxInstances, copy.MaxConcurrency = copyInt64(value.Scaling.MinInstances), copyInt64(value.Scaling.MaxInstances), copyInt64(value.Scaling.MaxConcurrency)
		value.Scaling = &copy
	}
	if value.Lifecycle != nil {
		copy := *value.Lifecycle
		copy.IdleTimeoutSeconds = copyInt64(value.Lifecycle.IdleTimeoutSeconds)
		if value.Lifecycle.IdleAction != nil {
			action := *value.Lifecycle.IdleAction
			copy.IdleAction = &action
		}
		value.Lifecycle = &copy
	}
	if value.Affinity != nil {
		copy := *value.Affinity
		value.Affinity = &copy
	}
	return value
}

func copyUpdateDeploymentOptions(value UpdateDeploymentOptions) UpdateDeploymentOptions {
	if value.Scaling != nil {
		copy := *value.Scaling
		value.Scaling = &copy
	}
	if value.Lifecycle != nil {
		copy := *value.Lifecycle
		value.Lifecycle = &copy
	}
	if value.Tags != nil {
		copy := append([]DeploymentTag(nil), (*value.Tags)...)
		value.Tags = &copy
	}
	return value
}

func copyDeploymentListOptions(value DeploymentListOptions) DeploymentListOptions {
	value.Filters = append([]DeploymentFilter(nil), value.Filters...)
	for i := range value.Filters {
		value.Filters[i].Values = append([]string(nil), value.Filters[i].Values...)
	}
	return value
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
