package deployment

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnectionForwardsHTTPSSEAndWebSocketWithTokenAndAffinity(t *testing.T) {
	const affinityHeader = "X-Workspace-Session"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var requests atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Access-Token") != "dpt_private" || r.Header.Get("Authorization") != "Bearer business" || r.Header.Get("X-Business") != "keep" {
			http.Error(w, "headers missing", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Origin") != "https://8080-dpl-a1b2c3d4.ap-test.agents.example.test" {
			http.Error(w, "origin was not normalized", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/http":
			if requests.Add(1) != 1 || r.Header.Get(affinityHeader) != "" {
				http.Error(w, "unexpected discovery request", http.StatusBadRequest)
				return
			}
			w.Header().Set(affinityHeader, "session-one")
			fmt.Fprint(w, "http-ok")
		case "/sse":
			if requests.Add(1) != 2 || r.Header.Get(affinityHeader) != "session-one" {
				http.Error(w, "affinity was not reused", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: ready\n\n")
			w.(http.Flusher).Flush()
		case "/ws":
			if requests.Add(1) != 3 || r.Header.Get(affinityHeader) != "session-one" {
				http.Error(w, "websocket affinity missing", http.StatusBadRequest)
				return
			}
			responseHeader := http.Header{affinityHeader: []string{"session-two"}}
			conn, err := upgrader.Upgrade(w, r, responseHeader)
			if err != nil {
				return
			}
			defer conn.Close()
			typeID, data, err := conn.ReadMessage()
			if err == nil {
				_ = conn.WriteMessage(typeID, data)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	var tokenCalls atomic.Int32
	connection := newTestConnection(t, upstream, Config{
		DeploymentID: "dpl-a1b2c3d4", Region: "ap-test", Domain: "example.test", Port: 8080,
		AffinityHeader: affinityHeader,
		TokenSource: func(context.Context, string) (Token, error) {
			tokenCalls.Add(1)
			return Token{Value: "dpt_private", ExpiresAt: time.Now().Add(3 * time.Hour)}, nil
		},
	})
	defer connection.Close()
	headers := http.Header{"Authorization": []string{"Bearer business"}, "X-Business": []string{"keep"}, "Origin": []string{"https://caller.invalid"}, "X-Access-Token": []string{"caller-value"}}
	response, err := connection.Do(context.Background(), Request{Method: http.MethodGet, Path: "/http", Header: headers})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "http-ok" || connection.AffinityID() != "session-one" {
		t.Fatalf("HTTP response=%d %q affinity=%q", response.StatusCode, body, connection.AffinityID())
	}
	response, err = connection.Do(context.Background(), Request{Method: http.MethodGet, Path: "/sse", Header: headers})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" || string(body) != "data: ready\n\n" {
		t.Fatalf("SSE headers=%v body=%q", response.Header, body)
	}
	socket, response, err := connection.DialWebSocket(context.Background(), "/ws", headers)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal(err)
	}
	defer socket.Close()
	if err = socket.Write(context.Background(), Message{Type: websocket.TextMessage, Data: []byte("echo")}); err != nil {
		t.Fatal(err)
	}
	message, err := socket.Read(context.Background())
	if err != nil || string(message.Data) != "echo" || connection.AffinityID() != "session-two" {
		t.Fatalf("WebSocket message=%q err=%v affinity=%q", message.Data, err, connection.AffinityID())
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls=%d, want one cached token", tokenCalls.Load())
	}
	if headers.Get("X-Access-Token") != "caller-value" || headers.Get("Origin") != "https://caller.invalid" {
		t.Fatalf("caller headers were mutated: %v", headers)
	}
}

func TestUnauthorizedInvalidatesTokenWithoutReplay(t *testing.T) {
	var requests, tokens atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := requests.Add(1)
		if call == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	connection := newTestConnection(t, upstream, Config{DeploymentID: "dpl-test", Region: "ap-test", Domain: "example.test", Port: 8080, TokenSource: func(context.Context, string) (Token, error) {
		call := tokens.Add(1)
		return Token{Value: fmt.Sprintf("dpt_%d", call), ExpiresAt: time.Now().Add(3 * time.Hour)}, nil
	}})
	defer connection.Close()
	first, err := connection.Do(context.Background(), Request{Method: http.MethodPost, Path: "/mutate"})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()
	if first.StatusCode != http.StatusUnauthorized || requests.Load() != 1 {
		t.Fatalf("first status=%d requests=%d; request was replayed", first.StatusCode, requests.Load())
	}
	second, err := connection.Do(context.Background(), Request{Method: http.MethodGet, Path: "/next"})
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Body.Close()
	if second.StatusCode != http.StatusOK || requests.Load() != 2 || tokens.Load() != 2 {
		t.Fatalf("second=%d requests=%d tokens=%d", second.StatusCode, requests.Load(), tokens.Load())
	}
}

func TestTokenManagerCoalescesRefreshAndFallsBackOnlyWhileValid(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	manager := newTokenManager(context.Background(), "dpl", func(context.Context, string) (Token, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return Token{Value: "dpt_shared", ExpiresAt: now.Add(3 * time.Hour)}, nil
	}, func() time.Time { return now })
	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			token, err := manager.Token(context.Background())
			if err != nil || token != "dpt_shared" {
				errs <- fmt.Errorf("token=%q err=%v", token, err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("refresh calls=%d", calls.Load())
	}

	now = now.Add(90 * time.Minute)
	manager.source = func(context.Context, string) (Token, error) { return Token{}, fmt.Errorf("refresh failed") }
	if token, err := manager.Token(context.Background()); err != nil || token != "dpt_shared" {
		t.Fatalf("valid fallback token=%q err=%v", token, err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := manager.Token(context.Background()); err == nil {
		t.Fatal("expired token was used as refresh fallback")
	}
}

func TestTokenRefreshDoesNotIgnoreCallerCancellation(t *testing.T) {
	lifecycle, stop := context.WithCancel(context.Background())
	defer stop()
	started := make(chan struct{})
	manager := newTokenManager(lifecycle, "dpl", func(ctx context.Context, _ string) (Token, error) {
		close(started)
		<-ctx.Done()
		return Token{}, ctx.Err()
	}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := manager.Token(ctx)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Token error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Token ignored caller cancellation")
	}
}

func TestConnectionCloseCancelsActiveSSEAndWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	sseStarted := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			close(sseStarted)
			<-r.Context().Done()
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		}
	}))
	defer upstream.Close()
	connection := newTestConnection(t, upstream, Config{DeploymentID: "dpl-test", Region: "ap-test", Domain: "example.test", Port: 8080, TokenSource: validTokenSource})
	response, err := connection.Do(context.Background(), Request{Method: http.MethodGet, Path: "/sse"})
	if err != nil {
		t.Fatal(err)
	}
	<-sseStarted
	socket, _, err := connection.DialWebSocket(context.Background(), "/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() { _, err := response.Body.Read(make([]byte, 1)); readDone <- err }()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("SSE read survived connection close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSE read was not canceled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err = socket.Read(ctx); err == nil {
		t.Fatal("WebSocket read survived connection close")
	}
}

func TestConnectionCloseDoesNotCloseCallerTransport(t *testing.T) {
	transport := &closeTrackingTransport{}
	connection, err := New(Config{
		DeploymentID: "dpl-test",
		Region:       "ap-test",
		Domain:       "example.test",
		Port:         8080,
		TokenSource:  validTokenSource,
		HTTPClient:   &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if transport.closed.Load() != 0 {
		t.Fatal("connection close took ownership of caller transport")
	}
}

func TestWebSocketReadLimitRejectsOversizedMessage(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, maxWebSocketMessageBytes+1))
	}))
	defer upstream.Close()
	connection := newTestConnection(t, upstream, Config{
		DeploymentID: "dpl-test", Region: "ap-test", Domain: "example.test", Port: 8080,
		TokenSource: validTokenSource,
	})
	defer connection.Close()
	socket, response, err := connection.DialWebSocket(context.Background(), "/ws", nil)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal(err)
	}
	defer socket.Close()
	if _, err = socket.Read(context.Background()); err == nil {
		t.Fatal("oversized WebSocket message was accepted")
	}
}

