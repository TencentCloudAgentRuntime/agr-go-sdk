package agr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestVolumeAndTemplateCommonRequestMapping(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][]map[string]any{}
	var missingToken atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TC-Token") != "session-token" {
			missingToken.Add(1)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		action := r.Header.Get("X-TC-Action")
		mu.Lock()
		bodies[action] = append(bodies[action], body)
		mu.Unlock()
		response := map[string]any{"RequestId": "request-" + action}
		switch action {
		case "CreateVolume":
			response["Volume"] = volumeJSON("vol-created", "ACTIVE")
		case "DescribeVolumeList":
			response["VolumeSet"] = []any{volumeJSON("vol-created", "FUTURE_STATE")}
			response["TotalCount"] = 1
		case "CreateVolumeTemplate":
			response["VolumeTemplate"] = volumeTemplateJSON("volt-created", "ACTIVE")
		case "DescribeVolumeTemplateList":
			response["VolumeTemplateSet"] = []any{volumeTemplateJSON("volt-created", "FUTURE_STATE")}
			response["TotalCount"] = 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()

	client, err := NewClient(WithRegion("ap-test"), WithTemporaryCredential(TemporaryCloudCredential{SecretID: "id", SecretKey: "key", Token: "session-token"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.Volumes().Create(context.Background(), CreateVolumeOptions{Name: "workspace", AccessMode: VolumeReadWriteMany, StorageType: VolumeStorageCFS, Storage: VolumeStorage{CFS: &VolumeCFSStorage{FileSystemID: "cfs-fixture", Path: "/sdk/volume"}}, Tags: []VolumeTag{{Key: "owner", Value: "sdk"}}, ClientToken: "volume-token", StorageRoleARN: "role-fixture"})
	if err != nil || created.ID != "vol-created" || created.RequestID == "" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	page, err := client.Volumes().List(context.Background(), VolumeListOptions{Names: []string{"workspace"}, Filters: []VolumeFilter{{Name: "status", Values: []string{"ACTIVE"}}}, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].State != VolumeState("FUTURE_STATE") || page.RequestID == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	info, err := client.Volumes().Get(context.Background(), "vol-created")
	if err != nil || info.ID != "vol-created" {
		t.Fatalf("get=%+v err=%v", info, err)
	}
	emptyTags := []VolumeTag{}
	mutation, err := client.Volumes().Update(context.Background(), "vol-created", UpdateVolumeOptions{Tags: &emptyTags})
	if err != nil || mutation.VolumeID != "vol-created" || mutation.RequestID == "" {
		t.Fatalf("mutation=%+v err=%v", mutation, err)
	}
	if err := client.Volumes().Delete(context.Background(), "vol-created"); err != nil {
		t.Fatal(err)
	}
	byName, err := client.Volumes().UpdateByName(context.Background(), "workspace", UpdateVolumeOptions{Tags: &emptyTags})
	if err != nil || byName.VolumeID != "" || byName.RequestID == "" {
		t.Fatalf("name mutation=%+v err=%v", byName, err)
	}
	if err := client.Volumes().DeleteByName(context.Background(), "workspace"); err != nil {
		t.Fatal(err)
	}

	template, err := client.VolumeTemplates().Create(context.Background(), CreateVolumeTemplateOptions{Name: "workspace-template", AccessMode: VolumeReadWriteMany, ReclaimPolicy: VolumeReclaimRetain, StorageType: VolumeStorageCFS, Storage: VolumeTemplateStorage{CFS: &VolumeTemplateCFSStorage{FileSystemID: "cfs-fixture", PathPattern: "/sdk/${reuse_key}"}}, VolumeNamePattern: "workspace-${reuse_key}", Tags: []VolumeTag{{Key: "owner", Value: "sdk"}}, ClientToken: "template-token"})
	if err != nil || template.ID != "volt-created" || template.RequestID == "" {
		t.Fatalf("template=%+v err=%v", template, err)
	}
	templatePage, err := client.VolumeTemplates().List(context.Background(), VolumeTemplateListOptions{IDs: []string{"volt-created"}, Limit: 1})
	if err != nil || len(templatePage.Items) != 1 || templatePage.Items[0].State != VolumeState("FUTURE_STATE") {
		t.Fatalf("template page=%+v err=%v", templatePage, err)
	}
	capacity := ""
	templateMutation, err := client.VolumeTemplates().Update(context.Background(), "volt-created", UpdateVolumeTemplateOptions{Tags: &emptyTags, DefaultCapacity: &capacity})
	if err != nil || templateMutation.VolumeTemplateID != "volt-created" || templateMutation.RequestID == "" {
		t.Fatalf("template mutation=%+v err=%v", templateMutation, err)
	}
	if err := client.VolumeTemplates().Delete(context.Background(), "volt-created"); err != nil {
		t.Fatal(err)
	}
	templateByName, err := client.VolumeTemplates().UpdateByName(context.Background(), "workspace-template", UpdateVolumeTemplateOptions{Tags: &emptyTags})
	if err != nil || templateByName.VolumeTemplateID != "" || templateByName.RequestID == "" {
		t.Fatalf("template name mutation=%+v err=%v", templateByName, err)
	}
	if err := client.VolumeTemplates().DeleteByName(context.Background(), "workspace-template"); err != nil {
		t.Fatal(err)
	}

	if missingToken.Load() != 0 {
		t.Fatal("CommonClient omitted the STS token")
	}
	createBody := bodies["CreateVolume"][0]
	if createBody["VolumeName"] != "workspace" || createBody["SourceVolumeId"] != nil || createBody["StorageType"] != "Cfs" {
		t.Fatalf("CreateVolume body=%#v", createBody)
	}
	storage, ok := createBody["Storage"].(map[string]any)
	if !ok || storage["Cos"] != nil || storage["Cfs"] == nil {
		t.Fatalf("CreateVolume storage=%#v", createBody["Storage"])
	}
	listBody := bodies["DescribeVolumeList"][0]
	if listBody["Offset"] != float64(0) || listBody["Limit"] != float64(1) {
		t.Fatalf("DescribeVolumeList body=%#v", listBody)
	}
	updateBody := bodies["UpdateVolume"][0]
	if tags, ok := updateBody["Tags"].([]any); !ok || len(tags) != 0 || updateBody["VolumeName"] != nil {
		t.Fatalf("UpdateVolume body=%#v", updateBody)
	}
	nameUpdateBody := bodies["UpdateVolume"][1]
	if nameUpdateBody["VolumeName"] != "workspace" || nameUpdateBody["VolumeId"] != nil {
		t.Fatalf("UpdateVolume by name body=%#v", nameUpdateBody)
	}
	nameDeleteBody := bodies["DeleteVolume"][1]
	if nameDeleteBody["VolumeName"] != "workspace" || nameDeleteBody["VolumeId"] != nil {
		t.Fatalf("DeleteVolume by name body=%#v", nameDeleteBody)
	}
	templateBody := bodies["CreateVolumeTemplate"][0]
	if templateBody["VolumeTemplateName"] != "workspace-template" || templateBody["Name"] != nil || templateBody["StorageSpec"] == nil {
		t.Fatalf("CreateVolumeTemplate body=%#v", templateBody)
	}
	templateUpdate := bodies["UpdateVolumeTemplate"][0]
	if value, ok := templateUpdate["DefaultCapacity"]; !ok || value != "" {
		t.Fatalf("explicit empty capacity lost: %#v", templateUpdate)
	}
	templateNameUpdate := bodies["UpdateVolumeTemplate"][1]
	if templateNameUpdate["VolumeTemplateName"] != "workspace-template" || templateNameUpdate["VolumeTemplateId"] != nil {
		t.Fatalf("UpdateVolumeTemplate by name body=%#v", templateNameUpdate)
	}
	templateNameDelete := bodies["DeleteVolumeTemplate"][1]
	if templateNameDelete["VolumeTemplateName"] != "workspace-template" || templateNameDelete["VolumeTemplateId"] != nil {
		t.Fatalf("DeleteVolumeTemplate by name body=%#v", templateNameDelete)
	}
}

func TestVolumeGetRequiresExactSingleMatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []any
	}{{"zero", []any{}}, {"multiple", []any{volumeJSON("vol-want", "ACTIVE"), volumeJSON("vol-other", "ACTIVE")}}, {"mismatch", []any{volumeJSON("vol-other", "ACTIVE")}}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"VolumeSet": tc.items, "TotalCount": len(tc.items), "RequestId": "r"}})
			}))
			defer server.Close()
			client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
			_, err := client.Volumes().Get(context.Background(), "vol-want")
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != NotFound || failure.ResourceType != "volume" || failure.ResourceID != "vol-want" {
				t.Fatalf("error=%#v", err)
			}
		})
	}
}

