package agr

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type sessionTestControl struct {
	metricControl
	calls  int
	create CreateSessionOptions
	event  EventInput
	list   SessionListOptions
	update UpdateSessionOptions
}

func (c *sessionTestControl) CreateSessionSpace(context.Context, CreateSessionSpaceOptions) (SessionSpaceInfo, error) {
	c.calls++
	return SessionSpaceInfo{ID: "space", Status: SessionSpaceStatus("Future")}, nil
}
func (c *sessionTestControl) GetSessionSpace(context.Context, string) (SessionSpaceInfo, error) {
	c.calls++
	return SessionSpaceInfo{}, nil
}
func (c *sessionTestControl) ListSessionSpaces(context.Context, SessionSpaceListOptions) (SessionSpacePage, error) {
	c.calls++
	return SessionSpacePage{}, nil
}
func (c *sessionTestControl) UpdateSessionSpace(context.Context, string, UpdateSessionSpaceOptions) (SessionSpaceInfo, error) {
	c.calls++
	return SessionSpaceInfo{}, nil
}
func (c *sessionTestControl) DeleteSessionSpace(context.Context, string) error { c.calls++; return nil }
func (c *sessionTestControl) CreateSession(_ context.Context, value CreateSessionOptions) (SessionInfo, error) {
	c.calls++
	c.create = value
	return SessionInfo{Ref: value.Ref}, nil
}
func (c *sessionTestControl) GetSession(context.Context, SessionRef) (SessionInfo, error) {
	c.calls++
	return SessionInfo{}, nil
}
func (c *sessionTestControl) ListSessions(_ context.Context, value SessionListOptions) (SessionPage, error) {
	c.calls++
	c.list = value
	next := value.Offset + 1
	return SessionPage{TotalCount: 2, Items: []SessionInfo{{}}, NextOffset: &next}, nil
}
func (c *sessionTestControl) UpdateSession(_ context.Context, _ SessionRef, value UpdateSessionOptions) (SessionInfo, error) {
	c.calls++
	c.update = value
	return SessionInfo{}, nil
}
func (c *sessionTestControl) DeleteSession(context.Context, SessionRef) error { c.calls++; return nil }
func (c *sessionTestControl) AppendEvent(_ context.Context, _ SessionRef, value EventInput) (EventInfo, error) {
	c.calls++
	c.event = value
	return EventInfo{}, nil
}
func (c *sessionTestControl) ListEvents(context.Context, SessionRef, EventListOptions) (EventPage, error) {
	c.calls++
	return EventPage{}, nil
}

func newSessionTestClient(t *testing.T, control *sessionTestControl) *Client {
	t.Helper()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{SecretID: "id", SecretKey: "key"}), withControlPlane(control), withMonitorTransport(&metricTransport{}))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSessionInvalidInputMakesNoNetworkCall(t *testing.T) {
	control := &sessionTestControl{}
	client := newSessionTestClient(t, control)
	ctx := context.Background()
	badJSON := json.RawMessage(`[]`)
	calls := []func() error{
		func() error {
			_, err := client.SessionSpaces().Create(ctx, CreateSessionSpaceOptions{Name: "  "})
			return err
		},
		func() error { _, err := client.Sessions().Get(ctx, SessionRef{SessionID: "id"}); return err },
		func() error {
			_, err := client.Sessions().Create(ctx, CreateSessionOptions{Ref: SessionRef{SpaceID: "s", UserID: "u"}, State: SessionState{CustomState: badJSON}})
			return err
		},
		func() error {
			_, err := client.Sessions().Update(ctx, SessionRef{SpaceID: "s", UserID: "u", SessionID: "x"}, UpdateSessionOptions{})
			return err
		},
		func() error {
			_, err := client.Sessions().AppendEvent(ctx, SessionRef{SpaceID: "s", UserID: "u", SessionID: "x"}, EventInput{Extensions: badJSON})
			return err
		},
	}
	for _, call := range calls {
		var sdk *Error
		if err := call(); !errors.As(err, &sdk) || sdk.Code != InvalidArgument {
			t.Fatalf("error=%#v", err)
		}
	}
	if control.calls != 0 {
		t.Fatalf("invalid input made %d calls", control.calls)
	}
}

func TestSessionInputsAreDeepCopiedAndPresencePreserved(t *testing.T) {
	control := &sessionTestControl{}
	client := newSessionTestClient(t, control)
	ctx := context.Background()
	state := json.RawMessage(`{"count":0}`)
	metadata := []SessionMetadata{{Name: "env", Value: "test"}}
	if _, err := client.Sessions().Create(ctx, CreateSessionOptions{Ref: SessionRef{SpaceID: "s", UserID: "u"}, State: SessionState{CustomState: state}, Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	state[2] = 'X'
	metadata[0].Value = "changed"
	if string(control.create.State.CustomState) != `{"count":0}` || control.create.Metadata[0].Value != "test" {
		t.Fatalf("create input escaped: %#v", control.create)
	}
	falseValue := false
	empty := ""
	function := json.RawMessage(`{"name":"f"}`)
	event := EventInput{Content: &EventContent{Parts: []EventPart{{Text: &empty, Thought: &falseValue, FunctionCall: function}}}}
	ref := SessionRef{SpaceID: "s", UserID: "u", SessionID: "x"}
	if _, err := client.Sessions().AppendEvent(ctx, ref, event); err != nil {
		t.Fatal(err)
	}
	function[2] = 'X'
	falseValue = true
	if control.event.Content.Parts[0].Thought == nil || *control.event.Content.Parts[0].Thought || string(control.event.Content.Parts[0].FunctionCall) != `{"name":"f"}` {
		t.Fatalf("event input escaped: %#v", control.event)
	}
	emptyMetadata := []SessionMetadata{}
	if _, err := client.Sessions().Update(ctx, ref, UpdateSessionOptions{Title: &empty, Metadata: &emptyMetadata}); err != nil {
		t.Fatal(err)
	}
	if control.update.Title == nil || *control.update.Title != "" || control.update.Metadata == nil || len(*control.update.Metadata) != 0 {
		t.Fatalf("presence lost: %#v", control.update)
	}
}

func TestSessionPaginationAndIsolationAreExplicit(t *testing.T) {
	first, second := &sessionTestControl{}, &sessionTestControl{}
	a, b := newSessionTestClient(t, first), newSessionTestClient(t, second)
	page, err := a.Sessions().List(context.Background(), SessionListOptions{SpaceID: "space-a", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextOffset == nil || *page.NextOffset != 1 || first.list.SpaceID != "space-a" {
		t.Fatalf("page=%#v request=%#v", page, first.list)
	}
	if _, err = b.Sessions().Get(context.Background(), SessionRef{SpaceID: "space-b", UserID: "same-user", SessionID: "same-session"}); err != nil {
		t.Fatal(err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("client transports crossed: first=%d second=%d", first.calls, second.calls)
	}
}

func TestEventListRejectsInvalidTimestampAndDefaultsLimit(t *testing.T) {
	control := &sessionTestControl{}
	client := newSessionTestClient(t, control)
	ref := SessionRef{SpaceID: "s", UserID: "u", SessionID: "x"}
	zero := time.Time{}
	if _, err := client.Sessions().ListEvents(context.Background(), ref, EventListOptions{AfterTimestamp: &zero}); err == nil {
		t.Fatal("zero timestamp accepted")
	}
	if control.calls != 0 {
		t.Fatal("invalid timestamp reached transport")
	}
}
