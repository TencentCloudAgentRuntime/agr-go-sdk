package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionCommonRequestMappingCoversAllActionsAndPresence(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		action := r.Header.Get("X-Tc-Action")
		mu.Lock()
		bodies[action] = body
		mu.Unlock()
		response := map[string]any{"RequestId": "request-1"}
		switch action {
		case "CreateSessionSpace", "DescribeSessionSpace", "ModifySessionSpace":
			response["SessionSpace"] = validSessionSpaceResponse()
		case "DescribeSessionSpaces":
			response["SessionSpaces"], response["TotalCount"] = []any{validSessionSpaceResponse()}, 1
		case "CreateSession", "DescribeSession", "ModifySession":
			response["Session"] = validSessionResponse()
		case "DescribeSessions":
			response["Sessions"], response["TotalCount"] = []any{validSessionResponse()}, 1
		case "AppendEvent":
			response["Event"] = validEventResponse()
		case "DescribeEvents":
			response["Events"], response["TotalCount"] = []any{validEventResponse()}, 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-shanghai", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) {
		return Credential{SecretID: "id", SecretKey: "key", Token: "session-token"}, nil
	})
	ctx := context.Background()
	space, session := "space-1", SessionRef{SpaceID: "space-1", UserID: "user-1", SessionID: "session-1"}
	if _, err := api.CreateSessionSpace(ctx, CreateSessionSpaceRequest{Name: "name"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetSessionSpace(ctx, space); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListSessionSpaces(ctx, SessionSpaceListRequest{Filters: []SessionFilter{{Name: "name", Values: []string{"x"}}}}); err != nil {
		t.Fatal(err)
	}
	emptyDescription := ""
	if _, err := api.UpdateSessionSpace(ctx, space, UpdateSessionSpaceRequest{Name: "name", Description: &emptyDescription}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteSessionSpace(ctx, space); err != nil {
		t.Fatal(err)
	}
	if _, err := api.CreateSession(ctx, CreateSessionRequest{Ref: session, State: SessionState{CustomState: json.RawMessage(`{"count":0}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListSessions(ctx, SessionListRequest{SpaceID: space}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	metadata := []SessionMetadata{}
	if _, err := api.UpdateSession(ctx, session, UpdateSessionRequest{Title: &empty, Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	falseValue := false
	if _, err := api.AppendEvent(ctx, session, Event{Author: "user", Content: &EventContent{Parts: []EventPart{{Thought: &falseValue, FunctionCall: json.RawMessage(`{"name":"f"}`)}}}, Metadata: json.RawMessage(`{"n":1}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListEvents(ctx, EventListRequest{Ref: session}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	want := []string{"AppendEvent", "CreateSession", "CreateSessionSpace", "DeleteSession", "DeleteSessionSpace", "DescribeEvents", "DescribeSession", "DescribeSessionSpace", "DescribeSessionSpaces", "DescribeSessions", "ModifySession", "ModifySessionSpace"}
	for _, action := range want {
		if bodies[action] == nil {
			t.Errorf("missing request for %s", action)
		}
	}
	if got, ok := bodies["CreateSession"]["State"].(map[string]any)["CustomState"].(string); !ok || got != `{"count":0}` {
		t.Fatalf("CustomState double encoded or absent: %#v", bodies["CreateSession"])
	}
	modify := bodies["ModifySession"]
	if title, ok := modify["Title"]; !ok || title != "" {
		t.Fatalf("explicit empty Title lost: %#v", modify)
	}
	if values, ok := modify["Metadata"].([]any); !ok || len(values) != 0 {
		t.Fatalf("explicit empty Metadata lost: %#v", modify)
	}
	part := bodies["AppendEvent"]["Event"].(map[string]any)["Content"].(map[string]any)["Parts"].([]any)[0].(map[string]any)
	if thought, ok := part["Thought"]; !ok || thought != false {
		t.Fatalf("explicit false Thought lost: %#v", part)
	}
	if _, ok := part["FunctionCall"].(string); !ok {
		t.Fatalf("FunctionCall was not encoded once as protocol string: %#v", part)
	}
}

func TestAppendEventCancellationDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := api.AppendEvent(ctx, SessionRef{SpaceID: "s", UserID: "u", SessionID: "x"}, Event{})
	if !errors.Is(err, context.Canceled) || calls.Load() > 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestSessionCommonRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		action   string
		response map[string]any
		call     func(*AGS) error
	}{
		{"DescribeSessionSpaces", map[string]any{"SessionSpaces": "bad", "TotalCount": 0, "RequestId": "r"}, func(a *AGS) error {
			_, err := a.ListSessionSpaces(context.Background(), SessionSpaceListRequest{})
			return err
		}},
		{"DescribeSessions", map[string]any{"Sessions": []any{validSessionResponse()}, "TotalCount": 1}, func(a *AGS) error {
			_, err := a.ListSessions(context.Background(), SessionListRequest{SpaceID: "s"})
			return err
		}},
		{"AppendEvent", map[string]any{"Event": map[string]any{"Timestamp": "bad"}, "RequestId": "r"}, func(a *AGS) error { _, err := a.AppendEvent(context.Background(), SessionRef{}, Event{}); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"Response": tc.response})
			}))
			defer server.Close()
			api := NewAGS(Config{Region: "ap-test", Endpoint: server.URL, Timeout: time.Second, Transport: server.Client().Transport}, func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil })
			var cloud *Error
			if err := tc.call(api); !errors.As(err, &cloud) || cloud.Code != "ClientError.MalformedResponse" {
				t.Fatalf("error=%#v", err)
			}
		})
	}
}

func validSessionSpaceResponse() map[string]any {
	return map[string]any{"SpaceId": "space-1", "Name": "name", "Status": "FutureStatus", "Default": false, "CreateTime": "2026-09-22T01:02:03Z", "UpdateTime": "2026-09-22T01:02:04Z"}
}
func validSessionResponse() map[string]any {
	return map[string]any{"SessionId": "session-1", "SpaceId": "space-1", "UserId": "user-1", "State": map[string]any{"CustomState": `{"count":1}`}, "Metadata": []any{}, "EventCount": 0, "CreateTime": "2026-09-22T01:02:03Z", "UpdateTime": "2026-09-22T01:02:04Z"}
}
func validEventResponse() map[string]any {
	return map[string]any{"EventId": "event-1", "Author": "user", "Metadata": `{"n":1}`, "Content": map[string]any{"Parts": []any{map[string]any{"FunctionResponse": "null"}}}, "Timestamp": "2026-09-22T01:02:05Z"}
}