func TestVolumeListPreservesUnknownStorageTypeWithoutPublishingDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{
			"VolumeSet": []any{map[string]any{
				"VolumeId": "vol-opaque", "VolumeName": "opaque", "AccessMode": "ReadWriteOnce", "ReclaimPolicy": "Retain", "Status": "ACTIVE", "StorageType": "AgentCbs",
				"Storage": map[string]any{"AgentCbs": map[string]any{"DiskId": "must-not-escape"}},
			}},
			"TotalCount": 1, "RequestId": "r",
		}})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	page, err := client.Volumes().List(context.Background(), VolumeListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].StorageType != VolumeStorageType("AgentCbs") || page.Items[0].Storage.COS != nil || page.Items[0].Storage.CFS != nil {
		t.Fatalf("opaque Volume = %+v", page.Items)
	}
	if strings.Contains(page.Items[0].String(), "must-not-escape") {
		t.Fatal("opaque backing details escaped through the public model")
	}
}

func TestVolumeTemplateGetRequiresExactSingleMatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []any
	}{{"zero", []any{}}, {"multiple", []any{volumeTemplateJSON("volt-want", "ACTIVE"), volumeTemplateJSON("volt-other", "ACTIVE")}}, {"mismatch", []any{volumeTemplateJSON("volt-other", "ACTIVE")}}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"VolumeTemplateSet": tc.items, "TotalCount": len(tc.items), "RequestId": "r"}})
			}))
			defer server.Close()
			client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
			_, err := client.VolumeTemplates().Get(context.Background(), "volt-want")
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != NotFound || failure.ResourceType != "volume_template" || failure.ResourceID != "volt-want" {
				t.Fatalf("error=%#v", err)
			}
		})
	}
}

