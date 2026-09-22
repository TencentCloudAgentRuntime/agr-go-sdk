package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"
)

// SessionFilter is a reviewed Session or SessionSpace filter.
type SessionFilter struct {
	Name   string
	Values []string
}

// SessionMetadata is one ordered metadata entry.
type SessionMetadata struct{ Name, Value string }

// SessionSpace is an immutable service snapshot.
type SessionSpace struct {
	ID, Name, Description, Status string
	Default                       bool
	CreatedAt, UpdatedAt          time.Time
}

type CreateSessionSpaceRequest struct {
	Name, Description string
	Tags              []VolumeTag
}

type UpdateSessionSpaceRequest struct {
	Name        string
	Description *string
}

type SessionSpaceListRequest struct {
	Offset, Limit int
	Filters       []SessionFilter
}

type SessionSpaceResult struct {
	Space     SessionSpace
	RequestID string
}

type SessionSpacePage struct {
	Items      []SessionSpace
	TotalCount int64
	RequestID  string
}

// SessionRef is the complete service identity of one Session.
type SessionRef struct{ SpaceID, UserID, SessionID string }

type SessionState struct{ CustomState json.RawMessage }

type Session struct {
	Ref                  SessionRef
	Title                string
	State                SessionState
	Metadata             []SessionMetadata
	EventCount           int64
	CreatedAt, UpdatedAt time.Time
}

type CreateSessionRequest struct {
	Ref      SessionRef
	Title    string
	State    SessionState
	Metadata []SessionMetadata
}

type UpdateSessionRequest struct {
	Title    *string
	Metadata *[]SessionMetadata
}

type SessionListRequest struct {
	SpaceID             string
	UserIDs, SessionIDs []string
	Filters             []SessionFilter
	Offset, Limit       int
}

type SessionResult struct {
	Session   Session
	RequestID string
}

type SessionPage struct {
	Items      []Session
	TotalCount int64
	RequestID  string
}

type InlineData struct{ MIMEType, Data string }

type EventPart struct {
	Text             *string
	Thought          *bool
	FunctionCall     json.RawMessage
	FunctionResponse json.RawMessage
	InlineData       *InlineData
}

type EventContent struct {
	Role  string
	Parts []EventPart
}

type EventActions struct{ StateDelta json.RawMessage }

type Event struct {
	ID, InvocationID, Author string
	Content                  *EventContent
	Actions                  *EventActions
	Metadata, Extensions     json.RawMessage
	ErrorCode, ErrorMessage  string
	Timestamp                time.Time
}

type EventListRequest struct {
	Ref            SessionRef
	Author         string
	AfterTimestamp *time.Time
	Offset, Limit  int
}

type EventResult struct {
	Event     Event
	RequestID string
}

type EventPage struct {
	Items      []Event
	TotalCount int64
	RequestID  string
}