type closeTrackingTransport struct{ closed atomic.Int32 }

func (*closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (t *closeTrackingTransport) CloseIdleConnections() { t.closed.Add(1) }

func TestAffinityIgnoresStaleConcurrentRotation(t *testing.T) {
	state := newAffinityState("X-Affinity", "one")
	first, _ := state.acquire(context.Background())
	second, _ := state.acquire(context.Background())
	first.complete("two")
	second.complete("stale")
	if state.currentID() != "two" {
		t.Fatalf("affinity=%q", state.currentID())
	}
}

func newTestConnection(t *testing.T, upstream *httptest.Server, config Config) *Connection {
	t.Helper()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec -- loopback fixture only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	}
	config.HTTPClient = &http.Client{Transport: transport}
	value, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func validTokenSource(context.Context, string) (Token, error) {
	return Token{Value: "dpt_private", ExpiresAt: time.Now().Add(3 * time.Hour)}, nil
}

func TestValidatePathRejectsForeignAuthorities(t *testing.T) {
	for _, path := range []string{"", "relative", "//foreign.invalid/path", "https://foreign.invalid/path", "/\\foreign"} {
		if ValidatePath(path) {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, path := range []string{"/", "/v1/items?limit=1", "/events"} {
		if !ValidatePath(path) {
			t.Fatalf("rejected %q", path)
		}
	}
}

func TestTokenErrorsDoNotContainToken(t *testing.T) {
	connection, err := New(Config{DeploymentID: "dpl", Region: "ap", Domain: "example.test", Port: 1, TokenSource: func(context.Context, string) (Token, error) {
		return Token{Value: "dpt_should_not_leak", ExpiresAt: time.Time{}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connection.Do(context.Background(), Request{Method: http.MethodGet, Path: "/"})
	if err == nil || strings.Contains(err.Error(), "dpt_should_not_leak") {
		t.Fatalf("unsafe error=%v", err)
	}
}
