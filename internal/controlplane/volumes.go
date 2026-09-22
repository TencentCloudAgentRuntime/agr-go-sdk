package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/model"
)

type VolumeTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type VolumeFilter struct {
	Name   string   `json:"Name"`
	Values []string `json:"Values"`
}

type VolumeCOS struct {
	Endpoint, BucketName, BucketPath string
}

type VolumeCFS struct {
	FileSystemID, Path string
}

type VolumeStorage struct {
	COS *VolumeCOS
	CFS *VolumeCFS
}

type VolumeCOSSpec struct {
	Endpoint, BucketName, BucketPathPattern string
}

type VolumeCFSSpec struct {
	FileSystemID, PathPattern string
}

type VolumeStorageSpec struct {
	COS *VolumeCOSSpec
	CFS *VolumeCFSSpec
}

type Volume struct {
	ID, Name, AccessMode, ReclaimPolicy, State, StorageType string
	Storage                                                 VolumeStorage
	TemplateID, ReuseKey, StorageRoleARN, MountedInstanceID string
	Tags                                                    []VolumeTag
	CreatedAt, UpdatedAt                                    time.Time
}

type VolumeTemplate struct {
	ID, Name, AccessMode, ReclaimPolicy, State, StorageType string
	StorageSpec                                             VolumeStorageSpec
	VolumeNamePattern, DefaultCapacity, StorageRoleARN      string
	Tags                                                    []VolumeTag
	CreatedAt, UpdatedAt                                    time.Time
}

type CreateVolumeRequest struct {
	Name, AccessMode, StorageType, ClientToken, StorageRoleARN string
	Storage                                                    VolumeStorage
	Tags                                                       []VolumeTag
}

type CreateVolumeTemplateRequest struct {
	Name, AccessMode, ReclaimPolicy, StorageType       string
	VolumeNamePattern, DefaultCapacity, StorageRoleARN string
	ClientToken                                        string
	StorageSpec                                        VolumeStorageSpec
	Tags                                               []VolumeTag
}

type VolumeListRequest struct {
	IDs, Names    []string
	Filters       []VolumeFilter
	Offset, Limit int
}

type UpdateVolumeRequest struct{ Tags *[]VolumeTag }

type VolumeTarget struct{ ID, Name string }

type UpdateVolumeTemplateRequest struct {
	Tags            *[]VolumeTag
	DefaultCapacity *string
}

type VolumeResult struct {
	Volume    Volume
	RequestID string
}

type VolumeTemplateResult struct {
	Template  VolumeTemplate
	RequestID string
}

type VolumeMutation struct{ ResourceID, RequestID string }

type VolumePage struct {
	Items      []Volume
	TotalCount int64
	RequestID  string
}

type VolumeTemplatePage struct {
	Items      []VolumeTemplate
	TotalCount int64
	RequestID  string
}

func (s *Service) CreateVolume(ctx context.Context, request CreateVolumeRequest) (VolumeResult, error) {
	out, err := s.ags.CreateVolume(ctx, request)
	return out, cloudError(err, "CreateVolume")
}

func (s *Service) GetVolume(ctx context.Context, id string) (VolumeResult, error) {
	page, err := s.ListVolumes(ctx, VolumeListRequest{IDs: []string{id}, Limit: 1})
	if err != nil {
		return VolumeResult{}, err
	}
	if len(page.Items) != 1 || page.Items[0].ID != id {
		return VolumeResult{}, failure(model.NotFound, "Volumes.Get", "VOLUME_NOT_FOUND", false, nil)
	}
	return VolumeResult{Volume: page.Items[0], RequestID: page.RequestID}, nil
}

func (s *Service) ListVolumes(ctx context.Context, request VolumeListRequest) (VolumePage, error) {
	out, err := s.ags.ListVolumes(ctx, request)
	return out, cloudError(err, "DescribeVolumeList")
}

func (s *Service) UpdateVolume(ctx context.Context, target VolumeTarget, request UpdateVolumeRequest) (VolumeMutation, error) {
	out, err := s.ags.UpdateVolume(ctx, target, request)
	out.ResourceID = target.ID
	return out, cloudError(err, "UpdateVolume")
}

