package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"
)

const maxRegistryJSONBytes = 1 << 20

// RegistryTag is one Cloud tag.
type RegistryTag struct{ Key, Value string }

// RegistryFilter is one reviewed Registry filter.
type RegistryFilter struct {
	Name   string
	Values []string
}

// Registry is an immutable control-plane Registry value.
type Registry struct {
	ID, Name, Description, ApprovalMode, Region, Status string
	CreatedAt, UpdatedAt                                time.Time
	RecordCount                                         int64
	PublishedRecordCount, AppID                         *int64
	CreatorUIN, CreatorSubUIN                           string
	Tags                                                []RegistryTag
}

type RegistryResult struct {
	Registry  Registry
	RequestID string
}
type RegistryPage struct {
	Items      []Registry
	TotalCount int64
	RequestID  string
}
type CreateRegistryRequest struct {
	Name, Description, ApprovalMode string
	Tags                            []RegistryTag
}
type UpdateRegistryRequest struct{ Description string }
type RegistryListRequest struct {
	Offset, Limit int
	Filters       []RegistryFilter
}

type RegistrySource struct {
	Type        string
	Descriptors json.RawMessage
	EndpointURL string
}
type RegistrySkillSource struct{ Type, SkillMD string }

type RegistryRecord struct {
	ID, RegistryID, Name, Description, DescriptorType, Status string
	AppID                                                     int64
	CreatorUIN, CreatorSubUIN                                 string
	CreatedAt, UpdatedAt                                      time.Time
	VersionCount                                              *int64
	Labels                                                    []string
}
type RegistryApprovalAction struct {
	ID, Type, ActorType, ActorUIN, ActorSubUIN, Comment, RequestID string
	CreatedAt                                                      time.Time
}
type RegistryRecordVersion struct {
	ID, RecordID                 string
	Revision, AppID              int64
	Status, ApprovalMode         string
	CreatorUIN, CreatorSubUIN    string
	CreatedAt, UpdatedAt         time.Time
	Name                         string
	Descriptors, SourceConfig    json.RawMessage
	SourceType                   string
	ContentStatus, ContentSHA256 string
	ContentSizeBytes             *int64
	ConfigSHA256, ChangeLog      string
	ContentReadyAt               time.Time
	ApprovalActions              []RegistryApprovalAction
	Labels                       []string
}
type RegistryRecordResult struct {
	Record                    RegistryRecord
	Version                   *RegistryRecordVersion
	ResolvedBy, ResolvedLabel string
	RequestID                 string
}
type RegistryRecordPage struct {
	Items      []RegistryRecord
	TotalCount int64
	RequestID  string
}
type RegistryRecordVersionPage struct {
	Items      []RegistryRecordVersion
	TotalCount int64
	RequestID  string
}
type RegistrySelector struct{ VersionID, Label string }
type CreateRegistryRecordRequest struct {
	Name, Description, VersionName, DescriptorType string
	MCPSource, AgentSource                         *RegistrySource
	SkillSource                                    *RegistrySkillSource
	CustomDescriptors                              json.RawMessage
}
type RegistryLabelMutation struct {
	Operation, Name string
	VersionID       *string
	Reason          string
}
type UpdateRegistryRecordRequest struct {
	Description            *string
	VersionName, ChangeLog string
	MCPSource, AgentSource *RegistrySource
	SkillSource            *RegistrySkillSource
	CustomDescriptors      json.RawMessage
	LabelMutations         []RegistryLabelMutation
}
type DeleteRegistryRecordRequest struct {
	VersionID *string
	Reason    string
}
type RegistryRecordListRequest struct {
	Offset, Limit int
	Filters       []RegistryFilter
}
type RegistryAuditLog struct {
	ID, RegistryID, RecordID, VersionID, Actor, Action string
	Details                                            json.RawMessage
	CreatedAt                                          time.Time
}
type RegistryAuditLogRequest struct {
	RecordID, VersionID, Action, Actor string
	Start, End                         *time.Time
	Offset, Limit                      int
}
type RegistryAuditLogPage struct {
	Items      []RegistryAuditLog
	TotalCount int64
	RequestID  string
}
type RegistryPreview struct {
	Result            json.RawMessage
	ResolvedVersionID string
	RequestID         string
}
type RegistrySyncResult struct {
	Status, ResolvedVersionID string
	CreatedVersion            *RegistryRecordVersion
	Record                    *RegistryRecord
	LastSyncAt                time.Time
	ErrorCode, ErrorMessage   string
	RequestID                 string
}

