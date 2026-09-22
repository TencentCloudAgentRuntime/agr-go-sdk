package agr

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type registryTestControl struct {
	metricControl
	calls        int
	registryID   string
	create       CreateRegistryRecordOptions
	update       UpdateRegistryRecordOptions
	deleted      DeleteRegistryRecordOptions
	selector     *RegistryVersionSelector
	registryList RegistryListOptions
	recordList   RegistryRecordListOptions
	versionList  RegistryVersionListOptions
	audit        RegistryAuditLogListOptions
}

func (c *registryTestControl) CreateRegistry(context.Context, CreateRegistryOptions) (RegistryInfo, error) {
	c.calls++
	return RegistryInfo{ID: "reg", Status: RegistryStatus("FUTURE")}, nil
}
func (c *registryTestControl) GetRegistry(context.Context, string) (RegistryInfo, error) {
	c.calls++
	return RegistryInfo{}, nil
}
func (c *registryTestControl) ListRegistries(_ context.Context, in RegistryListOptions) (RegistryPage, error) {
	c.calls++
	c.registryList = in
	next := in.Offset + 1
	return RegistryPage{Items: []RegistryInfo{{}}, TotalCount: 2, NextOffset: &next}, nil
}
func (c *registryTestControl) UpdateRegistry(context.Context, string, UpdateRegistryOptions) (RegistryInfo, error) {
	c.calls++
	return RegistryInfo{}, nil
}
func (c *registryTestControl) DeleteRegistry(context.Context, string) error { c.calls++; return nil }
func (c *registryTestControl) CreateRegistryRecord(_ context.Context, registryID string, in CreateRegistryRecordOptions) (RegistryRecordResult, error) {
	c.calls++
	c.registryID, c.create = registryID, in
	return RegistryRecordResult{Version: &RegistryRecordVersionInfo{Status: RegistryVersionStatus("FUTURE")}}, nil
}
func (c *registryTestControl) GetRegistryRecord(_ context.Context, registryID, _ string, selector *RegistryVersionSelector) (RegistryRecordResult, error) {
	c.calls++
	c.registryID, c.selector = registryID, selector
	return RegistryRecordResult{}, nil
}
func (c *registryTestControl) ListRegistryRecords(_ context.Context, registryID string, in RegistryRecordListOptions) (RegistryRecordPage, error) {
	c.calls++
	c.registryID, c.recordList = registryID, in
	next := in.Offset + 1
	return RegistryRecordPage{Items: []RegistryRecordInfo{{}}, TotalCount: 2, NextOffset: &next}, nil
}
func (c *registryTestControl) UpdateRegistryRecord(_ context.Context, registryID, _ string, in UpdateRegistryRecordOptions) (RegistryRecordResult, error) {
	c.calls++
	c.registryID, c.update = registryID, in
	return RegistryRecordResult{}, nil
}
func (c *registryTestControl) DeleteRegistryRecord(_ context.Context, registryID, _ string, in DeleteRegistryRecordOptions) error {
	c.calls++
	c.registryID, c.deleted = registryID, in
	return nil
}
func (c *registryTestControl) ListRegistryRecordVersions(_ context.Context, registryID, _ string, in RegistryVersionListOptions) (RegistryRecordVersionPage, error) {
	c.calls++
	c.registryID, c.versionList = registryID, in
	return RegistryRecordVersionPage{}, nil
}
func (c *registryTestControl) DecideRegistryRecord(context.Context, string, string, string, string, string) (RegistryRecordVersionInfo, error) {
	c.calls++
	return RegistryRecordVersionInfo{}, nil
}
func (c *registryTestControl) PreviewRegistryRecord(_ context.Context, registryID, _ string, selector *RegistryVersionSelector) (RegistryPreviewInfo, error) {
	c.calls++
	c.registryID, c.selector = registryID, selector
	return RegistryPreviewInfo{}, nil
}
func (c *registryTestControl) SyncRegistryRecord(_ context.Context, registryID, _ string, selector *RegistryVersionSelector, _ string) (RegistrySyncResult, error) {
	c.calls++
	c.registryID, c.selector = registryID, selector
	return RegistrySyncResult{}, nil
}
func (c *registryTestControl) ListRegistryAuditLogs(_ context.Context, registryID string, in RegistryAuditLogListOptions) (RegistryAuditLogPage, error) {
	c.calls++
	c.registryID, c.audit = registryID, in
	return RegistryAuditLogPage{}, nil
}

