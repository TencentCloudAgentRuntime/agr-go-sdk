package agr

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	deploymentwire "github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/dataplane/deployment"
)

// ClientOption configures NewClient. Use the provided With functions; private configuration is
// not an extension API.
type ClientOption func(*clientConfig)
type clientConfig struct {
	region           string
	credential       CredentialProvider
	timeout          time.Duration
	control          controlPlane
	monitor          monitorTransport
	controlEndpoint  string
	monitorEndpoint  string
	dataPlaneDomain  string
	sandboxDomain    string
	sandboxDomainSet bool
	httpClient       *http.Client
}

// WithRegion selects the nonempty Tencent Cloud region, overriding TENCENTCLOUD_REGION.
func WithRegion(v string) ClientOption { return func(c *clientConfig) { c.region = v } }

// WithCredential selects Cloud control and Metrics using a static SecretID/SecretKey pair.
func WithCredential(v CloudCredential) ClientOption {
	return func(c *clientConfig) { c.credential = v }
}

// WithTemporaryCredential explicitly selects Cloud control using a static SecretID, SecretKey,
// and optional session Token.
func WithTemporaryCredential(v TemporaryCloudCredential) ClientOption {
	return func(c *clientConfig) { c.credential = v }
}

// WithCredentialProvider selects a concurrent, per-request credential source.
func WithCredentialProvider(v CredentialProvider) ClientOption {
	return func(c *clientConfig) { c.credential = v }
}
func withControlPlane(cp controlPlane) ClientOption { return func(c *clientConfig) { c.control = cp } }
func withMonitorTransport(m monitorTransport) ClientOption {
	return func(c *clientConfig) { c.monitor = m }
}

// WithRequestTimeout sets a positive per-request timeout (default 30 seconds). Use context
// deadlines to bound orchestration and stream lifetime; this is not a sandbox lifetime.
func WithRequestTimeout(v time.Duration) ClientOption { return func(c *clientConfig) { c.timeout = v } }

// WithControlPlaneEndpoint selects the official Cloud AGS endpoint (default
// ags.tencentcloudapi.com).
func WithControlPlaneEndpoint(v string) ClientOption {
	return func(c *clientConfig) { c.controlEndpoint = v }
}

// WithMonitorEndpoint selects the official Cloud Monitor endpoint (default
// monitor.tencentcloudapi.com).
func WithMonitorEndpoint(v string) ClientOption {
	return func(c *clientConfig) { c.monitorEndpoint = v }
}

// WithDataPlaneDomain selects the base domain used for Deployment data-plane connections.
// The default is tencentags.com. Pass a hostname only, without a scheme, port, or path.
func WithDataPlaneDomain(v string) ClientOption {
	return func(c *clientConfig) { c.dataPlaneDomain = v }
}

// WithSandboxDomain selects the complete Sandbox data-plane hostname suffix,
// such as ap-guangzhou.tencentags.com. It affects public port hosts and runtime
// connections, but not Cloud endpoints or Deployment data-plane connections.
func WithSandboxDomain(v string) ClientOption {
	return func(c *clientConfig) { c.sandboxDomain, c.sandboxDomainSet = v, true }
}

// WithHTTPClient supplies trusted HTTP configuration; nil uses SDK defaults. The SDK rejects
// redirects and reuses the transport without taking ownership of the shared client. Do not log
// or forward credentials in custom transports.
func WithHTTPClient(v *http.Client) ClientOption { return func(c *clientConfig) { c.httpClient = v } }

// noRedirectClient copies caller-supplied HTTP settings and forces redirects to be
// returned to the signed request path. Following a redirect could otherwise forward
// Cloud authorization headers to an endpoint that was not part of the signature.
func noRedirectClient(client *http.Client) *http.Client {
	value := http.Client{}
	if client != nil {
		value = *client
	}
	value.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &value
}

