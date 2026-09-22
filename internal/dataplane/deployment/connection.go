// Package deployment owns the private Deployment data-plane transport.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tokenRefreshBefore       = 2 * time.Hour
	maxWebSocketMessageBytes = 1024 * 1024
)

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type TokenSource func(context.Context, string) (Token, error)

type Config struct {
	DeploymentID, Region, Domain string
	Port                         int
	AffinityHeader, InitialID    string
	TokenSource                  TokenSource
	HTTPClient                   *http.Client
	Now                          func() time.Time
}

type Request struct {
	Method, Path string
	Header       http.Header
	Body         io.Reader
}

type Message struct {
	Type int
	Data []byte
}

type Connection struct {
	id, target string
	http       *http.Client
	tokens     *tokenManager
	affinity   *affinityState
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	closed     bool
	sockets    map[*WebSocket]struct{}
}

func New(config Config) (*Connection, error) {
	if config.DeploymentID == "" || config.Region == "" || config.Domain == "" || config.TokenSource == nil {
		return nil, errors.New("deployment connection configuration is incomplete")
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("deployment port is out of range")
	}
	client := &http.Client{}
	if config.HTTPClient != nil {
		*client = *config.HTTPClient
	}
	client.Timeout = 0
	ctx, cancel := context.WithCancel(context.Background())
	value := &Connection{
		id:      config.DeploymentID,
		target:  fmt.Sprintf("%d-%s.%s.agents.%s", config.Port, config.DeploymentID, config.Region, config.Domain),
		http:    client,
		ctx:     ctx,
		cancel:  cancel,
		sockets: map[*WebSocket]struct{}{},
	}
	value.tokens = newTokenManager(ctx, config.DeploymentID, config.TokenSource, config.Now)
	if config.AffinityHeader != "" {
		value.affinity = newAffinityState(config.AffinityHeader, config.InitialID)
	}
	return value, nil
}

func (c *Connection) Do(ctx context.Context, in Request) (*http.Response, error) {
	requestCtx, cancel, err := c.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	keepContext := false
	defer func() {
		if !keepContext {
			cancel()
		}
	}()
	affinityRequest, err := c.acquireAffinity(requestCtx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !keepContext {
			affinityRequest.complete("")
		}
	}()
	token, err := c.tokens.Token(requestCtx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(requestCtx, in.Method, "https://"+c.target+in.Path, in.Body)
	if err != nil {
		return nil, err
	}
	req.Header = in.Header.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Host = c.target
	req.Header.Set("X-Access-Token", token)
	c.applyAffinity(req.Header, affinityRequest.sentID)
	if req.Header.Get("Origin") != "" {
		req.Header.Set("Origin", "https://"+c.target)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	responseID := ""
	if c.affinity != nil {
		responseID = response.Header.Get(c.affinity.headerName)
	}
	affinityRequest.complete(responseID)
	if response.StatusCode == http.StatusUnauthorized {
		c.tokens.Invalidate()
	}
	response.Body = &responseBody{ReadCloser: response.Body, cancel: cancel}
	keepContext = true
	return response, nil
}

func (c *Connection) DialWebSocket(ctx context.Context, path string, headers http.Header) (*WebSocket, *http.Response, error) {
	requestCtx, cancel, err := c.operationContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer cancel()
	affinityRequest, err := c.acquireAffinity(requestCtx)
	if err != nil {
		return nil, nil, err
	}
	defer affinityRequest.complete("")
	token, err := c.tokens.Token(requestCtx)
	if err != nil {
		return nil, nil, err
	}
	requestHeaders := headers.Clone()
	if requestHeaders == nil {
		requestHeaders = make(http.Header)
	}
	for _, name := range []string{"Connection", "Upgrade", "Proxy-Connection", "Keep-Alive", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions"} {
		requestHeaders.Del(name)
	}
	requestHeaders.Set("X-Access-Token", token)
	c.applyAffinity(requestHeaders, affinityRequest.sentID)
	if requestHeaders.Get("Origin") != "" {
		requestHeaders.Set("Origin", "https://"+c.target)
	}
	dialer := c.websocketDialer()
	conn, response, err := dialer.DialContext(requestCtx, "wss://"+c.target+path, requestHeaders)
	if response != nil && response.StatusCode == http.StatusUnauthorized {
		c.tokens.Invalidate()
	}
	if err != nil {
		return nil, response, err
	}
	conn.SetReadLimit(maxWebSocketMessageBytes)
	responseID := ""
	if c.affinity != nil && response != nil {
		responseID = response.Header.Get(c.affinity.headerName)
	}
	affinityRequest.complete(responseID)
	value := &WebSocket{conn: conn, owner: c}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return nil, response, context.Canceled
	}
	c.sockets[value] = struct{}{}
	c.mu.Unlock()
	return value, response, nil
}

func (c *Connection) AffinityID() string {
	if c.affinity == nil {
		return ""
	}
	return c.affinity.currentID()
}

func (c *Connection) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.cancel()
	sockets := make([]*WebSocket, 0, len(c.sockets))
	for socket := range c.sockets {
		sockets = append(sockets, socket)
	}
	c.mu.Unlock()
	for _, socket := range sockets {
		_ = socket.Close()
	}
	return nil
}

func (c *Connection) operationContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, errors.New("context is required")
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, nil, context.Canceled
	}
	operation, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.ctx, cancel)
	return operation, func() { stop(); cancel() }, nil
}

func (c *Connection) acquireAffinity(ctx context.Context) (*affinityRequest, error) {
	if c.affinity == nil {
		return &affinityRequest{}, nil
	}
	return c.affinity.acquire(ctx)
}

func (c *Connection) applyAffinity(header http.Header, id string) {
	if c.affinity == nil {
		return
	}
	header.Del(c.affinity.headerName)
	if id != "" {
		header.Set(c.affinity.headerName, id)
	}
}