func (s *Service) DeleteVolume(ctx context.Context, target VolumeTarget) error {
	return cloudError(s.ags.DeleteVolume(ctx, target), "DeleteVolume")
}

func (s *Service) CreateVolumeTemplate(ctx context.Context, request CreateVolumeTemplateRequest) (VolumeTemplateResult, error) {
	out, err := s.ags.CreateVolumeTemplate(ctx, request)
	return out, cloudError(err, "CreateVolumeTemplate")
}

func (s *Service) GetVolumeTemplate(ctx context.Context, id string) (VolumeTemplateResult, error) {
	page, err := s.ListVolumeTemplates(ctx, VolumeListRequest{IDs: []string{id}, Limit: 1})
	if err != nil {
		return VolumeTemplateResult{}, err
	}
	if len(page.Items) != 1 || page.Items[0].ID != id {
		return VolumeTemplateResult{}, failure(model.NotFound, "VolumeTemplates.Get", "VOLUME_TEMPLATE_NOT_FOUND", false, nil)
	}
	return VolumeTemplateResult{Template: page.Items[0], RequestID: page.RequestID}, nil
}

func (s *Service) ListVolumeTemplates(ctx context.Context, request VolumeListRequest) (VolumeTemplatePage, error) {
	out, err := s.ags.ListVolumeTemplates(ctx, request)
	return out, cloudError(err, "DescribeVolumeTemplateList")
}

func (s *Service) UpdateVolumeTemplate(ctx context.Context, target VolumeTarget, request UpdateVolumeTemplateRequest) (VolumeMutation, error) {
	out, err := s.ags.UpdateVolumeTemplate(ctx, target, request)
	out.ResourceID = target.ID
	return out, cloudError(err, "UpdateVolumeTemplate")
}

func (s *Service) DeleteVolumeTemplate(ctx context.Context, target VolumeTarget) error {
	return cloudError(s.ags.DeleteVolumeTemplate(ctx, target), "DeleteVolumeTemplate")
}

type volumeCOSWire struct {
	Endpoint   string `json:"Endpoint"`
	BucketName string `json:"BucketName"`
	BucketPath string `json:"BucketPath"`
}

type volumeCFSWire struct {
	FileSystemID string `json:"FileSystemId"`
	Path         string `json:"Path"`
}

type volumeStorageWire struct {
	COS *volumeCOSWire `json:"Cos,omitempty"`
	CFS *volumeCFSWire `json:"Cfs,omitempty"`
}

type volumeCOSSpecWire struct {
	Endpoint, BucketName string
	BucketPathPattern    string
}

type volumeCFSSpecWire struct {
	FileSystemID string `json:"FileSystemId"`
	PathPattern  string `json:"PathPattern"`
}

type volumeStorageSpecWire struct {
	COS *volumeCOSSpecWire `json:"Cos,omitempty"`
	CFS *volumeCFSSpecWire `json:"Cfs,omitempty"`
}

type wireVolume struct {
	ID                string            `json:"VolumeId"`
	Name              string            `json:"VolumeName"`
	AccessMode        string            `json:"AccessMode"`
	ReclaimPolicy     string            `json:"ReclaimPolicy"`
	State             string            `json:"Status"`
	StorageType       string            `json:"StorageType"`
	Storage           volumeStorageWire `json:"Storage"`
	TemplateID        string            `json:"VolumeTemplateId"`
	ReuseKey          string            `json:"ReuseKey"`
	Tags              []VolumeTag       `json:"Tags"`
	CreatedAt         string            `json:"CreatedAt"`
	UpdatedAt         string            `json:"UpdatedAt"`
	StorageRoleARN    string            `json:"StorageRoleArn"`
	MountedInstanceID string            `json:"MountedInstanceId"`
}