func TestVolumeInvalidInputsMakeNoNetworkCallsAndCopyInputs(t *testing.T) {
	var calls atomic.Int32
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: roundTripFuncRoot(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network")
	})}))
	invalid := []CreateVolumeOptions{
		{Name: "", AccessMode: VolumeReadWriteMany, StorageType: VolumeStorageCFS, Storage: VolumeStorage{CFS: &VolumeCFSStorage{FileSystemID: "cfs", Path: "/"}}},
		{Name: "x", AccessMode: "future", StorageType: VolumeStorageCFS, Storage: VolumeStorage{CFS: &VolumeCFSStorage{FileSystemID: "cfs", Path: "/"}}},
		{Name: "x", AccessMode: VolumeReadWriteMany, StorageType: VolumeStorageCFS, Storage: VolumeStorage{COS: &VolumeCOSStorage{Endpoint: "e", BucketName: "b", BucketPath: "p"}}},
	}
	for _, value := range invalid {
		if _, err := client.Volumes().Create(context.Background(), value); err == nil {
			t.Fatalf("accepted invalid create: %#v", value)
		}
	}
	if _, err := client.Volumes().List(context.Background(), VolumeListOptions{Limit: 101}); err == nil {
		t.Fatal("accepted invalid pagination")
	}
	if _, err := client.Volumes().Update(context.Background(), "vol", UpdateVolumeOptions{}); err == nil {
		t.Fatal("accepted empty update")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid inputs made %d calls", calls.Load())
	}
	tags := []VolumeTag{{Key: "owner", Value: "before"}}
	storage := &VolumeCFSStorage{FileSystemID: "before", Path: "/before"}
	copy := copyCreateVolumeOptions(CreateVolumeOptions{Tags: tags, Storage: VolumeStorage{CFS: storage}})
	tags[0].Value, storage.FileSystemID = "after", "after"
	if copy.Tags[0].Value != "before" || copy.Storage.CFS.FileSystemID != "before" {
		t.Fatal("Volume options retained caller memory")
	}
}