func (s *Service) CreateRegistry(ctx context.Context, request CreateRegistryRequest) (RegistryResult, error) {
	out, err := s.ags.CreateRegistry(ctx, request)
	return out, cloudError(err, "CreateRegistry")
}
func (s *Service) GetRegistry(ctx context.Context, id string) (RegistryResult, error) {
	out, err := s.ags.GetRegistry(ctx, id)
	return out, cloudError(err, "DescribeRegistry")
}
func (s *Service) ListRegistries(ctx context.Context, request RegistryListRequest) (RegistryPage, error) {
	out, err := s.ags.ListRegistries(ctx, request)
	return out, cloudError(err, "DescribeRegistryList")
}
func (s *Service) UpdateRegistry(ctx context.Context, id string, request UpdateRegistryRequest) (RegistryResult, error) {
	out, err := s.ags.UpdateRegistry(ctx, id, request)
	return out, cloudError(err, "UpdateRegistry")
}
func (s *Service) DeleteRegistry(ctx context.Context, id string) error {
	return cloudError(s.ags.DeleteRegistry(ctx, id), "DeleteRegistry")
}
func (s *Service) CreateRegistryRecord(ctx context.Context, registryID string, request CreateRegistryRecordRequest) (RegistryRecordResult, error) {
	out, err := s.ags.CreateRegistryRecord(ctx, registryID, request)
	return out, cloudError(err, "CreateRegistryRecord")
}
func (s *Service) GetRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector) (RegistryRecordResult, error) {
	out, err := s.ags.GetRegistryRecord(ctx, registryID, recordID, selector)
	return out, cloudError(err, "DescribeRegistryRecord")
}
func (s *Service) ListRegistryRecords(ctx context.Context, registryID string, request RegistryRecordListRequest) (RegistryRecordPage, error) {
	out, err := s.ags.ListRegistryRecords(ctx, registryID, request)
	return out, cloudError(err, "DescribeRegistryRecordList")
}
func (s *Service) UpdateRegistryRecord(ctx context.Context, registryID, recordID string, request UpdateRegistryRecordRequest) (RegistryRecordResult, error) {
	out, err := s.ags.UpdateRegistryRecord(ctx, registryID, recordID, request)
	return out, cloudError(err, "UpdateRegistryRecord")
}
func (s *Service) DeleteRegistryRecord(ctx context.Context, registryID, recordID string, request DeleteRegistryRecordRequest) error {
	return cloudError(s.ags.DeleteRegistryRecord(ctx, registryID, recordID, request), "DeleteRegistryRecord")
}
func (s *Service) ListRegistryRecordVersions(ctx context.Context, registryID, recordID string, request RegistryRecordListRequest) (RegistryRecordVersionPage, error) {
	out, err := s.ags.ListRegistryRecordVersions(ctx, registryID, recordID, request)
	return out, cloudError(err, "DescribeRegistryRecordVersionList")
}
func (s *Service) DecideRegistryRecord(ctx context.Context, decision, registryID, recordID, versionID, comment string) (RegistryRecordVersion, string, error) {
	out, requestID, err := s.ags.DecideRegistryRecord(ctx, decision, registryID, recordID, versionID, comment)
	return out, requestID, cloudError(err, decision+"RegistryRecord")
}
func (s *Service) PreviewRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector) (RegistryPreview, error) {
	out, err := s.ags.PreviewRegistryRecord(ctx, registryID, recordID, selector)
	return out, cloudError(err, "PreviewRegistryRecord")
}
func (s *Service) SyncRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector, changeLog string) (RegistrySyncResult, error) {
	out, err := s.ags.SyncRegistryRecord(ctx, registryID, recordID, selector, changeLog)
	return out, cloudError(err, "SyncRegistryRecord")
}
func (s *Service) ListRegistryAuditLogs(ctx context.Context, registryID string, request RegistryAuditLogRequest) (RegistryAuditLogPage, error) {
	out, err := s.ags.ListRegistryAuditLogs(ctx, registryID, request)
	return out, cloudError(err, "DescribeRegistryAuditLogList")
}

