package agr

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSessionJSONBytes = 1 << 20

// SessionSpaceStatus preserves service-defined SessionSpace status values.
type SessionSpaceStatus string

const (
	// SessionSpaceActive is a usable SessionSpace.
	SessionSpaceActive SessionSpaceStatus = "Active"
	// SessionSpaceDeleting is being deleted.
	SessionSpaceDeleting SessionSpaceStatus = "Deleting"
)

// SessionTag is a Tencent Cloud resource tag.
type SessionTag struct {
	// Key is the tag key.
	Key string
	// Value is the tag value.
	Value string
}

// SessionFilter is one server-side Session or SessionSpace filter.
type SessionFilter struct {
	// Name selects a reviewed service field.
	Name string
	// Values are ORed within this filter.
	Values []string
}

// CreateSessionSpaceOptions creates one isolation boundary.
type CreateSessionSpaceOptions struct {
	// Name is the nonempty Space name.
	Name string
	// Description is optional explanatory text.
	Description string
	// Tags are attached during creation.
	Tags []SessionTag
}

// UpdateSessionSpaceOptions replaces the required name and optionally the description. A nil
// Description preserves it; an empty pointed-to value clears it.
type UpdateSessionSpaceOptions struct {
	// Name is the required replacement name.
	Name string
	// Description preserves omission versus clearing.
	Description *string
}

// SessionSpaceListOptions selects one offset page.
type SessionSpaceListOptions struct {
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
	// Filters are ANDed by the service.
	Filters []SessionFilter
}

// SessionSpaceInfo is an immutable service snapshot.
type SessionSpaceInfo struct {
	// ID is the service-generated Space identity.
	ID string
	// Name is the current Space name.
	Name string
	// Description is the current optional description.
	Description string
	// Status preserves known and future service values.
	Status SessionSpaceStatus
	// Default reports whether this is the protected default Space.
	Default bool
	// CreatedAt is the service creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the service update timestamp.
	UpdatedAt time.Time
	// RequestID is service correlation data.
	RequestID string
}