func (s *Service) CreateSessionSpace(ctx context.Context, request CreateSessionSpaceRequest) (SessionSpaceResult, error) {
	out, err := s.ags.CreateSessionSpace(ctx, request)
	return out, cloudError(err, "CreateSessionSpace")
}
func (s *Service) GetSessionSpace(ctx context.Context, id string) (SessionSpaceResult, error) {
	out, err := s.ags.GetSessionSpace(ctx, id)
	return out, cloudError(err, "DescribeSessionSpace")
}
func (s *Service) ListSessionSpaces(ctx context.Context, request SessionSpaceListRequest) (SessionSpacePage, error) {
	out, err := s.ags.ListSessionSpaces(ctx, request)
	return out, cloudError(err, "DescribeSessionSpaces")
}
func (s *Service) UpdateSessionSpace(ctx context.Context, id string, request UpdateSessionSpaceRequest) (SessionSpaceResult, error) {
	out, err := s.ags.UpdateSessionSpace(ctx, id, request)
	return out, cloudError(err, "ModifySessionSpace")
}
func (s *Service) DeleteSessionSpace(ctx context.Context, id string) error {
	return cloudError(s.ags.DeleteSessionSpace(ctx, id), "DeleteSessionSpace")
}
func (s *Service) CreateSession(ctx context.Context, request CreateSessionRequest) (SessionResult, error) {
	out, err := s.ags.CreateSession(ctx, request)
	return out, cloudError(err, "CreateSession")
}
func (s *Service) GetSession(ctx context.Context, ref SessionRef) (SessionResult, error) {
	out, err := s.ags.GetSession(ctx, ref)
	return out, cloudError(err, "DescribeSession")
}
func (s *Service) ListSessions(ctx context.Context, request SessionListRequest) (SessionPage, error) {
	out, err := s.ags.ListSessions(ctx, request)
	return out, cloudError(err, "DescribeSessions")
}
func (s *Service) UpdateSession(ctx context.Context, ref SessionRef, request UpdateSessionRequest) (SessionResult, error) {
	out, err := s.ags.UpdateSession(ctx, ref, request)
	return out, cloudError(err, "ModifySession")
}
func (s *Service) DeleteSession(ctx context.Context, ref SessionRef) error {
	return cloudError(s.ags.DeleteSession(ctx, ref), "DeleteSession")
}
func (s *Service) AppendEvent(ctx context.Context, ref SessionRef, event Event) (EventResult, error) {
	out, err := s.ags.AppendEvent(ctx, ref, event)
	return out, cloudError(err, "AppendEvent")
}
func (s *Service) ListEvents(ctx context.Context, request EventListRequest) (EventPage, error) {
	out, err := s.ags.ListEvents(ctx, request)
	return out, cloudError(err, "DescribeEvents")
}

