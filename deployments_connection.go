package agr

import (
	"context"
	"io"
	"net/http"

	deploymentwire "github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/dataplane/deployment"
)

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
