package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestRegistryCloudJourney(t *testing.T) {
	if os.Getenv("AGS_E2E") != "1" {
		t.Fatal("real Registry E2E requires AGS_E2E=1")
	}
	for _, name := range []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY"} {
		if os.Getenv(name) == "" {
			t.Fatalf("real Registry E2E requires %s", name)
		}
	}
	client := newCloudClient(t, cloudEnvironment{region: "ap-shanghai"})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	runID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	manager := client.Registries()
	var registryID, registryName, recordID string
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		if recordID != "" {
			deleteRegistryRecordAndConfirm(t, cleanup, manager.Records(registryID), recordID)
		}
		if registryID != "" {
			deleteRegistryAndConfirm(t, cleanup, manager, registryID, registryName)
		}
	}()

	created, err := manager.Create(ctx, agr.CreateRegistryOptions{Name: "agr-sdk-e2e-" + runID, Description: "AGR SDK A5a owned test resource", ApprovalMode: agr.RegistryApprovalManual, Tags: []agr.RegistryTag{{Key: "agr-sdk-e2e", Value: "owned"}}})
	if err != nil {
		t.Fatalf("Create Registry: %v request=%s", err, cloudRequestID(err))
	}
	registryID = created.ID
	registryName = created.Name
	if registryID == "" || created.ApprovalMode != agr.RegistryApprovalManual {
		t.Fatalf("unsafe Registry result: %+v", created)
	}

	got, err := manager.Get(ctx, registryID)
	if err != nil || got.ID != registryID {
		t.Fatalf("Get Registry: %+v err=%v", got, err)
	}
	page, err := manager.List(ctx, agr.RegistryListOptions{Limit: 10, Filters: []agr.RegistryFilter{{Name: "name", Values: []string{created.Name}}}})
	if err != nil || !hasRegistry(page.Items, registryID) {
		t.Fatalf("List Registry: %+v err=%v", page, err)
	}
	updated, err := manager.Update(ctx, registryID, agr.UpdateRegistryOptions{Description: ""})
	if err != nil || updated.Description != "" {
		t.Fatalf("Update Registry: %+v err=%v", updated, err)
	}

	records := manager.Records(registryID)
	createdRecord, err := records.Create(ctx, agr.CreateRegistryRecordOptions{Name: "custom-" + runID, Description: "fixture", VersionName: "v1", DescriptorType: agr.RegistryDescriptorCustom, CustomDescriptors: json.RawMessage(`{"fixture":"v1"}`)})
	if err != nil {
		t.Fatalf("Create Record: %v request=%s", err, cloudRequestID(err))
	}
	if createdRecord.Version == nil {
		t.Fatalf("Create Record returned no Version: %+v", createdRecord)
	}
	recordID, versionID := createdRecord.Record.ID, createdRecord.Version.ID
	if recordID == "" || versionID == "" {
		t.Fatalf("unsafe Record result: %+v", createdRecord)
	}
	recordPage, err := records.List(ctx, agr.RegistryRecordListOptions{
		Limit:   10,
		Filters: []agr.RegistryFilter{{Name: "name", Values: []string{createdRecord.Record.Name}}},
	})
	if err != nil || !hasRegistryRecord(recordPage.Items, recordID) {
		t.Fatalf("List Registry Record: %+v err=%v", recordPage, err)
	}
	approved, err := records.Approve(ctx, recordID, versionID, "agr-sdk e2e")
	if err != nil || string(approved.Status) != "APPROVED" {
		t.Fatalf("Approve: %+v err=%v", approved, err)
	}

	description := "updated"
	labelVersion := versionID
	_, err = records.Update(ctx, recordID, agr.UpdateRegistryRecordOptions{Description: &description, LabelMutations: []agr.RegistryLabelMutation{{Operation: agr.RegistryLabelSet, Name: "stable", VersionID: &labelVersion, Reason: "e2e"}}})
	if err != nil {
		t.Fatalf("Update Record metadata: %v request=%s", err, cloudRequestID(err))
	}
	selected, err := records.Get(ctx, recordID, &agr.RegistryVersionSelector{Label: "stable"})
	if err != nil || selected.Version == nil || selected.Record.Description != description || selected.Version.ID != versionID || !jsonEqual(selected.Version.Descriptors, json.RawMessage(`{"fixture":"v1"}`)) {
		t.Fatalf("stable readback: %+v err=%v", selected, err)
	}

	for _, decision := range []struct {
		name, status string
		apply        func(context.Context, string, string, string) (agr.RegistryRecordVersionInfo, error)
	}{{"reject", "REJECTED", records.Reject}, {"cancel", "CANCELED", records.Cancel}} {
		next, createErr := records.Update(ctx, recordID, agr.UpdateRegistryRecordOptions{VersionName: decision.name, ChangeLog: "e2e", CustomDescriptors: json.RawMessage(fmt.Sprintf(`{"fixture":%q}`, decision.name))})
		if createErr != nil || next.Version == nil || next.Version.ID == "" {
			t.Fatalf("create %s Version: %+v err=%v", decision.name, next, createErr)
		}
		decided, decideErr := decision.apply(ctx, recordID, next.Version.ID, "agr-sdk e2e")
		if decideErr != nil || string(decided.Status) != decision.status {
			t.Fatalf("%s: %+v err=%v", decision.name, decided, decideErr)
		}
	}
	versions, err := records.ListVersions(ctx, recordID, agr.RegistryVersionListOptions{Limit: 10})
	if err != nil || versions.TotalCount < 3 {
		t.Fatalf("ListVersions: %+v err=%v", versions, err)
	}
	logs, err := waitForRegistryAuditLogs(ctx, records, recordID)
	if err != nil || logs.TotalCount == 0 || len(logs.Items) == 0 {
		t.Fatalf("ListAuditLogs: %+v err=%v cloud_code=%s request=%s chain=%s", logs, err, cloudErrorCode(err), cloudRequestID(err), errorTypeChain(err))
	}

	deleteRegistryRecordAndConfirm(t, ctx, records, recordID)
	recordID = ""
	deleteRegistryAndConfirm(t, ctx, manager, registryID, registryName)
	registryID = ""
}