// SessionSpacePage is one service-reported offset page.
type SessionSpacePage struct {
	// Items contains Space snapshots.
	Items []SessionSpaceInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// SessionRef is the complete identity and isolation boundary of one Session.
// SessionID alone is never sufficient.
type SessionRef struct {
	// SpaceID identifies the isolation Space.
	SpaceID string
	// UserID identifies the user within the Space.
	UserID string
	// SessionID identifies the Session within the Space and User.
	SessionID string
}

// SessionMetadata is one ordered metadata entry.
type SessionMetadata struct {
	// Name is the unique metadata key.
	Name string
	// Value may be empty.
	Value string
}

// SessionState is a service snapshot. CustomState must be a JSON object.
type SessionState struct {
	// CustomState is an optional bounded JSON object.
	CustomState json.RawMessage
}

// CreateSessionOptions creates a Session. SessionID may be empty for service generation.
type CreateSessionOptions struct {
	// Ref supplies SpaceID and UserID; SessionID may be empty.
	Ref SessionRef
	// Title is optional display text.
	Title string
	// State is the initial Session snapshot.
	State SessionState
	// Metadata is the initial complete metadata set.
	Metadata []SessionMetadata
}

// UpdateSessionOptions replaces selected fields. Nil preserves a field; an empty pointed-to
// value clears it.
type UpdateSessionOptions struct {
	// Title preserves omission versus explicit clearing.
	Title *string
	// Metadata preserves omission versus replacing with an empty set.
	Metadata *[]SessionMetadata
}

// SessionListOptions selects one offset page within a required SessionSpace.
type SessionListOptions struct {
	// SpaceID is the required isolation boundary.
	SpaceID string
	// UserIDs select exact users.
	UserIDs []string
	// SessionIDs select exact Session IDs.
	SessionIDs []string
	// Filters select reviewed title or metadata fields.
	Filters []SessionFilter
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 20 and must not exceed 100.
	Limit int
}

// SessionInfo is an immutable service snapshot.
type SessionInfo struct {
	// Ref is the complete Session identity.
	Ref SessionRef
	// Title is the current title.
	Title string
	// State is the current service snapshot.
	State SessionState
	// Metadata is the current complete metadata set.
	Metadata []SessionMetadata
	// EventCount is the service-reported event count.
	EventCount int64
	// CreatedAt is the service creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the service update timestamp.
	UpdatedAt time.Time
	// RequestID is service correlation data.
	RequestID string
}

// SessionPage is one service-reported offset page.
type SessionPage struct {
	// Items contains Session snapshots.
	Items []SessionInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

// EventInlineData is inline Base64 content and its media type.
type EventInlineData struct {
	// MIMEType identifies the encoded content type.
	MIMEType string
	// Data is Base64-encoded inline data.
	Data string
}

// EventPart is one structured content part. Pointer fields preserve explicit empty text and
// false values.
type EventPart struct {
	// Text preserves absent versus explicit empty text.
	Text *string
	// Thought preserves absent versus explicit false.
	Thought *bool
	// FunctionCall is an optional bounded JSON object.
	FunctionCall json.RawMessage
	// FunctionResponse is an optional bounded JSON object.
	FunctionResponse json.RawMessage
	// InlineData is optional inline content.
	InlineData *EventInlineData
}

// EventContent is structured, potentially multimodal event content.
type EventContent struct {
	// Role is the content role.
	Role string
	// Parts is the ordered content sequence.
	Parts []EventPart
}

// EventActions carries a JSON-object state delta applied by the service.
type EventActions struct {
	// StateDelta is an optional bounded JSON object.
	StateDelta json.RawMessage
}

// EventInput is append-only event input. Timestamp is always generated by the service.
type EventInput struct {
	// ID is optional caller-supplied event identity, not an SDK idempotency key.
	ID string
	// InvocationID correlates related events.
	InvocationID string
	// Author selects the event author.
	Author string
	// Content is optional structured content.
	Content *EventContent
	// Actions carries an optional state delta.
	Actions *EventActions
	// Metadata is an optional bounded JSON object.
	Metadata json.RawMessage
	// Extensions is an optional bounded JSON object.
	Extensions json.RawMessage
	// ErrorCode is optional application error data.
	ErrorCode string
	// ErrorMessage is optional application error text.
	ErrorMessage string
}

// EventInfo is an immutable service event snapshot.
type EventInfo struct {
	EventInput
	// Timestamp is generated by the service.
	Timestamp time.Time
	// RequestID is service correlation data.
	RequestID string
}

// EventListOptions selects one offset page of events for an exact Session.
type EventListOptions struct {
	// Author selects one exact event author.
	Author string
	// AfterTimestamp returns events after this instant.
	AfterTimestamp *time.Time
	// Offset is the nonnegative page offset.
	Offset int
	// Limit defaults to 50 and must not exceed 200.
	Limit int
}

// EventPage is one service-reported offset page.
type EventPage struct {
	// Items contains Event snapshots.
	Items []EventInfo
	// TotalCount is the service count for the query.
	TotalCount int64
	// NextOffset is nil when no continuation is reported.
	NextOffset *int
	// RequestID is service correlation data.
	RequestID string
}

type sessionSpaceControlPlane interface {
	CreateSessionSpace(context.Context, CreateSessionSpaceOptions) (SessionSpaceInfo, error)
	GetSessionSpace(context.Context, string) (SessionSpaceInfo, error)
	ListSessionSpaces(context.Context, SessionSpaceListOptions) (SessionSpacePage, error)
	UpdateSessionSpace(context.Context, string, UpdateSessionSpaceOptions) (SessionSpaceInfo, error)
	DeleteSessionSpace(context.Context, string) error
}

// SessionSpaceManager manages Preview SessionSpace resources.
type SessionSpaceManager struct{ client *Client }

func (m *SessionSpaceManager) transport(operation string) (sessionSpaceControlPlane, error) {
	cp, ok := m.client.cfg.control.(sessionSpaceControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "SESSION_SPACE_CONTROL_UNAVAILABLE")
	}
	return cp, nil
}

// Create creates one SessionSpace.
func (m *SessionSpaceManager) Create(ctx context.Context, opts CreateSessionSpaceOptions) (SessionSpaceInfo, error) {
	opts.Tags = append([]SessionTag(nil), opts.Tags...)
	if strings.TrimSpace(opts.Name) == "" || utf8.RuneCountInString(opts.Name) > 128 {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Create", "NAME_INVALID")
	}
	if utf8.RuneCountInString(opts.Description) > 512 {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Create", "DESCRIPTION_TOO_LONG")
	}
	cp, err := m.transport("SessionSpaces.Create")
	if err != nil {
		return SessionSpaceInfo{}, err
	}
	return cp.CreateSessionSpace(ctx, opts)
}

// Get returns one exact SessionSpace.
func (m *SessionSpaceManager) Get(ctx context.Context, id string) (SessionSpaceInfo, error) {
	if strings.TrimSpace(id) == "" {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Get", "SPACE_ID_REQUIRED")
	}
	cp, err := m.transport("SessionSpaces.Get")
	if err != nil {
		return SessionSpaceInfo{}, err
	}
	return cp.GetSessionSpace(ctx, id)
}

// List returns one server-filtered offset page.
func (m *SessionSpaceManager) List(ctx context.Context, opts SessionSpaceListOptions) (SessionSpacePage, error) {
	opts.Filters = copySessionFilters(opts.Filters)
	if err := validateSessionSpaceList(opts); err != nil {
		return SessionSpacePage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport("SessionSpaces.List")
	if err != nil {
		return SessionSpacePage{}, err
	}
	return cp.ListSessionSpaces(ctx, opts)
}

// Update replaces the name and description of one exact SessionSpace.
func (m *SessionSpaceManager) Update(ctx context.Context, id string, opts UpdateSessionSpaceOptions) (SessionSpaceInfo, error) {
	if strings.TrimSpace(id) == "" {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Update", "SPACE_ID_REQUIRED")
	}
	if strings.TrimSpace(opts.Name) == "" || utf8.RuneCountInString(opts.Name) > 128 {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Update", "NAME_INVALID")
	}
	if opts.Description != nil && utf8.RuneCountInString(*opts.Description) > 512 {
		return SessionSpaceInfo{}, codeError(InvalidArgument, "SessionSpaces.Update", "DESCRIPTION_TOO_LONG")
	}
	cp, err := m.transport("SessionSpaces.Update")
	if err != nil {
		return SessionSpaceInfo{}, err
	}
	return cp.UpdateSessionSpace(ctx, id, opts)
}

// Delete removes one non-default SessionSpace after its Sessions are deleted.
func (m *SessionSpaceManager) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return codeError(InvalidArgument, "SessionSpaces.Delete", "SPACE_ID_REQUIRED")
	}
	cp, err := m.transport("SessionSpaces.Delete")
	if err != nil {
		return err
	}
	return cp.DeleteSessionSpace(ctx, id)
}

