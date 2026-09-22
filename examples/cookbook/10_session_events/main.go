package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(parent context.Context) (err error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	client, err := agr.NewClient(agr.WithRegion("ap-shanghai"))
	if err != nil {
		return err
	}

	space, err := client.SessionSpaces().Create(ctx, agr.CreateSessionSpaceOptions{
		Name:        fmt.Sprintf("agr-session-example-%d", time.Now().UnixNano()),
		Description: "AGR Go SDK Session example",
	})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if cleanupErr := client.SessionSpaces().Delete(cleanup, space.ID); err == nil {
			err = cleanupErr
		}
	}()

	created, err := client.Sessions().Create(ctx, agr.CreateSessionOptions{
		Ref:      agr.SessionRef{SpaceID: space.ID, UserID: "cookbook-user"},
		Title:    "first conversation",
		State:    agr.SessionState{CustomState: json.RawMessage(`{"turn":0}`)},
		Metadata: []agr.SessionMetadata{{Name: "environment", Value: "example"}},
	})
	if err != nil {
		return err
	}
	ref := created.Ref
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if cleanupErr := client.Sessions().Delete(cleanup, ref); err == nil {
			err = cleanupErr
		}
	}()

	thought := false
	text := "hello"
	event, err := client.Sessions().AppendEvent(ctx, ref, agr.EventInput{
		Author: "user",
		Content: &agr.EventContent{Role: "user", Parts: []agr.EventPart{{
			Text:    &text,
			Thought: &thought,
		}}},
		Actions:  &agr.EventActions{StateDelta: json.RawMessage(`{"turn":1}`)},
		Metadata: json.RawMessage(`{"source":"cookbook"}`),
	})
	if err != nil {
		return err
	}
	page, err := client.Sessions().ListEvents(ctx, ref, agr.EventListOptions{Author: "user", Limit: 50})
	if err != nil {
		return err
	}

	title := "updated conversation"
	metadata := []agr.SessionMetadata{{Name: "environment", Value: "example"}, {Name: "stage", Value: "updated"}}
	updated, err := client.Sessions().Update(ctx, ref, agr.UpdateSessionOptions{Title: &title, Metadata: &metadata})
	if err != nil {
		return err
	}
	fmt.Printf("space=%s session=%s event=%s count=%d title=%q\n", space.ID, ref.SessionID, event.ID, page.TotalCount, updated.Title)
	return nil
}