func newRegistryTestClient(t *testing.T, control *registryTestControl) *Client {
	t.Helper()
	client, err := NewClient(WithRegion("ap-test"), WithCredential(CloudCredential{SecretID: "id", SecretKey: "key"}), withControlPlane(control), withMonitorTransport(&metricTransport{}))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestRegistryInvalidInputsMakeNoNetworkCall(t *testing.T) {
	control := &registryTestControl{}
	client := newRegistryTestClient(t, control)
	ctx := context.Background()
	bad := json.RawMessage(`[]`)
	calls := []func() error{
		func() error { _, err := client.Registries().Create(ctx, CreateRegistryOptions{Name: " "}); return err },
		func() error { _, err := client.Registries().List(ctx, RegistryListOptions{Limit: 101}); return err },
		func() error {
			_, err := client.Registries().Records("").List(ctx, RegistryRecordListOptions{})
			return err
		},
		func() error {
			_, err := client.Registries().Records("reg").Create(ctx, CreateRegistryRecordOptions{Name: "x", DescriptorType: RegistryDescriptorCustom, CustomDescriptors: bad})
			return err
		},
		func() error {
			_, err := client.Registries().Records("reg").Get(ctx, "rec", &RegistryVersionSelector{})
			return err
		},
		func() error {
			return client.Registries().Records("reg").Delete(ctx, "rec", DeleteRegistryRecordOptions{Version: &RegistryVersionSelector{}})
		},
		func() error {
			_, err := client.Registries().Records("reg").Update(ctx, "rec", UpdateRegistryRecordOptions{})
			return err
		},
		func() error { _, err := client.Registries().Records("reg").Approve(ctx, "rec", "ver", " "); return err },
	}
	for _, call := range calls {
		var sdk *Error
		if err := call(); !errors.As(err, &sdk) || (sdk.Code != InvalidArgument && sdk.Code != Unsupported) {
			t.Fatalf("error=%#v", err)
		}
	}
	if control.calls != 0 {
		t.Fatalf("invalid input made %d calls", control.calls)
	}
}

func TestRegistrySourceUnionAndPackageBoundary(t *testing.T) {
	control := &registryTestControl{}
	records := newRegistryTestClient(t, control).Registries().Records("reg")
	ctx := context.Background()
	valid := CreateRegistryRecordOptions{Name: "mcp", DescriptorType: RegistryDescriptorMCP, MCPSource: &RegistryMCPSource{Type: RegistrySourceManual, Descriptors: json.RawMessage(`{"name":"mcp"}`)}}
	if _, err := records.Create(ctx, valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.AgentSource = &RegistryAgentSource{Type: RegistrySourceURLImport, EndpointURL: "https://example.invalid"}
	if _, err := records.Create(ctx, invalid); err == nil {
		t.Fatal("multiple sources accepted")
	}
	_, err := records.Create(ctx, CreateRegistryRecordOptions{Name: "skill", DescriptorType: RegistryDescriptorAgentSkills, SkillSource: &RegistrySkillSource{Type: RegistrySourceTarPackage}})
	var sdk *Error
	if !errors.As(err, &sdk) || sdk.Code != Unsupported || sdk.Reason != "SKILL_PACKAGE_TRANSFER_UNAVAILABLE" {
		t.Fatalf("package error=%#v", err)
	}
	if control.calls != 1 {
		t.Fatalf("invalid sources reached transport: %d", control.calls)
	}
}

func TestRegistryInputsAreDeepCopiedAndPresencePreserved(t *testing.T) {
	control := &registryTestControl{}
	records := newRegistryTestClient(t, control).Registries().Records("reg-a")
	raw := json.RawMessage(`{"kind":"custom"}`)
	if _, err := records.Create(context.Background(), CreateRegistryRecordOptions{Name: "custom", DescriptorType: RegistryDescriptorCustom, CustomDescriptors: raw}); err != nil {
		t.Fatal(err)
	}
	raw[2] = 'X'
	if string(control.create.CustomDescriptors) != `{"kind":"custom"}` || control.registryID != "reg-a" {
		t.Fatalf("create escaped: %#v", control.create)
	}
	empty := ""
	version := "rv-1"
	labels := []RegistryLabelMutation{{Operation: RegistryLabelSet, Name: "stable", VersionID: &version}}
	if _, err := records.Update(context.Background(), "rec", UpdateRegistryRecordOptions{Description: &empty, LabelMutations: labels}); err != nil {
		t.Fatal(err)
	}
	version = "changed"
	labels[0].Name = "changed"
	if control.update.Description == nil || *control.update.Description != "" || control.update.LabelMutations[0].Name != "stable" || *control.update.LabelMutations[0].VersionID != "rv-1" {
		t.Fatalf("update escaped: %#v", control.update)
	}
}

func TestRegistrySelectorAndDeletePresence(t *testing.T) {
	control := &registryTestControl{}
	records := newRegistryTestClient(t, control).Registries().Records("reg")
	if _, err := records.Get(context.Background(), "rec", nil); err != nil {
		t.Fatal(err)
	}
	if control.selector != nil {
		t.Fatalf("nil selector became %#v", control.selector)
	}
	selector := &RegistryVersionSelector{Label: "stable"}
	if _, err := records.Preview(context.Background(), "rec", selector); err != nil {
		t.Fatal(err)
	}
	selector.Label = "changed"
	if control.selector == nil || control.selector.Label != "stable" {
		t.Fatalf("selector escaped: %#v", control.selector)
	}
	if err := records.Delete(context.Background(), "rec", DeleteRegistryRecordOptions{}); err != nil {
		t.Fatal(err)
	}
	if control.deleted.Version != nil {
		t.Fatalf("whole-record delete gained selector: %#v", control.deleted)
	}
	version := "rv-1"
	if err := records.Delete(context.Background(), "rec", DeleteRegistryRecordOptions{Version: &RegistryVersionSelector{VersionID: version}, Reason: "obsolete"}); err != nil {
		t.Fatal(err)
	}
	if control.deleted.Version == nil || control.deleted.Version.VersionID != version {
		t.Fatalf("version selector lost: %#v", control.deleted)
	}
}

func TestRegistryPaginationAndIdentityIsolation(t *testing.T) {
	first, second := &registryTestControl{}, &registryTestControl{}
	a, b := newRegistryTestClient(t, first), newRegistryTestClient(t, second)
	page, err := a.Registries().List(context.Background(), RegistryListOptions{Limit: 1, Filters: []RegistryFilter{{Name: "name", Values: []string{"a"}}}})
	if err != nil || page.NextOffset == nil || *page.NextOffset != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	if _, err = b.Registries().Records("reg-b").List(context.Background(), RegistryRecordListOptions{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if first.calls != 1 || second.calls != 1 || second.registryID != "reg-b" {
		t.Fatalf("identity crossed: first=%d second=%d id=%q", first.calls, second.calls, second.registryID)
	}
}

func TestRegistryJSONLimitAndURLValidation(t *testing.T) {
	control := &registryTestControl{}
	records := newRegistryTestClient(t, control).Registries().Records("reg")
	tooLarge := json.RawMessage(`{"x":"` + strings.Repeat("a", maxRegistryJSONBytes) + `"}`)
	if _, err := records.Create(context.Background(), CreateRegistryRecordOptions{Name: "custom", DescriptorType: RegistryDescriptorCustom, CustomDescriptors: tooLarge}); err == nil {
		t.Fatal("oversized JSON accepted")
	}
	if _, err := records.Create(context.Background(), CreateRegistryRecordOptions{Name: "mcp", DescriptorType: RegistryDescriptorMCP, MCPSource: &RegistryMCPSource{Type: RegistrySourceURLImport, EndpointURL: "http://example.invalid"}}); err == nil {
		t.Fatal("non-HTTPS import accepted")
	}
	if control.calls != 0 {
		t.Fatalf("invalid source reached transport: %d", control.calls)
	}
}

func TestRegistryPublicModelsExcludePackageURLs(t *testing.T) {
	for _, value := range []any{CreateRegistryRecordOptions{}, UpdateRegistryRecordOptions{}, RegistryRecordResult{}, RegistrySyncResult{}} {
		typeOf := reflect.TypeOf(value)
		for i := 0; i < typeOf.NumField(); i++ {
			name := typeOf.Field(i).Name
			if name == "UploadURL" || name == "DownloadURL" {
				t.Fatalf("%s exposes sensitive %s", typeOf.Name(), name)
			}
		}
	}
}
