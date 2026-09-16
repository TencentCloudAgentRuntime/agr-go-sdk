package sandbox

import (
	"context"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

// Public aliases keep the shortcut API compact while preserving one canonical resource and
// model implementation in the root agr package.
type (
	Sandbox            = agr.Sandbox
	CreateOptions      = agr.CreateOptions
	ConnectOptions     = agr.ConnectOptions
	SandboxListOptions = agr.SandboxListOptions
	SandboxInfo        = agr.SandboxInfo
	SandboxPage        = agr.SandboxPage
	SandboxState       = agr.SandboxState
	ToolRef            = agr.ToolRef
	MountOption        = agr.MountOption
	SandboxAuthMode    = agr.SandboxAuthMode
)

// Lifecycle and authentication aliases preserve the root package's canonical values.
const (
	Creating = agr.Creating
	Running  = agr.Running
	Pausing  = agr.Pausing
	Paused   = agr.Paused
	Resuming = agr.Resuming
	Stopping = agr.Stopping
	Stopped  = agr.Stopped
	Failed   = agr.Failed
	Unknown  = agr.Unknown

	AuthDefault = agr.SandboxAuthDefault
	AuthToken   = agr.SandboxAuthToken
	AuthNone    = agr.SandboxAuthNone
	AuthPublic  = agr.SandboxAuthPublic
)

var resolveDefaultClient = agr.DefaultClient

// Create creates a Sandbox with the cached environment-backed default Client.
func Create(ctx context.Context, opts CreateOptions) (*Sandbox, error) {
	client, err := resolveDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Sandboxes().Create(ctx, opts)
}

// Connect resumes or conditionally extends an existing Sandbox with the default Client.
func Connect(ctx context.Context, id string, opts ...ConnectOptions) (*Sandbox, error) {
	client, err := resolveDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Sandboxes().Connect(ctx, id, opts...)
}

// Get reads Sandbox information without resuming, extending, or opening the data plane.
func Get(ctx context.Context, id string) (SandboxInfo, error) {
	client, err := resolveDefaultClient()
	if err != nil {
		return SandboxInfo{}, err
	}
	return client.Sandboxes().Get(ctx, id)
}

// List returns one server-filtered Cloud page through the default Client.
func List(ctx context.Context, opts SandboxListOptions) (SandboxPage, error) {
	client, err := resolveDefaultClient()
	if err != nil {
		return SandboxPage{}, err
	}
	return client.Sandboxes().List(ctx, opts)
}