// Client binds region, providers, endpoints and HTTP configuration. Construct with NewClient;
// do not replace its public service pointer concurrently.
type Client struct {
	cfg             clientConfig
	sandboxes       *SandboxManager
	tools           *ToolManager
	preCacheImages  *PreCacheImageManager
	deployments     *DeploymentManager
	volumes         *VolumeManager
	volumeTemplates *VolumeTemplateManager
	sessionSpaces   *SessionSpaceManager
	sessions        *SessionManager
	registries      *RegistryManager
}

// Sandboxes returns the sandbox control service bound to this client's Cloud identity.
func (c *Client) Sandboxes() *SandboxManager { return c.sandboxes }

// Tools returns the sandbox-tool control service bound to this client's Cloud identity.
func (c *Client) Tools() *ToolManager { return c.tools }

// PreCacheImages returns the image pre-cache service bound to this client's Cloud identity.
func (c *Client) PreCacheImages() *PreCacheImageManager { return c.preCacheImages }

// Deployments returns the Deployment control and data-plane service bound to this client.
func (c *Client) Deployments() *DeploymentManager { return c.deployments }

// Volumes returns the Preview standalone Volume service.
func (c *Client) Volumes() *VolumeManager { return c.volumes }

// VolumeTemplates returns the Preview derived-Volume template service.
func (c *Client) VolumeTemplates() *VolumeTemplateManager { return c.volumeTemplates }

// SessionSpaces returns the Preview SessionSpace service.
func (c *Client) SessionSpaces() *SessionSpaceManager { return c.sessionSpaces }

// Sessions returns the Preview Session and Event service.
func (c *Client) Sessions() *SessionManager { return c.sessions }

// Registries returns the Preview Registry service.
func (c *Client) Registries() *RegistryManager { return c.registries }

// NewClient constructs a client without creating a sandbox. Region defaults from
// TENCENTCLOUD_REGION; request timeout defaults to 30 seconds. Explicit control credentials
// are read for each signed request so temporary credentials can rotate within one logical
// Cloud identity.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := clientConfig{region: os.Getenv("TENCENTCLOUD_REGION"), credential: EnvironmentCredentials{}, timeout: 30 * time.Second, controlEndpoint: "ags.tencentcloudapi.com", monitorEndpoint: "monitor.tencentcloudapi.com", dataPlaneDomain: "tencentags.com"}
	for _, opt := range opts {
		if opt == nil {
			return nil, codeError(InvalidArgument, "NewClient", "OPTION_REQUIRED")
		}
		opt(&c)
	}
	if c.region == "" {
		return nil, codeError(InvalidArgument, "NewClient", "REGION_REQUIRED")
	}
	if c.timeout <= 0 {
		return nil, codeError(InvalidArgument, "NewClient", "TIMEOUT_OUT_OF_RANGE")
	}
	if !validDataPlaneDomain(c.dataPlaneDomain) {
		return nil, codeError(InvalidArgument, "NewClient", "DATA_PLANE_DOMAIN_INVALID")
	}
	if !c.sandboxDomainSet {
		c.sandboxDomain = c.region + ".tencentags.com"
	}
	if !validDataPlaneDomain(c.sandboxDomain) {
		return nil, codeError(InvalidArgument, "NewClient", "SANDBOX_DOMAIN_INVALID")
	}
	c.httpClient = noRedirectClient(c.httpClient)
	if c.credential == nil {
		return nil, codeError(InvalidArgument, "NewClient", "CREDENTIAL_PROVIDER_REQUIRED")
	}
	var adapter *controlAdapter
	if c.control == nil || c.monitor == nil {
		adapter = newControlAdapter(c.region, c.credential, c.controlEndpoint, c.monitorEndpoint, c.httpClient, c.timeout, c.sandboxDomain)
	}
	if c.control == nil {
		c.control = adapter
	}
	if c.monitor == nil {
		c.monitor = adapter
	}
	out := &Client{cfg: c}
	out.sandboxes = &SandboxManager{client: out}
	out.tools = &ToolManager{client: out}
	out.preCacheImages = &PreCacheImageManager{client: out}
	out.deployments = &DeploymentManager{client: out}
	out.volumes = &VolumeManager{client: out}
	out.volumeTemplates = &VolumeTemplateManager{client: out}
	out.sessionSpaces = &SessionSpaceManager{client: out}
	out.sessions = &SessionManager{client: out}
	out.registries = &RegistryManager{client: out}
	return out, nil
}

var dataPlaneDomainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validDataPlaneDomain(value string) bool {
	return len(value) > 0 && len(value) <= 253 && dataPlaneDomainPattern.MatchString(value)
}

// SandboxManager provides control-plane operations and returns the same Sandbox type as the
// shortcut package.
type SandboxManager struct{ client *Client }

// Delete explicitly stops an instance by ID without acquiring a data-plane
// connection. This also permits cleanup after a failed Create readiness check.
func (m *SandboxManager) Delete(ctx context.Context, id string) error {
	if id == "" {
		return codeError(InvalidArgument, "Sandboxes.Delete", "INSTANCE_ID_REQUIRED")
	}
	info, err := m.Get(ctx, id)
	var failure *Error
	if errors.As(err, &failure) && failure.Code == NotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if info.State == Stopped {
		return nil
	}
	if info.State == Paused {
		if _, err = m.client.cfg.control.Resume(ctx, id, ResumeOptions{}); err != nil {
			return err
		}
	}
	err = m.client.cfg.control.Delete(ctx, id)
	if errors.As(err, &failure) && failure.Code == NotFound {
		return nil
	}
	return err
}

// Create submits one sandbox creation and waits for RUNNING and data-plane readiness within
// ctx. It never retries or automatically deletes an accepted resource. Post-acceptance errors
// preserve a known InstanceID. Returned handles require local Close and explicitly owned
// remote Delete.
func (m *SandboxManager) Create(ctx context.Context, opts CreateOptions) (*Sandbox, error) {
	var err error
	opts, err = copyCreateEnv(opts)
	if err != nil {
		return nil, err
	}
	if err := validateCreate(opts); err != nil {
		return nil, err
	}
	info, err := m.client.cfg.control.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	sandbox, err := m.connected(ctx, info)
	if err != nil {
		failure := &Error{Code: Unavailable, Operation: "Sandboxes.Create", Reason: "CONNECTION_FAILED", InstanceID: info.ID, Cause: err}
		var sdk *Error
		if errors.As(err, &sdk) {
			failure.Code, failure.Reason, failure.RequestID, failure.Retryable = sdk.Code, sdk.Reason, sdk.RequestID, sdk.Retryable
		}
		return nil, failure
	}
	return sandbox, nil
}

// Connect resumes a paused sandbox and extends a running sandbox only when its
// remaining lifetime is shorter than Timeout (five minutes by default).
func (m *SandboxManager) Connect(ctx context.Context, id string, options ...ConnectOptions) (*Sandbox, error) {
	if id == "" {
		return nil, codeError(InvalidArgument, "Sandboxes.Connect", "INSTANCE_ID_REQUIRED")
	}
	timeout, err := connectTimeout(options)
	if err != nil {
		return nil, err
	}
	info, err := m.client.cfg.control.Connect(ctx, id, timeout)
	if err != nil {
		return nil, err
	}
	return m.connected(ctx, info)
}

// Get reads the exact nonempty sandbox ID without extending lifetime, resuming or connecting
// envd. Missing resources return NotFound.
func (m *SandboxManager) Get(ctx context.Context, id string) (SandboxInfo, error) {
	if id == "" {
		return SandboxInfo{}, codeError(InvalidArgument, "Sandboxes.Get", "INSTANCE_ID_REQUIRED")
	}
	return m.client.cfg.control.Get(ctx, id)
}