type sessionFilterWire struct {
	Name   string   `json:"Name"`
	Values []string `json:"Values"`
}
type sessionMetadataWire struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}
type sessionStateWire struct {
	CustomState string `json:"CustomState,omitempty"`
}
type sessionSpaceWire struct {
	ID          string `json:"SpaceId"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Status      string `json:"Status"`
	Default     bool   `json:"Default"`
	CreateTime  string `json:"CreateTime"`
	UpdateTime  string `json:"UpdateTime"`
}
type sessionWire struct {
	SessionID  string                `json:"SessionId"`
	SpaceID    string                `json:"SpaceId"`
	UserID     string                `json:"UserId"`
	Title      string                `json:"Title"`
	State      *sessionStateWire     `json:"State"`
	Metadata   []sessionMetadataWire `json:"Metadata"`
	EventCount *int64                `json:"EventCount"`
	CreateTime string                `json:"CreateTime"`
	UpdateTime string                `json:"UpdateTime"`
}
type inlineDataWire struct {
	MimeType string `json:"MimeType,omitempty"`
	Data     string `json:"Data,omitempty"`
}
type eventPartWire struct {
	Text             *string         `json:"Text,omitempty"`
	Thought          *bool           `json:"Thought,omitempty"`
	FunctionCall     string          `json:"FunctionCall,omitempty"`
	FunctionResponse string          `json:"FunctionResponse,omitempty"`
	InlineData       *inlineDataWire `json:"InlineData,omitempty"`
}
type eventContentWire struct {
	Role  string          `json:"Role,omitempty"`
	Parts []eventPartWire `json:"Parts,omitempty"`
}
type eventActionsWire struct {
	StateDelta string `json:"StateDelta,omitempty"`
}
type eventWire struct {
	EventID      string            `json:"EventId,omitempty"`
	InvocationID string            `json:"InvocationId,omitempty"`
	Author       string            `json:"Author,omitempty"`
	Content      *eventContentWire `json:"Content,omitempty"`
	Actions      *eventActionsWire `json:"Actions,omitempty"`
	Metadata     string            `json:"Metadata,omitempty"`
	Extensions   string            `json:"Extensions,omitempty"`
	ErrorCode    string            `json:"ErrorCode,omitempty"`
	ErrorMessage string            `json:"ErrorMessage,omitempty"`
	Timestamp    string            `json:"Timestamp,omitempty"`
}

func (a *AGS) CreateSessionSpace(ctx context.Context, in CreateSessionSpaceRequest) (SessionSpaceResult, error) {
	req := struct {
		Name        string
		Description string      `json:"Description,omitempty"`
		Tags        []VolumeTag `json:"Tags,omitempty"`
	}{in.Name, in.Description, in.Tags}
	var response struct {
		SessionSpace *sessionSpaceWire
		RequestID    string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "CreateSessionSpace", req, &response); err != nil {
		return SessionSpaceResult{}, err
	}
	space, err := sessionSpaceFromWire(response.SessionSpace)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedSessionResponse("CreateSessionSpace response has no RequestId")
		}
		return SessionSpaceResult{}, err
	}
	return SessionSpaceResult{Space: space, RequestID: response.RequestID}, nil
}

func (a *AGS) GetSessionSpace(ctx context.Context, id string) (SessionSpaceResult, error) {
	var response struct {
		SessionSpace *sessionSpaceWire
		RequestID    string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeSessionSpace", struct {
		SpaceID string `json:"SpaceId"`
	}{id}, &response); err != nil {
		return SessionSpaceResult{}, err
	}
	space, err := sessionSpaceFromWire(response.SessionSpace)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedSessionResponse("DescribeSessionSpace response has no RequestId")
		}
		return SessionSpaceResult{}, err
	}
	return SessionSpaceResult{Space: space, RequestID: response.RequestID}, nil
}

func (a *AGS) ListSessionSpaces(ctx context.Context, in SessionSpaceListRequest) (SessionSpacePage, error) {
	req := struct {
		Offset, Limit int
		Filters       []sessionFilterWire `json:"Filters,omitempty"`
	}{in.Offset, in.Limit, filtersToWire(in.Filters)}
	var response struct {
		Items      json.RawMessage `json:"SessionSpaces"`
		TotalCount *int64
		RequestID  string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeSessionSpaces", req, &response); err != nil {
		return SessionSpacePage{}, err
	}
	var values []*sessionSpaceWire
	if invalidCollection(response.Items) || json.Unmarshal(response.Items, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return SessionSpacePage{}, malformedSessionResponse("DescribeSessionSpaces response is malformed")
	}
	out := SessionSpacePage{Items: make([]SessionSpace, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, value := range values {
		item, err := sessionSpaceFromWire(value)
		if err != nil {
			return SessionSpacePage{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (a *AGS) UpdateSessionSpace(ctx context.Context, id string, in UpdateSessionSpaceRequest) (SessionSpaceResult, error) {
	req := struct {
		SpaceID     string `json:"SpaceId"`
		Name        string
		Description *string `json:"Description,omitempty"`
	}{id, in.Name, in.Description}
	var response struct {
		SessionSpace *sessionSpaceWire
		RequestID    string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "ModifySessionSpace", req, &response); err != nil {
		return SessionSpaceResult{}, err
	}
	space, err := sessionSpaceFromWire(response.SessionSpace)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedSessionResponse("ModifySessionSpace response has no RequestId")
		}
		return SessionSpaceResult{}, err
	}
	return SessionSpaceResult{Space: space, RequestID: response.RequestID}, nil
}

func (a *AGS) DeleteSessionSpace(ctx context.Context, id string) error {
	var response requestIDResponse
	err := a.commonCall(ctx, "DeleteSessionSpace", struct {
		SpaceID string `json:"SpaceId"`
	}{id}, &response)
	if err == nil && response.RequestID == "" {
		return malformedSessionResponse("DeleteSessionSpace response has no RequestId")
	}
	return err
}

func (a *AGS) CreateSession(ctx context.Context, in CreateSessionRequest) (SessionResult, error) {
	req := struct {
		SpaceID   string                `json:"SpaceId"`
		UserID    string                `json:"UserId"`
		SessionID string                `json:"SessionId,omitempty"`
		Title     string                `json:"Title,omitempty"`
		State     *sessionStateWire     `json:"State,omitempty"`
		Metadata  []sessionMetadataWire `json:"Metadata,omitempty"`
	}{SpaceID: in.Ref.SpaceID, UserID: in.Ref.UserID, SessionID: in.Ref.SessionID, Title: in.Title, State: stateToWire(in.State), Metadata: metadataToWire(in.Metadata)}
	var response struct {
		Session   *sessionWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "CreateSession", req, &response); err != nil {
		return SessionResult{}, err
	}
	return sessionResultFromWire("CreateSession", response.Session, response.RequestID)
}

func (a *AGS) GetSession(ctx context.Context, ref SessionRef) (SessionResult, error) {
	var response struct {
		Session   *sessionWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeSession", refWire(ref), &response); err != nil {
		return SessionResult{}, err
	}
	return sessionResultFromWire("DescribeSession", response.Session, response.RequestID)
}

func (a *AGS) ListSessions(ctx context.Context, in SessionListRequest) (SessionPage, error) {
	req := struct {
		SpaceID       string   `json:"SpaceId"`
		UserIDs       []string `json:"UserIds,omitempty"`
		Offset, Limit int
		SessionIDs    []string            `json:"SessionIds,omitempty"`
		Filters       []sessionFilterWire `json:"Filters,omitempty"`
	}{in.SpaceID, in.UserIDs, in.Offset, in.Limit, in.SessionIDs, filtersToWire(in.Filters)}
	var response struct {
		TotalCount *int64
		Items      json.RawMessage `json:"Sessions"`
		RequestID  string          `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeSessions", req, &response); err != nil {
		return SessionPage{}, err
	}
	var values []*sessionWire
	if invalidCollection(response.Items) || json.Unmarshal(response.Items, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return SessionPage{}, malformedSessionResponse("DescribeSessions response is malformed")
	}
	out := SessionPage{Items: make([]Session, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, value := range values {
		item, err := sessionFromWire(value)
		if err != nil {
			return SessionPage{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (a *AGS) UpdateSession(ctx context.Context, ref SessionRef, in UpdateSessionRequest) (SessionResult, error) {
	req := struct {
		SpaceID   string                 `json:"SpaceId"`
		UserID    string                 `json:"UserId"`
		SessionID string                 `json:"SessionId"`
		Title     *string                `json:"Title,omitempty"`
		Metadata  *[]sessionMetadataWire `json:"Metadata,omitempty"`
	}{SpaceID: ref.SpaceID, UserID: ref.UserID, SessionID: ref.SessionID, Title: in.Title}
	if in.Metadata != nil {
		values := metadataToWire(*in.Metadata)
		req.Metadata = &values
	}
	var response struct {
		Session   *sessionWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "ModifySession", req, &response); err != nil {
		return SessionResult{}, err
	}
	return sessionResultFromWire("ModifySession", response.Session, response.RequestID)
}

func (a *AGS) DeleteSession(ctx context.Context, ref SessionRef) error {
	var response requestIDResponse
	err := a.commonCall(ctx, "DeleteSession", refWire(ref), &response)
	if err == nil && response.RequestID == "" {
		return malformedSessionResponse("DeleteSession response has no RequestId")
	}
	return err
}

func sessionResultFromWire(action string, session *sessionWire, requestID string) (SessionResult, error) {
	value, err := sessionFromWire(session)
	if err != nil || requestID == "" {
		if err == nil {
			err = malformedSessionResponse(action + " response has no RequestId")
		}
		return SessionResult{}, err
	}
	return SessionResult{Session: value, RequestID: requestID}, nil
}

func (a *AGS) AppendEvent(ctx context.Context, ref SessionRef, in Event) (EventResult, error) {
	req := struct {
		SpaceID   string    `json:"SpaceId"`
		UserID    string    `json:"UserId"`
		SessionID string    `json:"SessionId"`
		Event     eventWire `json:"Event"`
	}{ref.SpaceID, ref.UserID, ref.SessionID, eventToWire(in)}
	var response struct {
		Event     *eventWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "AppendEvent", req, &response); err != nil {
		return EventResult{}, err
	}
	value, err := eventFromWire(response.Event)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedSessionResponse("AppendEvent response has no RequestId")
		}
		return EventResult{}, err
	}
	return EventResult{Event: value, RequestID: response.RequestID}, nil
}

func (a *AGS) ListEvents(ctx context.Context, in EventListRequest) (EventPage, error) {
	req := struct {
		SpaceID                string `json:"SpaceId"`
		UserID                 string `json:"UserId"`
		SessionID              string `json:"SessionId"`
		Author, AfterTimestamp string `json:",omitempty"`
		Offset, Limit          int
	}{SpaceID: in.Ref.SpaceID, UserID: in.Ref.UserID, SessionID: in.Ref.SessionID, Author: in.Author, Offset: in.Offset, Limit: in.Limit}
	if in.AfterTimestamp != nil {
		req.AfterTimestamp = in.AfterTimestamp.Format(time.RFC3339Nano)
	}
	var response struct {
		Items      json.RawMessage `json:"Events"`
		TotalCount *int64
		RequestID  string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeEvents", req, &response); err != nil {
		return EventPage{}, err
	}
	var values []*eventWire
	if invalidCollection(response.Items) || json.Unmarshal(response.Items, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return EventPage{}, malformedSessionResponse("DescribeEvents response is malformed")
	}
	out := EventPage{Items: make([]Event, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, value := range values {
		item, err := eventFromWire(value)
		if err != nil {
			return EventPage{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func refWire(ref SessionRef) any {
	return struct {
		SpaceID   string `json:"SpaceId"`
		UserID    string `json:"UserId"`
		SessionID string `json:"SessionId"`
	}{ref.SpaceID, ref.UserID, ref.SessionID}
}
func filtersToWire(values []SessionFilter) []sessionFilterWire {
	out := make([]sessionFilterWire, len(values))
	for i, value := range values {
		out[i] = sessionFilterWire{value.Name, append([]string(nil), value.Values...)}
	}
	return out
}
func metadataToWire(values []SessionMetadata) []sessionMetadataWire {
	out := make([]sessionMetadataWire, len(values))
	for i, value := range values {
		out[i] = sessionMetadataWire{value.Name, value.Value}
	}
	return out
}
func stateToWire(value SessionState) *sessionStateWire {
	if len(value.CustomState) == 0 {
		return nil
	}
	return &sessionStateWire{CustomState: string(value.CustomState)}
}

func sessionSpaceFromWire(value *sessionSpaceWire) (SessionSpace, error) {
	if value == nil || value.ID == "" || value.Name == "" || value.Status == "" || value.CreateTime == "" || value.UpdateTime == "" {
		return SessionSpace{}, malformedSessionResponse("SessionSpace is missing required fields")
	}
	created, err := time.Parse(time.RFC3339Nano, value.CreateTime)
	if err != nil {
		return SessionSpace{}, malformedSessionResponse("SessionSpace CreateTime is invalid")
	}
	updated, err := time.Parse(time.RFC3339Nano, value.UpdateTime)
	if err != nil {
		return SessionSpace{}, malformedSessionResponse("SessionSpace UpdateTime is invalid")
	}
	return SessionSpace{ID: value.ID, Name: value.Name, Description: value.Description, Status: value.Status, Default: value.Default, CreatedAt: created, UpdatedAt: updated}, nil
}

func sessionFromWire(value *sessionWire) (Session, error) {
	if value == nil || value.SessionID == "" || value.SpaceID == "" || value.UserID == "" || value.EventCount == nil || *value.EventCount < 0 || value.CreateTime == "" || value.UpdateTime == "" {
		return Session{}, malformedSessionResponse("Session is missing required fields")
	}
	created, err := time.Parse(time.RFC3339Nano, value.CreateTime)
	if err != nil {
		return Session{}, malformedSessionResponse("Session CreateTime is invalid")
	}
	updated, err := time.Parse(time.RFC3339Nano, value.UpdateTime)
	if err != nil {
		return Session{}, malformedSessionResponse("Session UpdateTime is invalid")
	}
	out := Session{Ref: SessionRef{value.SpaceID, value.UserID, value.SessionID}, Title: value.Title, EventCount: *value.EventCount, CreatedAt: created, UpdatedAt: updated, Metadata: make([]SessionMetadata, len(value.Metadata))}
	for i, item := range value.Metadata {
		out.Metadata[i] = SessionMetadata{item.Name, item.Value}
	}
	if value.State != nil && value.State.CustomState != "" {
		raw, err := objectJSON("Session.CustomState", value.State.CustomState)
		if err != nil {
			return Session{}, err
		}
		out.State.CustomState = raw
	}
	return out, nil
}

func eventToWire(value Event) eventWire {
	out := eventWire{EventID: value.ID, InvocationID: value.InvocationID, Author: value.Author, Metadata: rawString(value.Metadata), Extensions: rawString(value.Extensions), ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage}
	if value.Content != nil {
		content := &eventContentWire{Role: value.Content.Role, Parts: make([]eventPartWire, len(value.Content.Parts))}
		for i, part := range value.Content.Parts {
			content.Parts[i] = eventPartWire{Text: part.Text, Thought: part.Thought, FunctionCall: rawString(part.FunctionCall), FunctionResponse: rawString(part.FunctionResponse)}
			if part.InlineData != nil {
				content.Parts[i].InlineData = &inlineDataWire{part.InlineData.MIMEType, part.InlineData.Data}
			}
		}
		out.Content = content
	}
	if value.Actions != nil {
		out.Actions = &eventActionsWire{StateDelta: rawString(value.Actions.StateDelta)}
	}
	return out
}

func eventFromWire(value *eventWire) (Event, error) {
	if value == nil || value.Timestamp == "" {
		return Event{}, malformedSessionResponse("Event is missing required Timestamp")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value.Timestamp)
	if err != nil {
		return Event{}, malformedSessionResponse("Event Timestamp is invalid")
	}
	out := Event{ID: value.EventID, InvocationID: value.InvocationID, Author: value.Author, ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, Timestamp: timestamp}
	if value.Metadata != "" {
		out.Metadata, err = optionalObjectJSON("Event.Metadata", value.Metadata)
		if err != nil {
			return Event{}, err
		}
	}
	if value.Extensions != "" {
		out.Extensions, err = optionalObjectJSON("Event.Extensions", value.Extensions)
		if err != nil {
			return Event{}, err
		}
	}
	if value.Actions != nil {
		action := &EventActions{}
		if value.Actions.StateDelta != "" {
			action.StateDelta, err = optionalObjectJSON("Event.StateDelta", value.Actions.StateDelta)
			if err != nil {
				return Event{}, err
			}
		}
		out.Actions = action
	}
	if value.Content != nil {
		content := &EventContent{Role: value.Content.Role, Parts: make([]EventPart, len(value.Content.Parts))}
		for i, part := range value.Content.Parts {
			content.Parts[i] = EventPart{Text: part.Text, Thought: part.Thought}
			if part.FunctionCall != "" {
				content.Parts[i].FunctionCall, err = optionalObjectJSON("Event.FunctionCall", part.FunctionCall)
				if err != nil {
					return Event{}, err
				}
			}
			if part.FunctionResponse != "" {
				content.Parts[i].FunctionResponse, err = optionalObjectJSON("Event.FunctionResponse", part.FunctionResponse)
				if err != nil {
					return Event{}, err
				}
			}
			if part.InlineData != nil {
				content.Parts[i].InlineData = &InlineData{part.InlineData.MimeType, part.InlineData.Data}
			}
		}
		out.Content = content
	}
	return out, nil
}

func objectJSON(field, value string) (json.RawMessage, error) {
	raw := json.RawMessage(value)
	var object map[string]json.RawMessage
	if !json.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, malformedSessionResponse(field + " JSON object is invalid")
	}
	return append(json.RawMessage(nil), raw...), nil
}

func optionalObjectJSON(field, value string) (json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace([]byte(value)), []byte("null")) {
		return nil, nil
	}
	return objectJSON(field, value)
}
func rawString(value json.RawMessage) string {
	if len(value) == 0 {
		return ""
	}
	return string(value)
}
func malformedSessionResponse(reason string) error {
	return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New(reason)}
}