func hasRegistry(values []agr.RegistryInfo, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func hasRegistryRecord(values []agr.RegistryRecordInfo, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func waitForRegistryAuditLogs(ctx context.Context, manager *agr.RegistryRecordManager, recordID string) (agr.RegistryAuditLogPage, error) {
	var page agr.RegistryAuditLogPage
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		page, err = manager.ListAuditLogs(ctx, agr.RegistryAuditLogListOptions{
			RecordID: recordID,
			Limit:    100,
		})
		if err == nil && len(page.Items) > 0 {
			return page, nil
		}
		select {
		case <-ctx.Done():
			return page, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return page, err
}

func deleteRegistryRecordAndConfirm(t *testing.T, ctx context.Context, manager *agr.RegistryRecordManager, id string) {
	t.Helper()
	if id == "" {
		return
	}
	if err := manager.Delete(ctx, id, agr.DeleteRegistryRecordOptions{}); err != nil && !isSDKNotFound(err) {
		t.Errorf("delete Registry Record: %v request=%s", err, cloudRequestID(err))
		return
	}
	if err := waitForRegistryRecordDeletion(ctx, manager, id); err != nil {
		t.Errorf("confirm Registry Record cleanup: %v cloud_code=%s request=%s chain=%s", err, cloudErrorCode(err), cloudRequestID(err), errorTypeChain(err))
	}
}

func waitForRegistryRecordDeletion(ctx context.Context, manager *agr.RegistryRecordManager, id string) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		_, err = manager.Get(ctx, id, nil)
		if isSDKNotFound(err) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if err == nil {
		return fmt.Errorf("Registry Record %q still exists", id)
	}
	return err
}

func deleteRegistryAndConfirm(t *testing.T, ctx context.Context, manager *agr.RegistryManager, id, name string) {
	t.Helper()
	if id == "" {
		return
	}
	if err := manager.Delete(ctx, id); err != nil && !isSDKNotFound(err) {
		t.Errorf("delete Registry: %v request=%s", err, cloudRequestID(err))
		return
	}
	if err := waitForRegistryDeletion(ctx, manager, id, name); err != nil {
		t.Errorf("confirm Registry cleanup: %v cloud_code=%s request=%s chain=%s", err, cloudErrorCode(err), cloudRequestID(err), errorTypeChain(err))
	}
}

func waitForRegistryDeletion(ctx context.Context, manager *agr.RegistryManager, id, name string) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		var page agr.RegistryPage
		page, err = manager.List(ctx, agr.RegistryListOptions{
			Limit:   100,
			Filters: []agr.RegistryFilter{{Name: "name", Values: []string{name}}},
		})
		if err == nil && !hasRegistry(page.Items, id) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if err == nil {
		return fmt.Errorf("Registry %q still exists", id)
	}
	return err
}