type registryTagWire struct{ Key, Value string }
type registryFilterWire struct {
	Name   string
	Values []string
}
type registryWire struct {
	RegistryID, Name, Description, ApprovalMode, Region, Status *string
	CreateTime, UpdateTime                                      *string
	RecordCount                                                 *int64
	PublishedRecordCount, AppID                                 *int64
	CreatorUIN, CreatorSubAccountUIN                            string
	Tags                                                        json.RawMessage
}
type registryRecordWire struct {
	RecordID, RegistryID, Name, Description, DescriptorType, LifecycleStatus *string
	AppID                                                                    *int64
	CreatorUIN, CreatorSubAccountUIN                                         *string
	CreateTime, UpdateTime                                                   *string
	VersionCount                                                             *int64
	LabelSet                                                                 []string
}
type registryApprovalWire struct {
	ActionID, ActionType, ActorType, ActorUIN, ActorSubAccountUIN, Comment, CreateTime, RequestID string
}
type registryVersionWire struct {
	VersionID, RecordID, Status, ApprovalMode, CreatorUIN, CreatorSubAccountUIN string
	CreateTime, UpdateTime, VersionName, Descriptors, SourceType, SourceConfig  string
	ContentStatus, ContentSHA256, ConfigSHA256, ContentReadyTime, ChangeLog     string
	Revision, AppID                                                             int64
	ContentSizeBytes                                                            *int64
	ApprovalActions                                                             []registryApprovalWire
	LabelSet                                                                    []string
}
type registrySourceWire struct {
	Type        string
	Descriptors string `json:",omitempty"`
	EndpointURL string `json:",omitempty"`
}
type registrySkillSourceWire struct {
	Type    string
	SkillMD string `json:"SkillMd,omitempty"`
}
type registryLabelMutationWire struct {
	Operation string
	Name      string
	VersionID *string `json:"VersionId,omitempty"`
	Reason    string  `json:",omitempty"`
}

func (a *AGS) CreateRegistry(ctx context.Context, in CreateRegistryRequest) (RegistryResult, error) {
	req := struct {
		Name         string
		Description  string            `json:",omitempty"`
		ApprovalMode string            `json:",omitempty"`
		Tags         []registryTagWire `json:",omitempty"`
	}{Name: in.Name, Description: in.Description, ApprovalMode: in.ApprovalMode, Tags: registryTagsToWire(in.Tags)}
	var response struct {
		RegistryID string `json:"RegistryId"`
		Registry   *registryWire
		RequestID  string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "CreateRegistry", req, &response); err != nil {
		return RegistryResult{}, err
	}
	value, err := registryFromWire(response.Registry)
	if err != nil || response.RegistryID == "" || response.RequestID == "" {
		if err == nil {
			err = malformedRegistryResponse("CreateRegistry response is malformed")
		}
		return RegistryResult{}, err
	}
	return RegistryResult{Registry: value, RequestID: response.RequestID}, nil
}

func (a *AGS) GetRegistry(ctx context.Context, id string) (RegistryResult, error) {
	var response struct {
		Registry  *registryWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeRegistry", struct {
		RegistryID string `json:"RegistryId"`
	}{id}, &response); err != nil {
		return RegistryResult{}, err
	}
	value, err := registryFromWire(response.Registry)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedRegistryResponse("DescribeRegistry response has no RequestId")
		}
		return RegistryResult{}, err
	}
	return RegistryResult{Registry: value, RequestID: response.RequestID}, nil
}