// List returns one server-filtered Cloud page. Limit defaults to 20. Unsupported filters fail
// explicitly rather than being dropped.
func (m *SandboxManager) List(ctx context.Context, opts SandboxListOptions) (SandboxPage, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.Limit < 1 || opts.Limit > 100 {
		return SandboxPage{}, codeError(InvalidArgument, "Sandboxes.List", "LIMIT_OUT_OF_RANGE")
	}
	if opts.Offset < 0 {
		return SandboxPage{}, codeError(InvalidArgument, "Sandboxes.List", "PAGINATION_OUT_OF_RANGE")
	}
	opts = copyListOptions(opts)
	return m.client.cfg.control.List(ctx, opts)
}

// ListPage returns one cursor-based Cloud page. A nonempty NextToken must be passed unchanged
// with the same query parameters; the SDK does not parse, restart, or aggregate cursors.
func (m *SandboxManager) ListPage(ctx context.Context, opts SandboxPageOptions) (SandboxCursorPage, error) {
	if opts.MaxResults == 0 {
		opts.MaxResults = 20
	}
	if opts.MaxResults < 1 || opts.MaxResults > 100 {
		return SandboxCursorPage{}, codeError(InvalidArgument, "Sandboxes.ListPage", "MAX_RESULTS_OUT_OF_RANGE")
	}
	if len(opts.Filter.InstanceIDs) > 100 {
		return SandboxCursorPage{}, codeError(InvalidArgument, "Sandboxes.ListPage", "TOO_MANY_INSTANCE_IDS")
	}
	for _, id := range opts.Filter.InstanceIDs {
		if id == "" {
			return SandboxCursorPage{}, codeError(InvalidArgument, "Sandboxes.ListPage", "INSTANCE_ID_REQUIRED")
		}
	}
	opts = copyPageOptions(opts)
	cursor, ok := m.client.cfg.control.(cursorControlPlane)
	if !ok {
		return SandboxCursorPage{}, codeError(Unsupported, "Sandboxes.ListPage", "CURSOR_PAGINATION_UNAVAILABLE")
	}
	return cursor.ListPage(ctx, opts)
}

// SandboxIterator fetches one cursor page at a time and retains at most one
// page. It is not a snapshot: changes to the remote list may affect later pages.
// Call Close when stopping before io.EOF.
type SandboxIterator struct {
	manager *SandboxManager
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	opts    SandboxPageOptions
	items   []SandboxInfo
	started bool
	done    bool
	closed  bool
}

// Iterate returns a lazy cursor iterator with a copy of the query options.
// Each Next call may issue a Cloud request; errors leave the cursor unchanged.
func (m *SandboxManager) Iterate(opts SandboxPageOptions) *SandboxIterator {
	ctx, cancel := context.WithCancel(context.Background())
	return &SandboxIterator{manager: m, ctx: ctx, cancel: cancel, opts: copyPageOptions(opts)}
}

// Next returns one Sandbox or io.EOF after the final page or Close. A canceled
// call returns a typed cancellation error and does not reset the cursor.
func (it *SandboxIterator) Next(ctx context.Context) (SandboxInfo, error) {
	if ctx == nil {
		return SandboxInfo{}, codeError(InvalidArgument, "Sandboxes.Iterate", "CONTEXT_REQUIRED")
	}
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.closed {
		return SandboxInfo{}, io.EOF
	}
	for emptyPages := 0; ; {
		if err := ctx.Err(); err != nil {
			return SandboxInfo{}, normalizeError("Sandboxes.Iterate", err)
		}
		if len(it.items) > 0 {
			item := it.items[0]
			it.items = it.items[1:]
			return item, nil
		}
		if it.done || it.started && it.opts.NextToken == "" {
			it.done = true
			return SandboxInfo{}, io.EOF
		}
		previousToken := it.opts.NextToken
		requestCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(it.ctx, cancel)
		page, err := it.manager.ListPage(requestCtx, it.opts)
		stop()
		cancel()
		if it.closed || it.ctx.Err() != nil {
			return SandboxInfo{}, codeError(Canceled, "Sandboxes.Iterate", "ITERATOR_CLOSED")
		}
		if err != nil {
			return SandboxInfo{}, err
		}
		it.started = true
		it.items = page.Items
		it.opts.NextToken = page.NextToken
		if len(it.items) == 0 && it.opts.NextToken != "" {
			if it.opts.NextToken == previousToken || emptyPages >= 15 {
				it.done = true
				return SandboxInfo{}, codeError(Protocol, "Sandboxes.Iterate", "EMPTY_CURSOR_LOOP")
			}
			emptyPages++
		}
	}
}