type sessionControlPlane interface {
	CreateSession(context.Context, CreateSessionOptions) (SessionInfo, error)
	GetSession(context.Context, SessionRef) (SessionInfo, error)
	ListSessions(context.Context, SessionListOptions) (SessionPage, error)
	UpdateSession(context.Context, SessionRef, UpdateSessionOptions) (SessionInfo, error)
	DeleteSession(context.Context, SessionRef) error
	AppendEvent(context.Context, SessionRef, EventInput) (EventInfo, error)
	ListEvents(context.Context, SessionRef, EventListOptions) (EventPage, error)
}

// SessionManager manages Preview Sessions and their Events.
type SessionManager struct{ client *Client }

func (m *SessionManager) transport(operation string) (sessionControlPlane, error) {
	cp, ok := m.client.cfg.control.(sessionControlPlane)
	if !ok {
		return nil, codeError(Unsupported, operation, "SESSION_CONTROL_UNAVAILABLE")
	}
	return cp, nil
}

// Create creates one Session in an exact Space and User isolation boundary.
func (m *SessionManager) Create(ctx context.Context, opts CreateSessionOptions) (SessionInfo, error) {
	opts = copyCreateSessionOptions(opts)
	if err := validateCreateSession(opts); err != nil {
		return SessionInfo{}, err
	}
	cp, err := m.transport("Sessions.Create")
	if err != nil {
		return SessionInfo{}, err
	}
	return cp.CreateSession(ctx, opts)
}

// Get returns one exact Session.
func (m *SessionManager) Get(ctx context.Context, ref SessionRef) (SessionInfo, error) {
	if err := validateSessionRef(ref, "Sessions.Get", true); err != nil {
		return SessionInfo{}, err
	}
	cp, err := m.transport("Sessions.Get")
	if err != nil {
		return SessionInfo{}, err
	}
	return cp.GetSession(ctx, ref)
}

