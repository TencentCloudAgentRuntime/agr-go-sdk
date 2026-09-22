package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestSessionCloudJourney(t *testing.T) {
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real Session E2E requires AGS_E2E=1")
	}
	for _, name := range []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY"} {
		if os.Getenv(name) == "" {
			t.Fatalf("real Session E2E requires %s", name)
		}
	}
	client := newCloudClient(t, cloudEnvironment{region: "ap-shanghai"})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	runID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	spaces := make([]string, 0, 2)
	sessions := make([]agr.SessionRef, 0, 2)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		for i := len(sessions) - 1; i >= 0; i-- {
			deleteSessionAndConfirm(t, cleanup, client.Sessions(), sessions[i])
		}
		for i := len(spaces) - 1; i >= 0; i-- {
			deleteSessionSpaceAndConfirm(t, cleanup, client.SessionSpaces(), spaces[i])
		}
	}()

	for _, suffix := range []string{"a", "b"} {
		space, err := client.SessionSpaces().Create(ctx, agr.CreateSessionSpaceOptions{
			Name:        "agr-sdk-e2e-session-" + suffix + "-" + runID,
			Description: "AGR SDK A4 owned test resource",
			Tags:        []agr.SessionTag{{Key: "agr-sdk-e2e", Value: "owned"}},
		})
		if err != nil {
			t.Fatalf("create Space %s: %v request=%s", suffix, err, cloudRequestID(err))
		}
		if space.ID == "" || space.Default {
			t.Fatalf("unsafe created Space: %+v", space)
		}
		spaces = append(spaces, space.ID)
	}

	sharedID := "agr-sdk-e2e-shared-" + runID
	refs := []agr.SessionRef{
		{SpaceID: spaces[0], UserID: "user-a-" + runID, SessionID: sharedID},
		{SpaceID: spaces[1], UserID: "user-b-" + runID, SessionID: sharedID},
	}
	states := []json.RawMessage{json.RawMessage(`{"tenant":"a","turn":0}`), json.RawMessage(`{"tenant":"b","turn":0}`)}
	for i, ref := range refs {
		created, err := client.Sessions().Create(ctx, agr.CreateSessionOptions{
			Ref:      ref,
			Title:    fmt.Sprintf("session-%d", i),
			State:    agr.SessionState{CustomState: states[i]},
			Metadata: []agr.SessionMetadata{{Name: "owner", Value: fmt.Sprintf("space-%d", i)}},
		})
		if err != nil {
			t.Fatalf("create Session %d: %v request=%s", i, err, cloudRequestID(err))
		}
		if created.Ref != ref || !jsonEqual(created.State.CustomState, states[i]) {
			t.Fatalf("created Session %d = %+v state=%s", i, created, created.State.CustomState)
		}
		sessions = append(sessions, ref)
	}
	for i, ref := range refs {
		got, err := client.Sessions().Get(ctx, ref)
		if err != nil || got.Ref != ref || !jsonEqual(got.State.CustomState, states[i]) {
			t.Fatalf("isolated Get %d = %+v state=%s err=%v", i, got, got.State.CustomState, err)
		}
	}

	after := time.Now().Add(-time.Second)
	// The service may omit an optional false zero value in responses. Request-level false
	// preservation is locked by the captured-request test; use true here to verify Cloud round-trip.
	text, thought := "hello from A4", true
	stateDelta := json.RawMessage(`{"tenant":"a","turn":1}`)
	appended, err := client.Sessions().AppendEvent(ctx, refs[0], agr.EventInput{
		ID:           "event-" + runID,
		InvocationID: "invocation-" + runID,
		Author:       "user",
		Content: &agr.EventContent{Role: "user", Parts: []agr.EventPart{
			{Text: &text, Thought: &thought, FunctionCall: json.RawMessage(`{"name":"lookup","arguments":{}}`)},
			{InlineData: &agr.EventInlineData{MIMEType: "text/plain", Data: "aGVsbG8="}},
		}},
		Actions:    &agr.EventActions{StateDelta: stateDelta},
		Metadata:   json.RawMessage(`{"source":"e2e"}`),
		Extensions: json.RawMessage(`{"trace":"a4"}`),
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v request=%s chain=%s", err, cloudRequestID(err), sessionErrorChain(err))
	}
	if appended.ID == "" || appended.Timestamp.IsZero() || appended.Content == nil || len(appended.Content.Parts) != 2 {
		t.Fatalf("AppendEvent response = %+v", appended)
	}

	events, err := client.Sessions().ListEvents(ctx, refs[0], agr.EventListOptions{Author: "user", AfterTimestamp: &after, Limit: 1})
	if err != nil {
		t.Fatalf("ListEvents: %v request=%s", err, cloudRequestID(err))
	}
	if events.TotalCount != 1 || len(events.Items) != 1 || events.Items[0].ID != appended.ID || events.Items[0].Content == nil || events.Items[0].Content.Parts[0].Thought == nil || !*events.Items[0].Content.Parts[0].Thought {
		t.Fatalf("events = %+v", events)
	}

	title := "updated-" + runID
	metadata := []agr.SessionMetadata{{Name: "owner", Value: "updated"}, {Name: "empty", Value: ""}}
	updated, err := client.Sessions().Update(ctx, refs[0], agr.UpdateSessionOptions{Title: &title, Metadata: &metadata})
	if err != nil || updated.Title != title || !reflect.DeepEqual(updated.Metadata, metadata) {
		t.Fatalf("Update = %+v err=%v", updated, err)
	}
	got, err := client.Sessions().Get(ctx, refs[0])
	if err != nil || got.EventCount != 1 || !jsonEqual(got.State.CustomState, stateDelta) {
		t.Fatalf("Get after event = %+v state=%s err=%v", got, got.State.CustomState, err)
	}
	page, err := client.Sessions().List(ctx, agr.SessionListOptions{
		SpaceID:    refs[0].SpaceID,
		UserIDs:    []string{refs[0].UserID},
		SessionIDs: []string{sharedID},
		Filters:    []agr.SessionFilter{{Name: "title", Values: []string{title}}},
		Limit:      1,
	})
	if err != nil || page.TotalCount != 1 || len(page.Items) != 1 || page.Items[0].Ref != refs[0] || page.Items[0].EventCount != got.EventCount || events.TotalCount != got.EventCount {
		t.Fatalf("Session/Event counts: get=%+v list=%+v events=%+v err=%v", got, page, events, err)
	}
	otherEvents, err := client.Sessions().ListEvents(ctx, refs[1], agr.EventListOptions{Limit: 1})
	if err != nil || otherEvents.TotalCount != 0 || len(otherEvents.Items) != 0 {
		t.Fatalf("cross-Space event isolation = %+v err=%v", otherEvents, err)
	}

	for i := range sessions {
		deleteSessionAndConfirm(t, ctx, client.Sessions(), sessions[i])
		sessions[i] = agr.SessionRef{}
	}
	for i := range spaces {
		deleteSessionSpaceAndConfirm(t, ctx, client.SessionSpaces(), spaces[i])
		spaces[i] = ""
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}

func deleteSessionAndConfirm(t *testing.T, ctx context.Context, manager *agr.SessionManager, ref agr.SessionRef) {
	t.Helper()
	if ref.SpaceID == "" {
		return
	}
	if err := manager.Delete(ctx, ref); err != nil && !isSDKNotFound(err) {
		t.Errorf("delete Session: %v request=%s", err, cloudRequestID(err))
		return
	}
	for {
		_, err := manager.Get(ctx, ref)
		if isSDKNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("confirm Session cleanup: %v", err)
			return
		}
		select {
		case <-ctx.Done():
			t.Errorf("confirm Session cleanup: %v", ctx.Err())
			return
		case <-time.After(time.Second):
		}
	}
}

func deleteSessionSpaceAndConfirm(t *testing.T, ctx context.Context, manager *agr.SessionSpaceManager, id string) {
	t.Helper()
	if id == "" {
		return
	}
	if err := manager.Delete(ctx, id); err != nil && !isSDKNotFound(err) {
		t.Errorf("delete SessionSpace: %v request=%s", err, cloudRequestID(err))
		return
	}
	for {
		_, err := manager.Get(ctx, id)
		if isSDKNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("confirm SessionSpace cleanup: %v", err)
			return
		}
		select {
		case <-ctx.Done():
			t.Errorf("confirm SessionSpace cleanup: %v", ctx.Err())
			return
		case <-time.After(time.Second):
		}
	}
}

func isSDKNotFound(err error) bool {
	var failure *agr.Error
	return errors.As(err, &failure) && failure.Code == agr.NotFound
}

func sessionErrorChain(err error) string {
	result := ""
	for current, depth := err, 0; current != nil && depth < 8; current, depth = errors.Unwrap(current), depth+1 {
		if result != "" {
			result += " <- "
		}
		result += fmt.Sprintf("%T:%s", current, current.Error())
	}
	return result
}