func TestVolumeTemplateInvalidInputsMakeNoNetworkCallsAndCopyInputs(t *testing.T) {
	var calls atomic.Int32
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: roundTripFuncRoot(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network")
	})}))
	invalid := []CreateVolumeTemplateOptions{
		{Name: "", AccessMode: VolumeReadWriteMany, ReclaimPolicy: VolumeReclaimRetain, StorageType: VolumeStorageCFS, Storage: VolumeTemplateStorage{CFS: &VolumeTemplateCFSStorage{FileSystemID: "cfs", PathPattern: "/${reuse_key}"}}},
		{Name: "x", AccessMode: VolumeReadWriteMany, ReclaimPolicy: "future", StorageType: VolumeStorageCFS, Storage: VolumeTemplateStorage{CFS: &VolumeTemplateCFSStorage{FileSystemID: "cfs", PathPattern: "/${reuse_key}"}}},
		{Name: "x", AccessMode: VolumeReadWriteMany, ReclaimPolicy: VolumeReclaimRetain, StorageType: VolumeStorageCFS, Storage: VolumeTemplateStorage{COS: &VolumeTemplateCOSStorage{Endpoint: "e", BucketName: "b", BucketPathPattern: "p"}}},
	}
	for _, value := range invalid {
		if _, err := client.VolumeTemplates().Create(context.Background(), value); err == nil {
			t.Fatalf("accepted invalid template create: %#v", value)
		}
	}
	tooMany := make([]string, 101)
	for index := range tooMany {
		tooMany[index] = "volt"
	}
	if _, err := client.VolumeTemplates().List(context.Background(), VolumeTemplateListOptions{IDs: tooMany}); err == nil {
		t.Fatal("accepted too many template IDs")
	}
	if _, err := client.VolumeTemplates().Update(context.Background(), "volt", UpdateVolumeTemplateOptions{}); err == nil {
		t.Fatal("accepted empty template update")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid template inputs made %d calls", calls.Load())
	}
	tags := []VolumeTag{{Key: "owner", Value: "before"}}
	storage := &VolumeTemplateCFSStorage{FileSystemID: "before", PathPattern: "/before/${reuse_key}"}
	copied := copyCreateVolumeTemplateOptions(CreateVolumeTemplateOptions{Tags: tags, Storage: VolumeTemplateStorage{CFS: storage}})
	tags[0].Value, storage.FileSystemID = "after", "after"
	if copied.Tags[0].Value != "before" || copied.Storage.CFS.FileSystemID != "before" {
		t.Fatal("VolumeTemplate options retained caller memory")
	}
	capacity := "before"
	update := copyUpdateVolumeTemplateOptions(UpdateVolumeTemplateOptions{Tags: &tags, DefaultCapacity: &capacity})
	tags[0].Value, capacity = "later", "later"
	if (*update.Tags)[0].Value != "after" || *update.DefaultCapacity != "before" {
		t.Fatal("VolumeTemplate update options retained caller memory")
	}
}

