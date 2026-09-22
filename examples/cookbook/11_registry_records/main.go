package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	region := os.Getenv("TENCENTCLOUD_REGION")
	if region == "" {
		region = "ap-guangzhou"
	}
	client, err := agr.NewClient(agr.WithRegion(region))
	if err != nil {
		log.Fatal(err)
	}

	registry, err := client.Registries().Create(ctx, agr.CreateRegistryOptions{Name: fmt.Sprintf("example-%d", time.Now().Unix()), Description: "temporary cookbook Registry", ApprovalMode: agr.RegistryApprovalManual})
	if err != nil {
		log.Fatal(err)
	}
	records := client.Registries().Records(registry.ID)
	var recordID string
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if recordID != "" {
			_ = records.Delete(cleanup, recordID, agr.DeleteRegistryRecordOptions{})
		}
		_ = client.Registries().Delete(cleanup, registry.ID)
	}()

	created, err := records.Create(ctx, agr.CreateRegistryRecordOptions{Name: "custom-tool", VersionName: "v1", DescriptorType: agr.RegistryDescriptorCustom, CustomDescriptors: json.RawMessage(`{"name":"custom-tool","version":"1.0.0"}`)})
	if err != nil {
		log.Fatal(err)
	}
	recordID = created.Record.ID
	if created.Version == nil {
		log.Fatal("create returned no Version")
	}
	approved, err := records.Approve(ctx, recordID, created.Version.ID, "publish cookbook version")
	if err != nil {
		log.Fatal(err)
	}
	versionID := approved.ID
	if _, err = records.Update(ctx, recordID, agr.UpdateRegistryRecordOptions{LabelMutations: []agr.RegistryLabelMutation{{Operation: agr.RegistryLabelSet, Name: "stable", VersionID: &versionID, Reason: "publish"}}}); err != nil {
		log.Fatal(err)
	}

	stable, err := records.Get(ctx, recordID, &agr.RegistryVersionSelector{Label: "stable"})
	if err != nil {
		log.Fatal(err)
	}
	logs, err := records.ListAuditLogs(ctx, agr.RegistryAuditLogListOptions{RecordID: recordID, Limit: 20})
	if err != nil {
		log.Fatal(err)
	}
	if stable.Version == nil {
		log.Fatal("stable label returned no Version")
	}
	fmt.Printf("record=%s version=%s audit_entries=%d\n", stable.Record.ID, stable.Version.ID, len(logs.Items))

	if err = records.Delete(ctx, recordID, agr.DeleteRegistryRecordOptions{}); err != nil {
		log.Fatal(err)
	}
	recordID = ""
	if err = client.Registries().Delete(ctx, registry.ID); err != nil {
		log.Fatal(err)
	}
}