type wireVolumeTemplate struct {
	ID                string                `json:"VolumeTemplateId"`
	Name              string                `json:"VolumeTemplateName"`
	AccessMode        string                `json:"AccessMode"`
	ReclaimPolicy     string                `json:"ReclaimPolicy"`
	State             string                `json:"Status"`
	StorageSpec       volumeStorageSpecWire `json:"StorageSpec"`
	StorageType       string                `json:"StorageType"`
	VolumeNamePattern string                `json:"VolumeNamePattern"`
	DefaultCapacity   string                `json:"DefaultCapacity"`
	StorageRoleARN    string                `json:"StorageRoleArn"`
	Tags              []VolumeTag           `json:"Tags"`
	CreatedAt         string                `json:"CreatedAt"`
	UpdatedAt         string                `json:"UpdatedAt"`
}

type createVolumeWireRequest struct {
	Name           string            `json:"VolumeName"`
	AccessMode     string            `json:"AccessMode"`
	StorageType    string            `json:"StorageType"`
	Storage        volumeStorageWire `json:"Storage"`
	Tags           []VolumeTag       `json:"Tags,omitempty"`
	ClientToken    string            `json:"ClientToken,omitempty"`
	StorageRoleARN string            `json:"StorageRoleArn,omitempty"`
}

type createVolumeTemplateWireRequest struct {
	Name              string                `json:"VolumeTemplateName"`
	AccessMode        string                `json:"AccessMode"`
	ReclaimPolicy     string                `json:"ReclaimPolicy"`
	StorageType       string                `json:"StorageType"`
	StorageSpec       volumeStorageSpecWire `json:"StorageSpec"`
	VolumeNamePattern string                `json:"VolumeNamePattern,omitempty"`
	DefaultCapacity   string                `json:"DefaultCapacity,omitempty"`
	StorageRoleARN    string                `json:"StorageRoleArn,omitempty"`
	Tags              []VolumeTag           `json:"Tags,omitempty"`
	ClientToken       string                `json:"ClientToken,omitempty"`
}

type volumeListWireRequest struct {
	IDs     []string       `json:"VolumeIds,omitempty"`
	Names   []string       `json:"VolumeNames,omitempty"`
	Filters []VolumeFilter `json:"Filters,omitempty"`
	Offset  int            `json:"Offset"`
	Limit   int            `json:"Limit"`
}

type volumeTemplateListWireRequest struct {
	IDs     []string       `json:"VolumeTemplateIds,omitempty"`
	Names   []string       `json:"VolumeTemplateNames,omitempty"`
	Filters []VolumeFilter `json:"Filters,omitempty"`
	Offset  int            `json:"Offset"`
	Limit   int            `json:"Limit"`
}

type updateVolumeWireRequest struct {
	ID   string       `json:"VolumeId,omitempty"`
	Name string       `json:"VolumeName,omitempty"`
	Tags *[]VolumeTag `json:"Tags,omitempty"`
}

type updateVolumeTemplateWireRequest struct {
	ID              string       `json:"VolumeTemplateId,omitempty"`
	Name            string       `json:"VolumeTemplateName,omitempty"`
	Tags            *[]VolumeTag `json:"Tags,omitempty"`
	DefaultCapacity *string      `json:"DefaultCapacity,omitempty"`
}

type requestIDResponse struct {
	RequestID string `json:"RequestId"`
}