func TestVolumeListDefaultsAndMountedDeleteConflict(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		response := map[string]any{"RequestId": "request-volume"}
		switch r.Header.Get("X-TC-Action") {
		case "DescribeVolumeList":
			response["VolumeSet"], response["TotalCount"] = []any{}, 0
		case "DescribeVolumeTemplateList":
			response["VolumeTemplateSet"], response["TotalCount"] = []any{}, 0
		case "DeleteVolume":
			response["Error"] = map[string]any{"Code": "ResourceInUse.VolumeMounted", "Message": "mounted"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if _, err := client.Volumes().List(context.Background(), VolumeListOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VolumeTemplates().List(context.Background(), VolumeTemplateListOptions{}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if bodies[index]["Offset"] != float64(0) || bodies[index]["Limit"] != float64(20) {
			t.Fatalf("default page request=%#v", bodies[index])
		}
	}
	err := client.Volumes().Delete(context.Background(), "vol-mounted")
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != Conflict || failure.ResourceType != "volume" || failure.ResourceID != "vol-mounted" {
		t.Fatalf("mounted delete error=%#v", err)
	}
}

func TestVolumeStringsRedactStorage(t *testing.T) {
	secret := "private-storage-location"
	storage := VolumeStorage{CFS: &VolumeCFSStorage{FileSystemID: secret, Path: "/" + secret}}
	values := []string{storage.String(), storage.GoString(), (CreateVolumeOptions{Storage: storage}).String(), (VolumeInfo{Storage: storage}).String()}
	for _, value := range values {
		if strings.Contains(value, secret) {
			t.Fatalf("storage leaked in %q", value)
		}
	}
}

func TestToolPreviewVolumeMountRequestAndResponseMapping(t *testing.T) {
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		response := map[string]any{"RequestId": "request-tool-preview"}
		switch r.Header.Get("X-TC-Action") {
		case "CreateSandboxTool":
			createBody = body
			response["ToolId"] = "sdt-volume"
		case "DescribeSandboxToolList":
			tool := toolJSON("sdt-volume", "ACTIVE")
			tool["StorageMounts"] = []any{
				map[string]any{"Name": "existing", "MountPath": "/mnt/existing", "ReadOnly": false, "Volume": map[string]any{"VolumeId": "vol-existing"}},
				map[string]any{"Name": "session", "MountPath": "/mnt/session", "ReadOnly": true, "VolumeTemplate": map[string]any{"VolumeTemplateName": "session-template", "ReuseKey": "${metadata.session_id}"}},
			}
			response["SandboxToolSet"], response["TotalCount"] = []any{tool}, 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithControlPlaneEndpoint(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	readOnly := true
	created, err := client.Tools().Create(context.Background(), CreateToolOptions{
		Name: "volume-tool", Type: ToolCustom, Network: ToolNetworkConfiguration{Mode: ToolNetworkSandbox},
		StorageMounts: []ToolStorageMount{
			{Name: "existing", MountPath: "/mnt/existing", Volume: &ToolVolumeRef{ID: "vol-existing", Name: "named-volume"}},
			{Name: "session", MountPath: "/mnt/session", ReadOnly: &readOnly, VolumeTemplate: &ToolVolumeTemplateRef{Name: "session-template"}},
		},
	})
	if err != nil || created.ToolID != "sdt-volume" {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	mounts, ok := createBody["StorageMounts"].([]any)
	if !ok || len(mounts) != 2 {
		t.Fatalf("StorageMounts=%#v", createBody["StorageMounts"])
	}
	first := mounts[0].(map[string]any)
	volume := first["Volume"].(map[string]any)
	if volume["VolumeId"] != "vol-existing" || volume["VolumeName"] != "named-volume" || first["StorageSource"] != nil || first["VolumeTemplate"] != nil {
		t.Fatalf("Volume mount=%#v", first)
	}
	second := mounts[1].(map[string]any)
	template := second["VolumeTemplate"].(map[string]any)
	if template["VolumeTemplateName"] != "session-template" || template["ReuseKey"] != nil || second["StorageSource"] != nil || second["Volume"] != nil {
		t.Fatalf("VolumeTemplate mount=%#v", second)
	}
	info, err := client.Tools().Get(context.Background(), "sdt-volume")
	if err != nil || len(info.StorageMounts) != 2 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if info.StorageMounts[0].Volume == nil || info.StorageMounts[0].Volume.ID != "vol-existing" {
		t.Fatalf("Volume response=%+v", info.StorageMounts[0])
	}
	if info.StorageMounts[1].VolumeTemplate == nil || info.StorageMounts[1].VolumeTemplate.ReuseKey != "${metadata.session_id}" {
		t.Fatalf("VolumeTemplate response=%+v", info.StorageMounts[1])
	}
}

func TestToolPreviewVolumeMountValidationAndCopy(t *testing.T) {
	var calls atomic.Int32
	client, _ := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{"id", "key"}), WithHTTPClient(&http.Client{Transport: roundTripFuncRoot(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network")
	})}))
	base := CreateToolOptions{Name: "invalid-volume", Type: ToolCustom, Network: ToolNetworkConfiguration{Mode: ToolNetworkSandbox}}
	invalid := [][]ToolStorageMount{
		{{Name: "conflict", Source: &ToolStorageSource{CFS: &ToolCFSStorage{FileSystemID: "cfs", Path: "/"}}, Volume: &ToolVolumeRef{ID: "vol"}}},
		{{Name: "relative-cos-path", Source: &ToolStorageSource{COS: &ToolCOSStorage{Endpoint: "bucket.cos.ap-test.myqcloud.com", BucketName: "bucket-1000000000", BucketPath: "relative"}}}},
		{{Name: "incomplete-cfs", Source: &ToolStorageSource{CFS: &ToolCFSStorage{FileSystemID: "cfs"}}}},
		{{Name: "missing-identity", Volume: &ToolVolumeRef{}}},
		{{Name: "missing-template-name", VolumeTemplate: &ToolVolumeTemplateRef{ReuseKey: "key"}}},
	}
	for _, mounts := range invalid {
		value := base
		value.StorageMounts = mounts
		if _, err := client.Tools().Create(context.Background(), value); err == nil {
			t.Fatalf("accepted invalid mounts: %#v", mounts)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid mounts made %d network calls", calls.Load())
	}
	if err := validateToolMounts([]ToolStorageMount{{Name: "optional-reuse", VolumeTemplate: &ToolVolumeTemplateRef{Name: "template"}}}, "Tools.Create"); err != nil {
		t.Fatalf("optional ReuseKey was rejected: %v", err)
	}
	volume := &ToolVolumeRef{ID: "before"}
	template := &ToolVolumeTemplateRef{Name: "before", ReuseKey: "${metadata.session_id}"}
	copied := copyToolMounts([]ToolStorageMount{{Name: "copy", Volume: volume}, {Name: "copy-template", VolumeTemplate: template}})
	volume.ID, template.Name = "after", "after"
	if copied[0].Volume.ID != "before" || copied[1].VolumeTemplate.Name != "before" || copied[1].VolumeTemplate.ReuseKey != "${metadata.session_id}" {
		t.Fatal("Preview Tool mount options retained caller memory or rewrote metadata expression")
	}
}

func volumeJSON(id, state string) map[string]any {
	return map[string]any{"VolumeId": id, "VolumeName": "workspace", "AccessMode": "ReadWriteMany", "ReclaimPolicy": "Retain", "Status": state, "StorageType": "Cfs", "Storage": map[string]any{"Cfs": map[string]any{"FileSystemId": "cfs-fixture", "Path": "/sdk/volume"}}, "Tags": []any{map[string]any{"Key": "owner", "Value": "sdk"}}, "CreatedAt": "2026-09-21T00:00:00Z", "UpdatedAt": "2026-09-21T00:01:00Z"}
}

func volumeTemplateJSON(id, state string) map[string]any {
	return map[string]any{"VolumeTemplateId": id, "VolumeTemplateName": "workspace-template", "AccessMode": "ReadWriteMany", "ReclaimPolicy": "Retain", "Status": state, "StorageType": "Cfs", "StorageSpec": map[string]any{"Cfs": map[string]any{"FileSystemId": "cfs-fixture", "PathPattern": "/sdk/${reuse_key}"}}, "VolumeNamePattern": "workspace-${reuse_key}", "Tags": []any{map[string]any{"Key": "owner", "Value": "sdk"}}, "CreatedAt": "2026-09-21T00:00:00Z", "UpdatedAt": "2026-09-21T00:01:00Z"}
}
