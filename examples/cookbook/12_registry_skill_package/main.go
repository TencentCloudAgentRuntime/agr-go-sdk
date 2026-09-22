package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

const skillMD = "---\nname: cookbook-skill\ndescription: Disposable AGR SDK example\n---\n# Cookbook skill\n"

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	region := os.Getenv("TENCENTCLOUD_REGION")
	if region == "" {
		region = "ap-shanghai"
	}
	client, err := agr.NewClient(agr.WithRegion(region))
	if err != nil {
		return err
	}

	registry, err := client.Registries().Create(ctx, agr.CreateRegistryOptions{Name: fmt.Sprintf("skill-example-%d", time.Now().Unix()), Description: "temporary Skill package example", ApprovalMode: agr.RegistryApprovalAuto})
	if err != nil {
		return err
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

	created, err := records.Create(ctx, agr.CreateRegistryRecordOptions{Name: "cookbook-skill", VersionName: "v1", DescriptorType: agr.RegistryDescriptorAgentSkills, SkillSource: &agr.RegistrySkillSource{Type: agr.RegistrySourceTarPackage}})
	if err != nil {
		return fmt.Errorf("create TAR_PACKAGE Record: %w", err)
	}
	if created.Version == nil {
		return fmt.Errorf("create TAR_PACKAGE Record returned no Version")
	}
	recordID = created.Record.ID
	versionID := created.Version.ID
	archive, err := makeSkillArchive()
	if err != nil {
		return err
	}
	upload, err := records.UploadSkillPackage(ctx, recordID, versionID, bytes.NewReader(archive), agr.RegistrySkillPackageUploadOptions{SizeBytes: int64(len(archive))})
	if err != nil {
		return err
	}
	ready, err := waitReady(ctx, records, recordID, versionID)
	if err != nil {
		return err
	}

	var downloaded bytes.Buffer
	result, err := records.DownloadSkillPackage(ctx, recordID, &agr.RegistryVersionSelector{VersionID: versionID}, &downloaded, agr.RegistrySkillPackageDownloadOptions{})
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	if upload.SHA256 != digest || result.SHA256 != digest || !bytes.Equal(downloaded.Bytes(), archive) {
		return fmt.Errorf("verified package round trip did not preserve bytes")
	}
	fmt.Printf("record=%s version=%s status=%s bytes=%d\n", recordID, result.ResolvedVersionID, ready.ContentStatus, result.SizeBytes)

	if err = records.Delete(ctx, recordID, agr.DeleteRegistryRecordOptions{}); err != nil {
		return err
	}
	recordID = ""
	if err = client.Registries().Delete(ctx, registry.ID); err != nil {
		return err
	}
	return nil
}

func makeSkillArchive() ([]byte, error) {
	var output bytes.Buffer
	zip := gzip.NewWriter(&output)
	writer := tar.NewWriter(zip)
	if err := writer.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0o600, Size: int64(len(skillMD))}); err != nil {
		return nil, err
	}
	if _, err := writer.Write([]byte(skillMD)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if err := zip.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func waitReady(ctx context.Context, records *agr.RegistryRecordManager, recordID, versionID string) (agr.RegistryRecordVersionInfo, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		value, err := records.Get(ctx, recordID, &agr.RegistryVersionSelector{VersionID: versionID})
		if err != nil {
			return agr.RegistryRecordVersionInfo{}, err
		}
		if value.Version == nil {
			return agr.RegistryRecordVersionInfo{}, fmt.Errorf("record read returned no Version")
		}
		switch value.Version.ContentStatus {
		case "READY":
			return *value.Version, nil
		case "FAILED", "INVALID", "EXPIRED":
			return agr.RegistryRecordVersionInfo{}, fmt.Errorf("package processing ended in %s", value.Version.ContentStatus)
		}
		select {
		case <-ctx.Done():
			return agr.RegistryRecordVersionInfo{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