// Close cancels an in-flight Next request and releases the buffered page.
// It does not change any remote Sandbox.
func (it *SandboxIterator) Close() error {
	it.cancel()
	it.mu.Lock()
	it.closed = true
	it.items = nil
	it.mu.Unlock()
	return nil
}

func (m *SandboxManager) connected(ctx context.Context, info SandboxInfo) (*Sandbox, error) {
	for info.State != Running {
		if info.State == Failed || info.State == Stopped {
			return nil, codeError(Conflict, "Sandboxes.Connect", "TERMINAL_STATE")
		}
		select {
		case <-ctx.Done():
			return nil, normalizeError("Sandboxes.Connect", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
		var err error
		info, err = m.Get(ctx, info.ID)
		if err != nil {
			return nil, err
		}
	}
	dp, err := m.client.cfg.control.dataPlane(ctx, info.ID)
	if err != nil {
		return nil, err
	}
	if err := dp.Ready(ctx); err != nil {
		_ = dp.Close()
		return nil, err
	}
	return newSandbox(m.client, info.ID, dp), nil
}
func validateCreate(opts CreateOptions) error {
	if (opts.Tool.ID == "") == (opts.Tool.Name == "") {
		return codeError(InvalidArgument, "Sandboxes.Create", "EXACTLY_ONE_TOOL_REFERENCE_REQUIRED")
	}
	if opts.Timeout != nil && (*opts.Timeout < 30*time.Second || *opts.Timeout > 24*time.Hour || *opts.Timeout%time.Second != 0) {
		return codeError(InvalidArgument, "Sandboxes.Create", "TIMEOUT_OUT_OF_RANGE")
	}
	if len(opts.ClientToken) > 64 {
		return codeError(InvalidArgument, "Sandboxes.Create", "CLIENT_TOKEN_TOO_LONG")
	}
	for key, value := range opts.Metadata {
		if key == "" || strings.ContainsAny(key, "\x00\r\n") || strings.ContainsRune(value, 0) {
			return codeError(InvalidArgument, "Sandboxes.Create", "METADATA_INVALID")
		}
	}
	for _, mount := range opts.MountOptions {
		if mount.Name == "" || strings.ContainsRune(mount.Name, 0) || strings.ContainsRune(mount.MountPath, 0) || strings.ContainsRune(mount.SubPath, 0) {
			return codeError(InvalidArgument, "Sandboxes.Create", "MOUNT_OPTION_INVALID")
		}
	}
	switch opts.AuthMode {
	case "", SandboxAuthDefault, SandboxAuthToken, SandboxAuthNone, SandboxAuthPublic:
	default:
		return codeError(InvalidArgument, "Sandboxes.Create", "AUTH_MODE_INVALID")
	}
	return nil
}

// ConnectOptions controls sandbox lifetime, not the HTTP or context deadline.
type ConnectOptions struct {
	// Timeout defaults to five minutes. Connect never intentionally shortens a
	// running sandbox's remaining lifetime. Running updates require at least 300 seconds;
	// paused resume accepts at least 30 seconds. Both allow up to 24 hours.
	Timeout *time.Duration
}

func connectTimeout(options []ConnectOptions) (time.Duration, error) {
	if len(options) > 1 {
		return 0, codeError(InvalidArgument, "Sandboxes.Connect", "SINGLE_OPTIONS_REQUIRED")
	}
	timeout := 5 * time.Minute
	if len(options) == 1 && options[0].Timeout != nil {
		timeout = *options[0].Timeout
	}
	if timeout < 30*time.Second || timeout > 24*time.Hour || timeout%time.Second != 0 {
		return 0, codeError(InvalidArgument, "Sandboxes.Connect", "TIMEOUT_OUT_OF_RANGE")
	}
	return timeout, nil
}

func copyResumeOptions(options ResumeOptions) (ResumeOptions, error) {
	if options.Timeout == nil {
		return options, nil
	}
	timeout := *options.Timeout
	if timeout < 30*time.Second || timeout > 24*time.Hour || timeout%time.Second != 0 {
		return ResumeOptions{}, codeError(InvalidArgument, "Sandbox.Resume", "TIMEOUT_OUT_OF_RANGE")
	}
	options.Timeout = &timeout
	return options, nil
}

var createEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func copyCreateEnv(opts CreateOptions) (CreateOptions, error) {
	values := make(map[string]string, len(opts.Env))
	for key, value := range opts.Env {
		if !createEnvKey.MatchString(key) || strings.ContainsRune(value, 0) {
			return opts, codeError(InvalidArgument, "Sandboxes.Create", "ENV_INVALID")
		}
		values[key] = value
	}
	if len(values) > 0 {
		opts.Env = values
	}
	opts.Metadata = cloneStringMap(opts.Metadata)
	opts.MountOptions = append([]MountOption(nil), opts.MountOptions...)
	for i := range opts.MountOptions {
		if opts.MountOptions[i].ReadOnly != nil {
			value := *opts.MountOptions[i].ReadOnly
			opts.MountOptions[i].ReadOnly = &value
		}
	}
	return opts, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

var defaultClient struct {
	sync.Mutex
	client *Client
}

// DefaultClient lazily caches the first successfully configured environment-backed client.
// Mode, endpoints and provider bindings then remain fixed; do not change process environment
// to switch tenants. Use NewClient for explicit multi-account configuration.
func DefaultClient() (*Client, error) {
	defaultClient.Lock()
	defer defaultClient.Unlock()
	if defaultClient.client == nil {
		client, err := NewClient()
		if err != nil {
			return nil, err
		}
		defaultClient.client = client
	}
	return defaultClient.client, nil
}

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

// DeploymentConnectOptions selects one Tool port and optional existing affinity session.
type DeploymentConnectOptions struct {
	// Port is the remote Tool port, in the range 1..65535.
	Port int
	// InitialAffinityID resumes one known affinity session. It is rejected when affinity is off.
	InitialAffinityID string
}

// String returns a redacted representation.
func (DeploymentConnectOptions) String() string { return "DeploymentConnectOptions{REDACTED}" }

// GoString returns a redacted Go-syntax representation.
func (o DeploymentConnectOptions) GoString() string { return o.String() }

// DeploymentRequest is one relative request sent to the fixed Deployment authority.
type DeploymentRequest struct {
	// Method is the HTTP method.
	Method string
	// Path is an absolute-path reference and may include a query string.
	Path string
	// Header contains business headers and is copied before SDK headers are applied.
	Header http.Header
	// Body is the optional streaming request body.
	Body io.Reader
}

// String returns a redacted representation.
func (DeploymentRequest) String() string { return "DeploymentRequest{REDACTED}" }

// GoString returns a redacted Go-syntax representation.
func (r DeploymentRequest) GoString() string { return r.String() }

// DeploymentWebSocketOptions configures one WebSocket handshake on the fixed authority.
type DeploymentWebSocketOptions struct {
	// Path is an absolute-path reference and may include a query string.
	Path string
	// Header contains business handshake headers and is copied before SDK headers are applied.
	Header http.Header
}

// String returns a redacted representation.
func (DeploymentWebSocketOptions) String() string { return "DeploymentWebSocketOptions{REDACTED}" }

// GoString returns a redacted Go-syntax representation.
func (o DeploymentWebSocketOptions) GoString() string { return o.String() }

// DeploymentMessageType identifies one supported WebSocket message kind.
type DeploymentMessageType int

const (
	// DeploymentTextMessage is a UTF-8 WebSocket text message.
	DeploymentTextMessage DeploymentMessageType = 1
	// DeploymentBinaryMessage is an opaque WebSocket binary message.
	DeploymentBinaryMessage DeploymentMessageType = 2
)

// DeploymentMessage is one SDK-owned WebSocket message value.
type DeploymentMessage struct {
	// Type identifies text or binary framing.
	Type DeploymentMessageType
	// Data is copied at the SDK boundary.
	Data []byte
}

// DeploymentConnection owns one remote port connection policy and its local token/affinity
// state. Close cancels active operations but never deletes the remote Deployment.
type DeploymentConnection struct{ wire *deploymentwire.Connection }

// DeploymentWebSocket is the SDK-owned, context-aware WebSocket handle.
type DeploymentWebSocket struct{ wire *deploymentwire.WebSocket }

// Connect verifies an ACTIVE Deployment and returns a lazy managed connection for one Tool port.
func (m *DeploymentManager) Connect(ctx context.Context, id string, opts DeploymentConnectOptions) (*DeploymentConnection, error) {
	if id == "" {
		return nil, codeError(InvalidArgument, "Deployments.Connect", "DEPLOYMENT_ID_REQUIRED")
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, codeError(InvalidArgument, "Deployments.Connect", "DEPLOYMENT_PORT_OUT_OF_RANGE")
	}
	info, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if info.State != DeploymentActive {
		return nil, withResourceError(codeError(Conflict, "Deployments.Connect", "DEPLOYMENT_NOT_ACTIVE"), "deployment", id)
	}
	affinityHeader := ""
	if info.Affinity != nil && info.Affinity.Mode != "" {
		affinityHeader = info.Affinity.HeaderName
		if affinityHeader == "" {
			affinityHeader = "X-Tencent-Agr-Affinity-Id"
		}
	} else if opts.InitialAffinityID != "" {
		return nil, codeError(InvalidArgument, "Deployments.Connect", "DEPLOYMENT_AFFINITY_DISABLED")
	}
	cp, err := m.transport("Deployments.Connect")
	if err != nil {
		return nil, err
	}
	wire, err := deploymentwire.New(deploymentwire.Config{
		DeploymentID:   id,
		Region:         m.client.cfg.region,
		Domain:         m.client.cfg.dataPlaneDomain,
		Port:           opts.Port,
		AffinityHeader: affinityHeader,
		InitialID:      opts.InitialAffinityID,
		HTTPClient:     m.client.cfg.httpClient,
		TokenSource: func(tokenCtx context.Context, deploymentID string) (deploymentwire.Token, error) {
			value, acquireErr := cp.acquireDeploymentToken(tokenCtx, deploymentID)
			if acquireErr != nil {
				return deploymentwire.Token{}, acquireErr
			}
			return deploymentwire.Token{Value: value.token, ExpiresAt: value.expiresAt}, nil
		},
	})
	if err != nil {
		return nil, normalizeError("Deployments.Connect", err)
	}
	return &DeploymentConnection{wire: wire}, nil
}

// Do sends one request to the fixed Deployment authority. It never retries. For SSE, consume
// the returned Body incrementally and close it when finished.
func (c *DeploymentConnection) Do(ctx context.Context, request DeploymentRequest) (*http.Response, error) {
	if c == nil || c.wire == nil {
		return nil, codeError(Conflict, "Deployments.Do", "DEPLOYMENT_CONNECTION_CLOSED")
	}
	if request.Method == "" || !deploymentwire.ValidatePath(request.Path) {
		return nil, codeError(InvalidArgument, "Deployments.Do", "DEPLOYMENT_REQUEST_INVALID")
	}
	response, err := c.wire.Do(ctx, deploymentwire.Request{Method: request.Method, Path: request.Path, Header: request.Header, Body: request.Body})
	if err != nil {
		return nil, normalizeError("Deployments.Do", err)
	}
	return response, nil
}

// DialWebSocket opens one managed WebSocket without exposing the underlying third-party type.
func (c *DeploymentConnection) DialWebSocket(ctx context.Context, opts DeploymentWebSocketOptions) (*DeploymentWebSocket, error) {
	if c == nil || c.wire == nil {
		return nil, codeError(Conflict, "Deployments.DialWebSocket", "DEPLOYMENT_CONNECTION_CLOSED")
	}
	if !deploymentwire.ValidatePath(opts.Path) {
		return nil, codeError(InvalidArgument, "Deployments.DialWebSocket", "DEPLOYMENT_REQUEST_INVALID")
	}
	wire, response, err := c.wire.DialWebSocket(ctx, opts.Path, opts.Header)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			switch response.StatusCode {
			case http.StatusUnauthorized:
				return nil, codeError(Unauthenticated, "Deployments.DialWebSocket", "DEPLOYMENT_TOKEN_REJECTED")
			case http.StatusForbidden:
				return nil, codeError(PermissionDenied, "Deployments.DialWebSocket", "DEPLOYMENT_ACCESS_DENIED")
			case http.StatusNotFound:
				return nil, codeError(NotFound, "Deployments.DialWebSocket", "DEPLOYMENT_ENDPOINT_NOT_FOUND")
			}
		}
		return nil, normalizeError("Deployments.DialWebSocket", err)
	}
	return &DeploymentWebSocket{wire: wire}, nil
}

// AffinityID returns the currently discovered affinity ID, or an empty string when unavailable.
func (c *DeploymentConnection) AffinityID() string {
	if c == nil || c.wire == nil {
		return ""
	}
	return c.wire.AffinityID()
}

// Close cancels active HTTP, SSE, and WebSocket work without deleting the Deployment.
func (c *DeploymentConnection) Close() error {
	if c == nil || c.wire == nil {
		return nil
	}
	err := c.wire.Close()
	return normalizeError("Deployments.Close", err)
}

// Read waits for one WebSocket message or context cancellation.
func (w *DeploymentWebSocket) Read(ctx context.Context) (DeploymentMessage, error) {
	if w == nil || w.wire == nil {
		return DeploymentMessage{}, codeError(Conflict, "Deployments.WebSocket.Read", "WEBSOCKET_CLOSED")
	}
	message, err := w.wire.Read(ctx)
	if err != nil {
		return DeploymentMessage{}, normalizeError("Deployments.WebSocket.Read", err)
	}
	return DeploymentMessage{Type: DeploymentMessageType(message.Type), Data: message.Data}, nil
}

// Write sends one text or binary WebSocket message or stops on context cancellation.
func (w *DeploymentWebSocket) Write(ctx context.Context, message DeploymentMessage) error {
	if w == nil || w.wire == nil {
		return codeError(Conflict, "Deployments.WebSocket.Write", "WEBSOCKET_CLOSED")
	}
	if message.Type != DeploymentTextMessage && message.Type != DeploymentBinaryMessage {
		return codeError(InvalidArgument, "Deployments.WebSocket.Write", "WEBSOCKET_MESSAGE_TYPE_INVALID")
	}
	return normalizeError("Deployments.WebSocket.Write", w.wire.Write(ctx, deploymentwire.Message{Type: int(message.Type), Data: message.Data}))
}

// Close closes the local WebSocket handle.
func (w *DeploymentWebSocket) Close() error {
	if w == nil || w.wire == nil {
		return nil
	}
	err := w.wire.Close()
	return normalizeError("Deployments.WebSocket.Close", err)
}