func (c *Connection) websocketDialer() *websocket.Dialer {
	dialer := &websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 15 * time.Second, ReadBufferSize: 65536, WriteBufferSize: 65536}
	if transport, ok := c.http.Transport.(*http.Transport); ok {
		dialer.Proxy = transport.Proxy
		dialer.NetDialContext = transport.DialContext
		if transport.TLSClientConfig != nil {
			dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
		}
	}
	return dialer
}

type responseBody struct {
	io.ReadCloser
	once   sync.Once
	cancel context.CancelFunc
}

func (b *responseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}

type WebSocket struct {
	conn      *websocket.Conn
	owner     *Connection
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (w *WebSocket) Read(ctx context.Context) (Message, error) {
	if ctx == nil {
		return Message{}, errors.New("context is required")
	}
	w.readMu.Lock()
	defer w.readMu.Unlock()
	stop := deadlineOnCancel(ctx, w.conn.SetReadDeadline)
	defer stop()
	typeID, data, err := w.conn.ReadMessage()
	return Message{Type: typeID, Data: append([]byte(nil), data...)}, err
}

func (w *WebSocket) Write(ctx context.Context, message Message) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	stop := deadlineOnCancel(ctx, w.conn.SetWriteDeadline)
	defer stop()
	return w.conn.WriteMessage(message.Type, append([]byte(nil), message.Data...))
}

func (w *WebSocket) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = w.conn.Close()
		w.owner.mu.Lock()
		delete(w.owner.sockets, w)
		w.owner.mu.Unlock()
	})
	return w.closeErr
}

func deadlineOnCancel(ctx context.Context, set func(time.Time) error) func() {
	if deadline, ok := ctx.Deadline(); ok {
		_ = set(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = set(time.Now()) })
	return func() {
		stop()
		_ = set(time.Time{})
	}
}

type tokenManager struct {
	mu         sync.Mutex
	token      string
	expires    time.Time
	refreshing bool
	wait       chan struct{}
	refreshErr error
	lifecycle  context.Context
	id         string
	source     TokenSource
	now        func() time.Time
}

func newTokenManager(lifecycle context.Context, id string, source TokenSource, now func() time.Time) *tokenManager {
	if now == nil {
		now = time.Now
	}
	return &tokenManager{lifecycle: lifecycle, id: id, source: source, now: now}
}

func (m *tokenManager) Token(ctx context.Context) (string, error) {
	for {
		m.mu.Lock()
		if m.token != "" && m.expires.Sub(m.now()) > tokenRefreshBefore {
			token := m.token
			m.mu.Unlock()
			return token, nil
		}
		if !m.refreshing {
			m.refreshing = true
			m.wait = make(chan struct{})
			go m.refresh(m.wait)
		}
		wait := m.wait
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-m.lifecycle.Done():
			return "", m.lifecycle.Err()
		case <-wait:
			m.mu.Lock()
			token, err := m.token, m.refreshErr
			valid := token != "" && m.expires.After(m.now())
			m.mu.Unlock()
			if valid {
				return token, nil
			}
			if err == nil {
				err = errors.New("deployment token response is invalid")
			}
			return "", err
		}
	}
}

func (m *tokenManager) refresh(wait chan struct{}) {
	value, err := m.source(m.lifecycle, m.id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil && value.Value != "" && value.ExpiresAt.After(m.now()) {
		m.token, m.expires = value.Value, value.ExpiresAt
	} else if err == nil {
		err = errors.New("deployment token response is invalid")
	}
	// Keep a still-valid token when proactive refresh fails. Clearing it would turn a
	// transient control-plane failure into an avoidable data-plane outage.
	if err != nil && (m.token == "" || !m.expires.After(m.now())) {
		m.token, m.expires = "", time.Time{}
	}
	m.refreshErr = err
	m.refreshing = false
	close(wait)
}

func (m *tokenManager) Invalidate() {
	m.mu.Lock()
	m.token, m.expires = "", time.Time{}
	m.mu.Unlock()
}

type affinityState struct {
	mu            sync.RWMutex
	headerName    string
	id            string
	discoveryGate chan struct{}
}

func newAffinityState(headerName, initialID string) *affinityState {
	return &affinityState{headerName: headerName, id: initialID, discoveryGate: make(chan struct{}, 1)}
}

func (s *affinityState) currentID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

func (s *affinityState) acquire(ctx context.Context) (*affinityRequest, error) {
	if id := s.currentID(); id != "" {
		return &affinityRequest{state: s, sentID: id}, nil
	}
	select {
	case s.discoveryGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if id := s.currentID(); id != "" {
		<-s.discoveryGate
		return &affinityRequest{state: s, sentID: id}, nil
	}
	return &affinityRequest{state: s, discovery: true}, nil
}

type affinityRequest struct {
	state     *affinityState
	sentID    string
	discovery bool
	once      sync.Once
}

func (r *affinityRequest) complete(responseID string) {
	if r == nil || r.state == nil {
		return
	}
	r.once.Do(func() {
		if r.discovery {
			r.state.mu.Lock()
			if responseID != "" && r.state.id == "" {
				r.state.id = responseID
			}
			r.state.mu.Unlock()
			<-r.state.discoveryGate
			return
		}
		if responseID == "" {
			return
		}
		r.state.mu.Lock()
		if r.state.id == r.sentID && r.state.id != responseID {
			r.state.id = responseID
		}
		r.state.mu.Unlock()
	})
}

func ValidatePath(path string) bool {
	parsed, err := url.Parse(path)
	return err == nil && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.Contains(path, `\`) && !parsed.IsAbs() && parsed.Host == ""
}

func IsNormalClose(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}