func (a *AGS) CreateVolume(ctx context.Context, in CreateVolumeRequest) (VolumeResult, error) {
	request := createVolumeWireRequest{Name: in.Name, AccessMode: in.AccessMode, StorageType: in.StorageType, Storage: volumeStorageToWire(in.Storage), Tags: in.Tags, ClientToken: in.ClientToken, StorageRoleARN: in.StorageRoleARN}
	var response struct {
		Volume    *wireVolume `json:"Volume"`
		RequestID string      `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "CreateVolume", request, &response); err != nil {
		return VolumeResult{}, err
	}
	if response.Volume == nil {
		return VolumeResult{}, malformedVolumeResponse("CreateVolume response has no Volume")
	}
	if response.RequestID == "" {
		return VolumeResult{}, malformedVolumeResponse("CreateVolume response has no RequestId")
	}
	value, err := volumeFromWire(response.Volume)
	return VolumeResult{Volume: value, RequestID: response.RequestID}, err
}

func (a *AGS) ListVolumes(ctx context.Context, in VolumeListRequest) (VolumePage, error) {
	request := volumeListWireRequest{IDs: in.IDs, Names: in.Names, Filters: in.Filters, Offset: in.Offset, Limit: in.Limit}
	var response struct {
		Items      json.RawMessage `json:"VolumeSet"`
		TotalCount *int64          `json:"TotalCount"`
		RequestID  string          `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeVolumeList", request, &response); err != nil {
		return VolumePage{}, err
	}
	var values []*wireVolume
	if invalidCollection(response.Items) || json.Unmarshal(response.Items, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return VolumePage{}, malformedVolumeResponse("DescribeVolumeList response has an invalid collection")
	}
	out := VolumePage{TotalCount: *response.TotalCount, RequestID: response.RequestID, Items: make([]Volume, 0, len(values))}
	for _, item := range values {
		value, err := volumeFromWire(item)
		if err != nil {
			return VolumePage{}, err
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

func (a *AGS) UpdateVolume(ctx context.Context, target VolumeTarget, in UpdateVolumeRequest) (VolumeMutation, error) {
	request := updateVolumeWireRequest{ID: target.ID, Name: target.Name, Tags: in.Tags}
	var response requestIDResponse
	err := a.commonCall(ctx, "UpdateVolume", request, &response)
	if err == nil && response.RequestID == "" {
		err = malformedVolumeResponse("UpdateVolume response has no RequestId")
	}
	return VolumeMutation{ResourceID: target.ID, RequestID: response.RequestID}, err
}

func (a *AGS) DeleteVolume(ctx context.Context, target VolumeTarget) error {
	var response requestIDResponse
	err := a.commonCall(ctx, "DeleteVolume", struct {
		ID   string `json:"VolumeId,omitempty"`
		Name string `json:"VolumeName,omitempty"`
	}{ID: target.ID, Name: target.Name}, &response)
	if err == nil && response.RequestID == "" {
		return malformedVolumeResponse("DeleteVolume response has no RequestId")
	}
	return err
}

func (a *AGS) CreateVolumeTemplate(ctx context.Context, in CreateVolumeTemplateRequest) (VolumeTemplateResult, error) {
	request := createVolumeTemplateWireRequest{Name: in.Name, AccessMode: in.AccessMode, ReclaimPolicy: in.ReclaimPolicy, StorageType: in.StorageType, StorageSpec: volumeStorageSpecToWire(in.StorageSpec), VolumeNamePattern: in.VolumeNamePattern, DefaultCapacity: in.DefaultCapacity, StorageRoleARN: in.StorageRoleARN, Tags: in.Tags, ClientToken: in.ClientToken}
	var response struct {
		Template  *wireVolumeTemplate `json:"VolumeTemplate"`
		RequestID string              `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "CreateVolumeTemplate", request, &response); err != nil {
		return VolumeTemplateResult{}, err
	}
	if response.Template == nil {
		return VolumeTemplateResult{}, malformedVolumeResponse("CreateVolumeTemplate response has no VolumeTemplate")
	}
	if response.RequestID == "" {
		return VolumeTemplateResult{}, malformedVolumeResponse("CreateVolumeTemplate response has no RequestId")
	}
	value, err := volumeTemplateFromWire(response.Template)
	return VolumeTemplateResult{Template: value, RequestID: response.RequestID}, err
}

func (a *AGS) ListVolumeTemplates(ctx context.Context, in VolumeListRequest) (VolumeTemplatePage, error) {
	request := volumeTemplateListWireRequest{IDs: in.IDs, Names: in.Names, Filters: in.Filters, Offset: in.Offset, Limit: in.Limit}
	var response struct {
		Items      json.RawMessage `json:"VolumeTemplateSet"`
		TotalCount *int64          `json:"TotalCount"`
		RequestID  string          `json:"RequestId"`
	}
	if err := a.commonCall(ctx, "DescribeVolumeTemplateList", request, &response); err != nil {
		return VolumeTemplatePage{}, err
	}
	var values []*wireVolumeTemplate
	if invalidCollection(response.Items) || json.Unmarshal(response.Items, &values) != nil || response.TotalCount == nil || *response.TotalCount < 0 || response.RequestID == "" {
		return VolumeTemplatePage{}, malformedVolumeResponse("DescribeVolumeTemplateList response has an invalid collection")
	}
	out := VolumeTemplatePage{TotalCount: *response.TotalCount, RequestID: response.RequestID, Items: make([]VolumeTemplate, 0, len(values))}
	for _, item := range values {
		value, err := volumeTemplateFromWire(item)
		if err != nil {
			return VolumeTemplatePage{}, err
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

func (a *AGS) UpdateVolumeTemplate(ctx context.Context, target VolumeTarget, in UpdateVolumeTemplateRequest) (VolumeMutation, error) {
	request := updateVolumeTemplateWireRequest{ID: target.ID, Name: target.Name, Tags: in.Tags, DefaultCapacity: in.DefaultCapacity}
	var response requestIDResponse
	err := a.commonCall(ctx, "UpdateVolumeTemplate", request, &response)
	if err == nil && response.RequestID == "" {
		err = malformedVolumeResponse("UpdateVolumeTemplate response has no RequestId")
	}
	return VolumeMutation{ResourceID: target.ID, RequestID: response.RequestID}, err
}

func (a *AGS) DeleteVolumeTemplate(ctx context.Context, target VolumeTarget) error {
	var response requestIDResponse
	err := a.commonCall(ctx, "DeleteVolumeTemplate", struct {
		ID   string `json:"VolumeTemplateId,omitempty"`
		Name string `json:"VolumeTemplateName,omitempty"`
	}{ID: target.ID, Name: target.Name}, &response)
	if err == nil && response.RequestID == "" {
		return malformedVolumeResponse("DeleteVolumeTemplate response has no RequestId")
	}
	return err
}

func volumeFromWire(value *wireVolume) (Volume, error) {
	if value == nil || value.ID == "" || value.Name == "" || value.AccessMode == "" || value.ReclaimPolicy == "" || value.State == "" || value.StorageType == "" {
		return Volume{}, malformedVolumeResponse("Volume is missing required fields")
	}
	storage, err := volumeStorageFromWire(value.StorageType, value.Storage)
	if err != nil {
		return Volume{}, err
	}
	created, err := parseOptionalVolumeTime(value.CreatedAt)
	if err != nil {
		return Volume{}, err
	}
	updated, err := parseOptionalVolumeTime(value.UpdatedAt)
	if err != nil {
		return Volume{}, err
	}
	return Volume{ID: value.ID, Name: value.Name, AccessMode: value.AccessMode, ReclaimPolicy: value.ReclaimPolicy, State: value.State, StorageType: value.StorageType, Storage: storage, TemplateID: value.TemplateID, ReuseKey: value.ReuseKey, Tags: append([]VolumeTag(nil), value.Tags...), CreatedAt: created, UpdatedAt: updated, StorageRoleARN: value.StorageRoleARN, MountedInstanceID: value.MountedInstanceID}, nil
}

func volumeTemplateFromWire(value *wireVolumeTemplate) (VolumeTemplate, error) {
	if value == nil || value.ID == "" || value.Name == "" || value.AccessMode == "" || value.ReclaimPolicy == "" || value.State == "" || value.StorageType == "" {
		return VolumeTemplate{}, malformedVolumeResponse("VolumeTemplate is missing required fields")
	}
	storage, err := volumeStorageSpecFromWire(value.StorageType, value.StorageSpec)
	if err != nil {
		return VolumeTemplate{}, err
	}
	created, err := parseOptionalVolumeTime(value.CreatedAt)
	if err != nil {
		return VolumeTemplate{}, err
	}
	updated, err := parseOptionalVolumeTime(value.UpdatedAt)
	if err != nil {
		return VolumeTemplate{}, err
	}
	return VolumeTemplate{ID: value.ID, Name: value.Name, AccessMode: value.AccessMode, ReclaimPolicy: value.ReclaimPolicy, State: value.State, StorageType: value.StorageType, StorageSpec: storage, VolumeNamePattern: value.VolumeNamePattern, DefaultCapacity: value.DefaultCapacity, StorageRoleARN: value.StorageRoleARN, Tags: append([]VolumeTag(nil), value.Tags...), CreatedAt: created, UpdatedAt: updated}, nil
}

func volumeStorageToWire(value VolumeStorage) volumeStorageWire {
	out := volumeStorageWire{}
	if value.COS != nil {
		out.COS = &volumeCOSWire{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPath: value.COS.BucketPath}
	}
	if value.CFS != nil {
		out.CFS = &volumeCFSWire{FileSystemID: value.CFS.FileSystemID, Path: value.CFS.Path}
	}
	return out
}

func volumeStorageSpecToWire(value VolumeStorageSpec) volumeStorageSpecWire {
	out := volumeStorageSpecWire{}
	if value.COS != nil {
		out.COS = &volumeCOSSpecWire{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPathPattern: value.COS.BucketPathPattern}
	}
	if value.CFS != nil {
		out.CFS = &volumeCFSSpecWire{FileSystemID: value.CFS.FileSystemID, PathPattern: value.CFS.PathPattern}
	}
	return out
}

func volumeStorageFromWire(storageType string, value volumeStorageWire) (VolumeStorage, error) {
	if storageType != "Cos" && storageType != "Cfs" {
		// The account-wide List API can contain storage backends that this
		// Preview SDK deliberately does not model. Preserve StorageType on the
		// resource while keeping those backend details private and opaque.
		return VolumeStorage{}, nil
	}
	count := 0
	out := VolumeStorage{}
	if value.COS != nil {
		count++
		if value.COS.Endpoint == "" || value.COS.BucketName == "" || value.COS.BucketPath == "" {
			return VolumeStorage{}, malformedVolumeResponse("Volume COS storage is incomplete")
		}
		out.COS = &VolumeCOS{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPath: value.COS.BucketPath}
	}
	if value.CFS != nil {
		count++
		if value.CFS.FileSystemID == "" || value.CFS.Path == "" {
			return VolumeStorage{}, malformedVolumeResponse("Volume CFS storage is incomplete")
		}
		out.CFS = &VolumeCFS{FileSystemID: value.CFS.FileSystemID, Path: value.CFS.Path}
	}
	if count != 1 || (storageType == "Cos") != (out.COS != nil) || (storageType == "Cfs") != (out.CFS != nil) {
		return VolumeStorage{}, malformedVolumeResponse("Volume storage does not match StorageType")
	}
	return out, nil
}

func volumeStorageSpecFromWire(storageType string, value volumeStorageSpecWire) (VolumeStorageSpec, error) {
	if storageType != "Cos" && storageType != "Cfs" {
		return VolumeStorageSpec{}, nil
	}
	count := 0
	out := VolumeStorageSpec{}
	if value.COS != nil {
		count++
		if value.COS.Endpoint == "" || value.COS.BucketName == "" || value.COS.BucketPathPattern == "" {
			return VolumeStorageSpec{}, malformedVolumeResponse("VolumeTemplate COS storage is incomplete")
		}
		out.COS = &VolumeCOSSpec{Endpoint: value.COS.Endpoint, BucketName: value.COS.BucketName, BucketPathPattern: value.COS.BucketPathPattern}
	}
	if value.CFS != nil {
		count++
		if value.CFS.FileSystemID == "" || value.CFS.PathPattern == "" {
			return VolumeStorageSpec{}, malformedVolumeResponse("VolumeTemplate CFS storage is incomplete")
		}
		out.CFS = &VolumeCFSSpec{FileSystemID: value.CFS.FileSystemID, PathPattern: value.CFS.PathPattern}
	}
	if count != 1 || (storageType == "Cos") != (out.COS != nil) || (storageType == "Cfs") != (out.CFS != nil) {
		return VolumeStorageSpec{}, malformedVolumeResponse("VolumeTemplate storage does not match StorageType")
	}
	return out, nil
}

func invalidCollection(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || trimmed[0] != '['
}

func parseOptionalVolumeTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, malformedVolumeResponse("Volume timestamp is not RFC3339")
	}
	return parsed, nil
}

func malformedVolumeResponse(reason string) error {
	return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New(reason)}
}