// List returns one server-filtered offset page within a SessionSpace.
func (m *SessionManager) List(ctx context.Context, opts SessionListOptions) (SessionPage, error) {
	opts = copySessionListOptions(opts)
	if err := validateSessionList(opts); err != nil {
		return SessionPage{}, err
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	cp, err := m.transport("Sessions.List")
	if err != nil {
		return SessionPage{}, err
	}
	return cp.ListSessions(ctx, opts)
}

// Update replaces selected mutable fields on one exact Session.
func (m *SessionManager) Update(ctx context.Context, ref SessionRef, opts UpdateSessionOptions) (SessionInfo, error) {
	if err := validateSessionRef(ref, "Sessions.Update", true); err != nil {
		return SessionInfo{}, err
	}
	opts = copyUpdateSessionOptions(opts)
	if opts.Title == nil && opts.Metadata == nil {
		return SessionInfo{}, codeError(InvalidArgument, "Sessions.Update", "UPDATE_REQUIRED")
	}
	if opts.Title != nil && utf8.RuneCountInString(*opts.Title) > 255 {
		return SessionInfo{}, codeError(InvalidArgument, "Sessions.Update", "TITLE_TOO_LONG")
	}
	if opts.Metadata != nil {
		if err := validateSessionMetadata(*opts.Metadata, "Sessions.Update"); err != nil {
			return SessionInfo{}, err
		}
	}
	cp, err := m.transport("Sessions.Update")
	if err != nil {
		return SessionInfo{}, err
	}
	return cp.UpdateSession(ctx, ref, opts)
}

// Delete removes one exact Session.
func (m *SessionManager) Delete(ctx context.Context, ref SessionRef) error {
	if err := validateSessionRef(ref, "Sessions.Delete", true); err != nil {
		return err
	}
	cp, err := m.transport("Sessions.Delete")
	if err != nil {
		return err
	}
	return cp.DeleteSession(ctx, ref)
}

// AppendEvent appends one event without automatic retry. Cancellation does not prove that the
// service rejected the event and an Event ID is not claimed as an idempotency key.
func (m *SessionManager) AppendEvent(ctx context.Context, ref SessionRef, event EventInput) (EventInfo, error) {
	if err := validateSessionRef(ref, "Sessions.AppendEvent", true); err != nil {
		return EventInfo{}, err
	}
	event = copyEventInput(event)
	if err := validateEvent(event); err != nil {
		return EventInfo{}, err
	}
	cp, err := m.transport("Sessions.AppendEvent")
	if err != nil {
		return EventInfo{}, err
	}
	return cp.AppendEvent(ctx, ref, event)
}

// ListEvents returns one offset page for one exact Session.
func (m *SessionManager) ListEvents(ctx context.Context, ref SessionRef, opts EventListOptions) (EventPage, error) {
	if err := validateSessionRef(ref, "Sessions.ListEvents", true); err != nil {
		return EventPage{}, err
	}
	if opts.Offset < 0 || opts.Limit < 0 || opts.Limit > 200 {
		return EventPage{}, codeError(InvalidArgument, "Sessions.ListEvents", "PAGINATION_INVALID")
	}
	if utf8.RuneCountInString(opts.Author) > 128 {
		return EventPage{}, codeError(InvalidArgument, "Sessions.ListEvents", "AUTHOR_TOO_LONG")
	}
	if opts.AfterTimestamp != nil {
		value := *opts.AfterTimestamp
		opts.AfterTimestamp = &value
		if value.IsZero() {
			return EventPage{}, codeError(InvalidArgument, "Sessions.ListEvents", "AFTER_TIMESTAMP_INVALID")
		}
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	cp, err := m.transport("Sessions.ListEvents")
	if err != nil {
		return EventPage{}, err
	}
	return cp.ListEvents(ctx, ref, opts)
}

func validateSessionSpaceList(opts SessionSpaceListOptions) error {
	if opts.Offset < 0 || opts.Limit < 0 || opts.Limit > 100 || len(opts.Filters) > 3 {
		return codeError(InvalidArgument, "SessionSpaces.List", "PAGINATION_OR_FILTER_INVALID")
	}
	allowed := map[string]bool{"space-id": true, "name": true, "name-like": true, "description-like": true}
	seen := map[string]bool{}
	for _, filter := range opts.Filters {
		if !allowed[filter.Name] || seen[filter.Name] || len(filter.Values) == 0 || len(filter.Values) > 100 {
			return codeError(InvalidArgument, "SessionSpaces.List", "FILTER_INVALID")
		}
		seen[filter.Name] = true
		for _, value := range filter.Values {
			if value == "" || utf8.RuneCountInString(value) > 128 {
				return codeError(InvalidArgument, "SessionSpaces.List", "FILTER_INVALID")
			}
		}
	}
	if seen["name"] && seen["name-like"] {
		return codeError(InvalidArgument, "SessionSpaces.List", "FILTER_CONFLICT")
	}
	return nil
}

func validateCreateSession(opts CreateSessionOptions) error {
	if err := validateSessionRef(opts.Ref, "Sessions.Create", false); err != nil {
		return err
	}
	if utf8.RuneCountInString(opts.Title) > 256 {
		return codeError(InvalidArgument, "Sessions.Create", "TITLE_TOO_LONG")
	}
	if err := validateJSONObject(opts.State.CustomState, maxSessionJSONBytes, "Sessions.Create", "CUSTOM_STATE_INVALID"); err != nil {
		return err
	}
	return validateSessionMetadata(opts.Metadata, "Sessions.Create")
}

func validateSessionRef(ref SessionRef, operation string, sessionRequired bool) error {
	if strings.TrimSpace(ref.SpaceID) == "" {
		return codeError(InvalidArgument, operation, "SPACE_ID_REQUIRED")
	}
	if strings.TrimSpace(ref.UserID) == "" {
		return codeError(InvalidArgument, operation, "USER_ID_REQUIRED")
	}
	if sessionRequired && strings.TrimSpace(ref.SessionID) == "" {
		return codeError(InvalidArgument, operation, "SESSION_ID_REQUIRED")
	}
	return nil
}

func validateSessionMetadata(values []SessionMetadata, operation string) error {
	if len(values) > 64 {
		return codeError(InvalidArgument, operation, "METADATA_TOO_MANY")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if value.Name == "" || len(value.Name) > 253 || len(value.Value) > 1024 || seen[value.Name] {
			return codeError(InvalidArgument, operation, "METADATA_INVALID")
		}
		seen[value.Name] = true
	}
	wire, err := json.Marshal(values)
	if err != nil || len(wire) > 64<<10 {
		return codeError(InvalidArgument, operation, "METADATA_TOO_LARGE")
	}
	return nil
}

func validateSessionList(opts SessionListOptions) error {
	if strings.TrimSpace(opts.SpaceID) == "" {
		return codeError(InvalidArgument, "Sessions.List", "SPACE_ID_REQUIRED")
	}
	if opts.Offset < 0 || opts.Limit < 0 || opts.Limit > 100 || len(opts.UserIDs) > 100 || len(opts.SessionIDs) > 100 || len(opts.Filters) > 10 {
		return codeError(InvalidArgument, "Sessions.List", "PAGINATION_OR_FILTER_INVALID")
	}
	for _, value := range append(append([]string(nil), opts.UserIDs...), opts.SessionIDs...) {
		if strings.TrimSpace(value) == "" {
			return codeError(InvalidArgument, "Sessions.List", "ID_INVALID")
		}
	}
	seen := map[string]bool{}
	for _, filter := range opts.Filters {
		validName := filter.Name == "title" || filter.Name == "title-like" || strings.HasPrefix(filter.Name, "metadata:") && strings.TrimPrefix(filter.Name, "metadata:") != ""
		if !validName || seen[filter.Name] || len(filter.Values) == 0 || len(filter.Values) > 100 {
			return codeError(InvalidArgument, "Sessions.List", "FILTER_INVALID")
		}
		seen[filter.Name] = true
		for _, value := range filter.Values {
			if strings.TrimSpace(value) == "" {
				return codeError(InvalidArgument, "Sessions.List", "FILTER_INVALID")
			}
		}
	}
	if seen["title"] && seen["title-like"] {
		return codeError(InvalidArgument, "Sessions.List", "FILTER_CONFLICT")
	}
	return nil
}

func validateEvent(value EventInput) error {
	if utf8.RuneCountInString(value.InvocationID) > 128 || utf8.RuneCountInString(value.Author) > 128 || utf8.RuneCountInString(value.ErrorCode) > 128 || utf8.RuneCountInString(value.ErrorMessage) > 2048 {
		return codeError(InvalidArgument, "Sessions.AppendEvent", "EVENT_FIELD_TOO_LONG")
	}
	if err := validateJSONObject(value.Metadata, maxSessionJSONBytes, "Sessions.AppendEvent", "METADATA_JSON_INVALID"); err != nil {
		return err
	}
	if err := validateJSONObject(value.Extensions, 8192, "Sessions.AppendEvent", "EXTENSIONS_INVALID"); err != nil {
		return err
	}
	if value.Actions != nil {
		if err := validateJSONObject(value.Actions.StateDelta, 8192, "Sessions.AppendEvent", "STATE_DELTA_INVALID"); err != nil {
			return err
		}
	}
	if value.Content != nil {
		if utf8.RuneCountInString(value.Content.Role) > 64 {
			return codeError(InvalidArgument, "Sessions.AppendEvent", "ROLE_TOO_LONG")
		}
		for _, part := range value.Content.Parts {
			if part.Text != nil && utf8.RuneCountInString(*part.Text) > 8192 {
				return codeError(InvalidArgument, "Sessions.AppendEvent", "TEXT_TOO_LONG")
			}
			if err := validateJSONObject(part.FunctionCall, 8192, "Sessions.AppendEvent", "FUNCTION_CALL_INVALID"); err != nil {
				return err
			}
			if err := validateJSONObject(part.FunctionResponse, 8192, "Sessions.AppendEvent", "FUNCTION_RESPONSE_INVALID"); err != nil {
				return err
			}
			if part.InlineData != nil && (utf8.RuneCountInString(part.InlineData.MIMEType) > 128 || utf8.RuneCountInString(part.InlineData.Data) > 8192) {
				return codeError(InvalidArgument, "Sessions.AppendEvent", "INLINE_DATA_INVALID")
			}
		}
	}
	return nil
}

func validateJSONObject(value json.RawMessage, limit int, operation, reason string) error {
	if len(value) == 0 {
		return nil
	}
	if len(value) > limit || !json.Valid(value) {
		return codeError(InvalidArgument, operation, reason)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil || object == nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return codeError(InvalidArgument, operation, reason)
	}
	return nil
}

func copySessionFilters(values []SessionFilter) []SessionFilter {
	out := append([]SessionFilter(nil), values...)
	for i := range out {
		out[i].Values = append([]string(nil), out[i].Values...)
	}
	return out
}
func copyCreateSessionOptions(value CreateSessionOptions) CreateSessionOptions {
	value.State.CustomState = append(json.RawMessage(nil), value.State.CustomState...)
	value.Metadata = append([]SessionMetadata(nil), value.Metadata...)
	return value
}
func copySessionListOptions(value SessionListOptions) SessionListOptions {
	value.UserIDs = append([]string(nil), value.UserIDs...)
	value.SessionIDs = append([]string(nil), value.SessionIDs...)
	value.Filters = copySessionFilters(value.Filters)
	return value
}
func copyUpdateSessionOptions(value UpdateSessionOptions) UpdateSessionOptions {
	if value.Title != nil {
		title := *value.Title
		value.Title = &title
	}
	if value.Metadata != nil {
		metadata := append([]SessionMetadata(nil), (*value.Metadata)...)
		value.Metadata = &metadata
	}
	return value
}
func copyEventInput(value EventInput) EventInput {
	value.Metadata = append(json.RawMessage(nil), value.Metadata...)
	value.Extensions = append(json.RawMessage(nil), value.Extensions...)
	if value.Actions != nil {
		action := *value.Actions
		action.StateDelta = append(json.RawMessage(nil), action.StateDelta...)
		value.Actions = &action
	}
	if value.Content != nil {
		content := *value.Content
		content.Parts = append([]EventPart(nil), content.Parts...)
		for i := range content.Parts {
			part := &content.Parts[i]
			if part.Text != nil {
				text := *part.Text
				part.Text = &text
			}
			if part.Thought != nil {
				thought := *part.Thought
				part.Thought = &thought
			}
			part.FunctionCall = append(json.RawMessage(nil), part.FunctionCall...)
			part.FunctionResponse = append(json.RawMessage(nil), part.FunctionResponse...)
			if part.InlineData != nil {
				data := *part.InlineData
				part.InlineData = &data
			}
		}
		value.Content = &content
	}
	return value
}