func (a *AGS) ListRegistries(ctx context.Context, in RegistryListRequest) (RegistryPage, error) {
	req := struct {
		Offset, Limit int
		Filters       []registryFilterWire `json:",omitempty"`
	}{in.Offset, in.Limit, registryFiltersToWire(in.Filters)}
	var response struct {
		RegistrySet json.RawMessage
		TotalCount  *int64
		RequestID   string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeRegistryList", req, &response); err != nil {
		return RegistryPage{}, err
	}
	var values []*registryWire
	if invalidCollection(response.RegistrySet) || json.Unmarshal(response.RegistrySet, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return RegistryPage{}, malformedRegistryResponse("DescribeRegistryList response is malformed")
	}
	out := RegistryPage{Items: make([]Registry, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, item := range values {
		value, err := registryFromWire(item)
		if err != nil {
			return RegistryPage{}, err
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

func (a *AGS) UpdateRegistry(ctx context.Context, id string, in UpdateRegistryRequest) (RegistryResult, error) {
	req := struct {
		RegistryID  string `json:"RegistryId"`
		Description string
	}{id, in.Description}
	var response struct {
		Registry  *registryWire
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "UpdateRegistry", req, &response); err != nil {
		return RegistryResult{}, err
	}
	value, err := registryFromWire(response.Registry)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedRegistryResponse("UpdateRegistry response has no RequestId")
		}
		return RegistryResult{}, err
	}
	return RegistryResult{Registry: value, RequestID: response.RequestID}, nil
}

func (a *AGS) DeleteRegistry(ctx context.Context, id string) error {
	var response struct {
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DeleteRegistry", struct {
		RegistryID string `json:"RegistryId"`
	}{id}, &response); err != nil {
		return err
	}
	if response.RequestID == "" {
		return malformedRegistryResponse("DeleteRegistry response has no RequestId")
	}
	return nil
}

func (a *AGS) CreateRegistryRecord(ctx context.Context, registryID string, in CreateRegistryRecordRequest) (RegistryRecordResult, error) {
	req := registryRecordMutationWire{RegistryID: registryID, Name: in.Name, Description: &in.Description, VersionName: in.VersionName, DescriptorType: in.DescriptorType, MCPSource: registrySourceToWire(in.MCPSource), AgentSource: registrySourceToWire(in.AgentSource), SkillSource: registrySkillToWire(in.SkillSource), CustomDescriptors: rawString(in.CustomDescriptors)}
	var response registryRecordResponseWire
	if err := a.commonCall(ctx, "CreateRegistryRecord", req, &response); err != nil {
		return RegistryRecordResult{}, err
	}
	return registryRecordResultFromWire(response, true)
}

func (a *AGS) GetRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector) (RegistryRecordResult, error) {
	req := struct {
		RegistryID string `json:"RegistryId"`
		RecordID   string `json:"RecordId"`
		VersionID  string `json:"VersionId,omitempty"`
		Label      string `json:",omitempty"`
	}{RegistryID: registryID, RecordID: recordID}
	if selector != nil {
		req.VersionID, req.Label = selector.VersionID, selector.Label
	}
	var response registryRecordResponseWire
	if err := a.commonCall(ctx, "DescribeRegistryRecord", req, &response); err != nil {
		return RegistryRecordResult{}, err
	}
	return registryRecordResultFromWire(response, true)
}

func (a *AGS) ListRegistryRecords(ctx context.Context, registryID string, in RegistryRecordListRequest) (RegistryRecordPage, error) {
	req := struct {
		RegistryID    string `json:"RegistryId"`
		Offset, Limit int
		Filters       []registryFilterWire `json:",omitempty"`
	}{registryID, in.Offset, in.Limit, registryFiltersToWire(in.Filters)}
	var response struct {
		RecordSet  json.RawMessage
		TotalCount *int64
		RequestID  string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeRegistryRecordList", req, &response); err != nil {
		return RegistryRecordPage{}, err
	}
	var values []*registryRecordWire
	if invalidCollection(response.RecordSet) || json.Unmarshal(response.RecordSet, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return RegistryRecordPage{}, malformedRegistryResponse("DescribeRegistryRecordList response is malformed")
	}
	out := RegistryRecordPage{Items: make([]RegistryRecord, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, item := range values {
		value, err := registryRecordFromWire(item)
		if err != nil {
			return RegistryRecordPage{}, err
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

func (a *AGS) UpdateRegistryRecord(ctx context.Context, registryID, recordID string, in UpdateRegistryRecordRequest) (RegistryRecordResult, error) {
	req := registryRecordMutationWire{RegistryID: registryID, RecordID: recordID, Description: in.Description, VersionName: in.VersionName, ChangeLog: in.ChangeLog, MCPSource: registrySourceToWire(in.MCPSource), AgentSource: registrySourceToWire(in.AgentSource), SkillSource: registrySkillToWire(in.SkillSource), CustomDescriptors: rawString(in.CustomDescriptors), LabelMutations: registryLabelsToWire(in.LabelMutations)}
	var response registryRecordResponseWire
	if err := a.commonCall(ctx, "UpdateRegistryRecord", req, &response); err != nil {
		return RegistryRecordResult{}, err
	}
	return registryRecordResultFromWire(response, false)
}

func (a *AGS) DeleteRegistryRecord(ctx context.Context, registryID, recordID string, in DeleteRegistryRecordRequest) error {
	req := struct {
		RegistryID string  `json:"RegistryId"`
		RecordID   string  `json:"RecordId"`
		VersionID  *string `json:"VersionId,omitempty"`
		Reason     string  `json:",omitempty"`
	}{registryID, recordID, in.VersionID, in.Reason}
	var response struct {
		RequestID string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DeleteRegistryRecord", req, &response); err != nil {
		return err
	}
	if response.RequestID == "" {
		return malformedRegistryResponse("DeleteRegistryRecord response has no RequestId")
	}
	return nil
}

func (a *AGS) ListRegistryRecordVersions(ctx context.Context, registryID, recordID string, in RegistryRecordListRequest) (RegistryRecordVersionPage, error) {
	req := struct {
		RegistryID    string `json:"RegistryId"`
		RecordID      string `json:"RecordId"`
		Offset, Limit int
		Filters       []registryFilterWire `json:",omitempty"`
	}{registryID, recordID, in.Offset, in.Limit, registryFiltersToWire(in.Filters)}
	var response struct {
		VersionSet json.RawMessage
		TotalCount *int64
		RequestID  string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeRegistryRecordVersionList", req, &response); err != nil {
		return RegistryRecordVersionPage{}, err
	}
	var values []*registryVersionWire
	if invalidCollection(response.VersionSet) || json.Unmarshal(response.VersionSet, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return RegistryRecordVersionPage{}, malformedRegistryResponse("DescribeRegistryRecordVersionList response is malformed")
	}
	out := RegistryRecordVersionPage{Items: make([]RegistryRecordVersion, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, item := range values {
		value, err := registryVersionFromWire(item)
		if err != nil {
			return RegistryRecordVersionPage{}, err
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

func (a *AGS) DecideRegistryRecord(ctx context.Context, decision, registryID, recordID, versionID, comment string) (RegistryRecordVersion, string, error) {
	action := map[string]string{"approve": "ApproveRegistryRecord", "reject": "RejectRegistryRecord", "cancel": "CancelRegistryRecord"}[decision]
	if action == "" {
		return RegistryRecordVersion{}, "", malformedRegistryResponse("unknown Registry decision")
	}
	req := struct {
		RegistryID string `json:"RegistryId"`
		RecordID   string `json:"RecordId"`
		VersionID  string `json:"VersionId"`
		Comment    string
	}{registryID, recordID, versionID, comment}
	var response struct {
		Version   *registryVersionWire
		RequestID string `json:"RequestId"`
	}
	var err error
	switch decision {
	case "approve":
		err = a.commonCall(ctx, "ApproveRegistryRecord", req, &response)
	case "reject":
		err = a.commonCall(ctx, "RejectRegistryRecord", req, &response)
	case "cancel":
		err = a.commonCall(ctx, "CancelRegistryRecord", req, &response)
	}
	if err != nil {
		return RegistryRecordVersion{}, "", err
	}
	value, err := registryVersionFromWire(response.Version)
	if err != nil || response.RequestID == "" {
		if err == nil {
			err = malformedRegistryResponse(action + " response has no RequestId")
		}
		return RegistryRecordVersion{}, "", err
	}
	return value, response.RequestID, nil
}

func (a *AGS) PreviewRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector) (RegistryPreview, error) {
	req := registrySelectorRequest(registryID, recordID, selector)
	var response struct {
		PreviewResult, ResolvedVersionID string
		RequestID                        string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "PreviewRegistryRecord", req, &response); err != nil {
		return RegistryPreview{}, err
	}
	result, err := registryJSONObject("PreviewResult", response.PreviewResult)
	if err != nil || response.ResolvedVersionID == "" || response.RequestID == "" {
		if err == nil {
			err = malformedRegistryResponse("PreviewRegistryRecord response is malformed")
		}
		return RegistryPreview{}, err
	}
	return RegistryPreview{Result: result, ResolvedVersionID: response.ResolvedVersionID, RequestID: response.RequestID}, nil
}

func (a *AGS) SyncRegistryRecord(ctx context.Context, registryID, recordID string, selector *RegistrySelector, changeLog string) (RegistrySyncResult, error) {
	req := registrySelectorRequest(registryID, recordID, selector)
	req.ChangeLog = changeLog
	var response struct {
		SyncStatus, ResolvedVersionID, LastSyncTime, ErrorCode, ErrorMessage string
		CreatedVersion                                                       *registryVersionWire
		Record                                                               *registryRecordWire
		RequestID                                                            string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "SyncRegistryRecord", req, &response); err != nil {
		return RegistrySyncResult{}, err
	}
	if response.SyncStatus == "" || response.ResolvedVersionID == "" || response.RequestID == "" {
		return RegistrySyncResult{}, malformedRegistryResponse("SyncRegistryRecord response is malformed")
	}
	out := RegistrySyncResult{Status: response.SyncStatus, ResolvedVersionID: response.ResolvedVersionID, ErrorCode: response.ErrorCode, ErrorMessage: response.ErrorMessage, RequestID: response.RequestID}
	var err error
	if response.LastSyncTime != "" {
		out.LastSyncAt, err = time.Parse(time.RFC3339Nano, response.LastSyncTime)
		if err != nil {
			return RegistrySyncResult{}, malformedRegistryResponse("Sync LastSyncTime is invalid")
		}
	}
	if response.CreatedVersion != nil {
		value, e := registryVersionFromWire(response.CreatedVersion)
		if e != nil {
			return RegistrySyncResult{}, e
		}
		out.CreatedVersion = &value
	}
	if response.Record != nil {
		value, e := registryRecordFromWire(response.Record)
		if e != nil {
			return RegistrySyncResult{}, e
		}
		out.Record = &value
	}
	return out, nil
}

func (a *AGS) ListRegistryAuditLogs(ctx context.Context, registryID string, in RegistryAuditLogRequest) (RegistryAuditLogPage, error) {
	req := struct {
		RegistryID   string `json:"RegistryId"`
		RecordID     string `json:"RecordId,omitempty"`
		VersionID    string `json:"VersionId,omitempty"`
		ActionFilter string `json:",omitempty"`
		Actor        string `json:",omitempty"`
		StartTime    string `json:",omitempty"`
		EndTime      string `json:",omitempty"`
		Offset       int
		Limit        int
	}{RegistryID: registryID, RecordID: in.RecordID, VersionID: in.VersionID, ActionFilter: in.Action, Actor: in.Actor, Offset: in.Offset, Limit: in.Limit}
	if in.Start != nil {
		req.StartTime = in.Start.UTC().Format(time.RFC3339)
	}
	if in.End != nil {
		req.EndTime = in.End.UTC().Format(time.RFC3339)
	}
	var response struct {
		AuditLogSet json.RawMessage
		TotalCount  *int64
		RequestID   string `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeRegistryAuditLogList", req, &response); err != nil {
		return RegistryAuditLogPage{}, err
	}
	var values []*struct {
		AuditLogID                         string `json:"AuditLogId"`
		RegistryID                         string `json:"RegistryId"`
		RecordID                           string `json:"RecordId"`
		VersionID                          string `json:"VersionId"`
		Actor, Action, Details, CreateTime string
	}
	if invalidCollection(response.AuditLogSet) || json.Unmarshal(response.AuditLogSet, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return RegistryAuditLogPage{}, malformedRegistryResponse("DescribeRegistryAuditLogList response is malformed")
	}
	out := RegistryAuditLogPage{Items: make([]RegistryAuditLog, 0, len(values)), TotalCount: *response.TotalCount, RequestID: response.RequestID}
	for _, item := range values {
		if item == nil || item.AuditLogID == "" || item.RegistryID == "" || item.Actor == "" || item.Action == "" || item.CreateTime == "" {
			return RegistryAuditLogPage{}, malformedRegistryResponse("AuditLog is missing required fields")
		}
		at, err := time.Parse(time.RFC3339Nano, item.CreateTime)
		if err != nil {
			return RegistryAuditLogPage{}, malformedRegistryResponse("AuditLog CreateTime is invalid")
		}
		details, err := registryJSONObject("AuditLog.Details", item.Details)
		if err != nil {
			return RegistryAuditLogPage{}, err
		}
		out.Items = append(out.Items, RegistryAuditLog{ID: item.AuditLogID, RegistryID: item.RegistryID, RecordID: item.RecordID, VersionID: item.VersionID, Actor: item.Actor, Action: item.Action, Details: details, CreatedAt: at})
	}
	return out, nil
}

type registryRecordMutationWire struct {
	RegistryID        string                      `json:"RegistryId"`
	RecordID          string                      `json:"RecordId,omitempty"`
	Name              string                      `json:",omitempty"`
	Description       *string                     `json:",omitempty"`
	VersionName       string                      `json:",omitempty"`
	ChangeLog         string                      `json:",omitempty"`
	DescriptorType    string                      `json:",omitempty"`
	MCPSource         *registrySourceWire         `json:",omitempty"`
	AgentSource       *registrySourceWire         `json:",omitempty"`
	SkillSource       *registrySkillSourceWire    `json:",omitempty"`
	CustomDescriptors string                      `json:",omitempty"`
	LabelMutations    []registryLabelMutationWire `json:",omitempty"`
}
type registryRecordResponseWire struct {
	Record                    *registryRecordWire
	Version                   *registryVersionWire
	ResolvedBy, ResolvedLabel string
	RequestID                 string `json:"RequestId"`
}

func registryRecordResultFromWire(response registryRecordResponseWire, requireVersion bool) (RegistryRecordResult, error) {
	record, err := registryRecordFromWire(response.Record)
	if err != nil {
		return RegistryRecordResult{}, err
	}
	var version *RegistryRecordVersion
	if response.Version != nil {
		value, versionErr := registryVersionFromWire(response.Version)
		err = versionErr
		if err != nil {
			return RegistryRecordResult{}, err
		}
		version = &value
	} else if requireVersion {
		return RegistryRecordResult{}, malformedRegistryResponse("Registry Record response has no Version")
	}
	if response.RequestID == "" {
		return RegistryRecordResult{}, malformedRegistryResponse("Registry Record response has no RequestId")
	}
	return RegistryRecordResult{Record: record, Version: version, ResolvedBy: response.ResolvedBy, ResolvedLabel: response.ResolvedLabel, RequestID: response.RequestID}, nil
}

type registrySelectorRequestWire struct {
	RegistryID string `json:"RegistryId"`
	RecordID   string `json:"RecordId"`
	VersionID  string `json:"VersionId,omitempty"`
	Label      string `json:",omitempty"`
	ChangeLog  string `json:",omitempty"`
}

func registrySelectorRequest(registryID, recordID string, selector *RegistrySelector) registrySelectorRequestWire {
	out := registrySelectorRequestWire{RegistryID: registryID, RecordID: recordID}
	if selector != nil {
		out.VersionID, out.Label = selector.VersionID, selector.Label
	}
	return out
}
func registrySourceToWire(value *RegistrySource) *registrySourceWire {
	if value == nil {
		return nil
	}
	return &registrySourceWire{Type: value.Type, Descriptors: rawString(value.Descriptors), EndpointURL: value.EndpointURL}
}
func registrySkillToWire(value *RegistrySkillSource) *registrySkillSourceWire {
	if value == nil {
		return nil
	}
	return &registrySkillSourceWire{Type: value.Type, SkillMD: value.SkillMD}
}
func registryTagsToWire(values []RegistryTag) []registryTagWire {
	out := make([]registryTagWire, len(values))
	for i, v := range values {
		out[i] = registryTagWire{v.Key, v.Value}
	}
	return out
}
func registryFiltersToWire(values []RegistryFilter) []registryFilterWire {
	out := make([]registryFilterWire, len(values))
	for i, v := range values {
		out[i] = registryFilterWire{v.Name, append([]string(nil), v.Values...)}
	}
	return out
}
func registryLabelsToWire(values []RegistryLabelMutation) []registryLabelMutationWire {
	out := make([]registryLabelMutationWire, len(values))
	for i, v := range values {
		out[i] = registryLabelMutationWire{v.Operation, v.Name, v.VersionID, v.Reason}
	}
	return out
}

func registryFromWire(value *registryWire) (Registry, error) {
	if value == nil || value.RegistryID == nil || *value.RegistryID == "" || value.Name == nil || value.Description == nil || value.ApprovalMode == nil || value.Region == nil || value.Status == nil || value.CreateTime == nil || value.UpdateTime == nil || value.RecordCount == nil || value.Tags == nil {
		return Registry{}, malformedRegistryResponse("Registry is missing required fields")
	}
	created, err := time.Parse(time.RFC3339Nano, *value.CreateTime)
	if err != nil {
		return Registry{}, malformedRegistryResponse("Registry CreateTime is invalid")
	}
	updated, err := time.Parse(time.RFC3339Nano, *value.UpdateTime)
	if err != nil {
		return Registry{}, malformedRegistryResponse("Registry UpdateTime is invalid")
	}
	var tags []registryTagWire
	if bytes.Equal(bytes.TrimSpace(value.Tags), []byte("null")) || json.Unmarshal(value.Tags, &tags) != nil {
		return Registry{}, malformedRegistryResponse("Registry Tags is invalid")
	}
	out := Registry{ID: *value.RegistryID, Name: *value.Name, Description: *value.Description, ApprovalMode: *value.ApprovalMode, Region: *value.Region, Status: *value.Status, CreatedAt: created, UpdatedAt: updated, RecordCount: *value.RecordCount, PublishedRecordCount: value.PublishedRecordCount, AppID: value.AppID, CreatorUIN: value.CreatorUIN, CreatorSubUIN: value.CreatorSubAccountUIN, Tags: make([]RegistryTag, len(tags))}
	for i, tag := range tags {
		out.Tags[i] = RegistryTag{tag.Key, tag.Value}
	}
	return out, nil
}

func registryRecordFromWire(value *registryRecordWire) (RegistryRecord, error) {
	if value == nil || value.RecordID == nil || *value.RecordID == "" || value.RegistryID == nil || value.Name == nil || value.Description == nil || value.DescriptorType == nil || value.LifecycleStatus == nil || value.AppID == nil || value.CreatorUIN == nil || value.CreatorSubAccountUIN == nil || value.CreateTime == nil || value.UpdateTime == nil {
		return RegistryRecord{}, malformedRegistryResponse("Record is missing required fields")
	}
	created, err := time.Parse(time.RFC3339Nano, *value.CreateTime)
	if err != nil {
		return RegistryRecord{}, malformedRegistryResponse("Record CreateTime is invalid")
	}
	updated, err := time.Parse(time.RFC3339Nano, *value.UpdateTime)
	if err != nil {
		return RegistryRecord{}, malformedRegistryResponse("Record UpdateTime is invalid")
	}
	return RegistryRecord{ID: *value.RecordID, RegistryID: *value.RegistryID, Name: *value.Name, Description: *value.Description, DescriptorType: *value.DescriptorType, Status: *value.LifecycleStatus, AppID: *value.AppID, CreatorUIN: *value.CreatorUIN, CreatorSubUIN: *value.CreatorSubAccountUIN, CreatedAt: created, UpdatedAt: updated, VersionCount: value.VersionCount, Labels: append([]string(nil), value.LabelSet...)}, nil
}

func registryVersionFromWire(value *registryVersionWire) (RegistryRecordVersion, error) {
	if value == nil || value.VersionID == "" || value.RecordID == "" {
		return RegistryRecordVersion{}, malformedRegistryResponse("Version is missing identity")
	}
	out := RegistryRecordVersion{ID: value.VersionID, RecordID: value.RecordID, Revision: value.Revision, AppID: value.AppID, Status: value.Status, ApprovalMode: value.ApprovalMode, CreatorUIN: value.CreatorUIN, CreatorSubUIN: value.CreatorSubAccountUIN, Name: value.VersionName, SourceType: value.SourceType, ContentStatus: value.ContentStatus, ContentSHA256: value.ContentSHA256, ContentSizeBytes: value.ContentSizeBytes, ConfigSHA256: value.ConfigSHA256, ChangeLog: value.ChangeLog, Labels: append([]string(nil), value.LabelSet...)}
	var err error
	if value.CreateTime != "" {
		out.CreatedAt, err = time.Parse(time.RFC3339Nano, value.CreateTime)
		if err != nil {
			return RegistryRecordVersion{}, malformedRegistryResponse("Version CreateTime is invalid")
		}
	}
	if value.UpdateTime != "" {
		out.UpdatedAt, err = time.Parse(time.RFC3339Nano, value.UpdateTime)
		if err != nil {
			return RegistryRecordVersion{}, malformedRegistryResponse("Version UpdateTime is invalid")
		}
	}
	if value.ContentReadyTime != "" {
		out.ContentReadyAt, err = time.Parse(time.RFC3339Nano, value.ContentReadyTime)
		if err != nil {
			return RegistryRecordVersion{}, malformedRegistryResponse("Version ContentReadyTime is invalid")
		}
	}
	if value.Descriptors != "" {
		out.Descriptors, err = registryJSONObject("Version.Descriptors", value.Descriptors)
		if err != nil {
			return RegistryRecordVersion{}, err
		}
	}
	if value.SourceConfig != "" {
		out.SourceConfig, err = registryJSONObject("Version.SourceConfig", value.SourceConfig)
		if err != nil {
			return RegistryRecordVersion{}, err
		}
	}
	out.ApprovalActions = make([]RegistryApprovalAction, len(value.ApprovalActions))
	for i, item := range value.ApprovalActions {
		out.ApprovalActions[i] = RegistryApprovalAction{ID: item.ActionID, Type: item.ActionType, ActorType: item.ActorType, ActorUIN: item.ActorUIN, ActorSubUIN: item.ActorSubAccountUIN, Comment: item.Comment, RequestID: item.RequestID}
		if item.CreateTime != "" {
			out.ApprovalActions[i].CreatedAt, err = time.Parse(time.RFC3339Nano, item.CreateTime)
			if err != nil {
				return RegistryRecordVersion{}, malformedRegistryResponse("Approval CreateTime is invalid")
			}
		}
	}
	return out, nil
}

func registryJSONObject(field, value string) (json.RawMessage, error) {
	raw := json.RawMessage(value)
	if len(raw) == 0 || len(raw) > maxRegistryJSONBytes || !json.Valid(raw) {
		return nil, malformedRegistryResponse(field + " JSON object is invalid")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, malformedRegistryResponse(field + " JSON object is invalid")
	}
	return append(json.RawMessage(nil), raw...), nil
}
func malformedRegistryResponse(reason string) error {
	return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New(reason)}
}
