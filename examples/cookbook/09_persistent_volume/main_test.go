package main

import (
	"testing"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func TestVolumeFixtureFromEnvRequiresExactlyOneBackend(t *testing.T) {
	for _, tc := range []struct {
		name       string
		endpoint   string
		bucket     string
		fileSystem string
	}{
		{name: "none"},
		{name: "partial-cos-endpoint", endpoint: "cos.ap-guangzhou.myqcloud.com"},
		{name: "partial-cos-bucket", bucket: "fixture-1250000000"},
		{name: "both", endpoint: "cos.ap-guangzhou.myqcloud.com", bucket: "fixture-1250000000", fileSystem: "cfs-fixture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGR_VOLUME_COS_ENDPOINT", tc.endpoint)
			t.Setenv("AGR_VOLUME_COS_BUCKET", tc.bucket)
			t.Setenv("AGR_VOLUME_CFS_FILESYSTEM_ID", tc.fileSystem)
			if _, err := volumeFixtureFromEnv(); err == nil {
				t.Fatal("volumeFixtureFromEnv unexpectedly accepted an ambiguous fixture")
			}
		})
	}
}

func TestVolumeFixtureBuildsCOSAndCFSContracts(t *testing.T) {
	t.Run("cos", func(t *testing.T) {
		t.Setenv("AGR_VOLUME_COS_ENDPOINT", "cos.ap-guangzhou.myqcloud.com")
		t.Setenv("AGR_VOLUME_COS_BUCKET", "fixture-1250000000")
		t.Setenv("AGR_VOLUME_CFS_FILESYSTEM_ID", "")
		fixture, err := volumeFixtureFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if fixture.storageType != agr.VolumeStorageCOS {
			t.Fatalf("storage type = %q", fixture.storageType)
		}
		mount := fixture.bootstrapMount("agr-sdk-e2e/run")
		if mount.Source == nil || mount.Source.COS == nil || mount.Source.CFS != nil || mount.Source.COS.BucketPath != "agr-sdk-e2e/run" {
			t.Fatalf("COS bootstrap mount = %+v", mount)
		}
		storage := fixture.volumeStorage("agr-sdk-e2e/run/direct")
		if storage.COS == nil || storage.CFS != nil || storage.COS.BucketPath != "agr-sdk-e2e/run/direct" {
			t.Fatalf("COS Volume storage = %+v", storage)
		}
		if fixture.volumeTemplateStorage("agr-sdk-e2e/run/${reuse_key}").COS == nil {
			t.Fatal("COS template storage is missing")
		}
	})

	t.Run("cfs", func(t *testing.T) {
		t.Setenv("AGR_VOLUME_COS_ENDPOINT", "")
		t.Setenv("AGR_VOLUME_COS_BUCKET", "")
		t.Setenv("AGR_VOLUME_CFS_FILESYSTEM_ID", "cfs-fixture")
		fixture, err := volumeFixtureFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if fixture.storageType != agr.VolumeStorageCFS {
			t.Fatalf("storage type = %q", fixture.storageType)
		}
		mount := fixture.bootstrapMount("agr-sdk-e2e/run")
		if mount.Source == nil || mount.Source.CFS == nil || mount.Source.COS != nil || mount.Source.CFS.Path != "/" {
			t.Fatalf("CFS bootstrap mount = %+v", mount)
		}
		storage := fixture.volumeStorage("agr-sdk-e2e/run/direct")
		if storage.CFS == nil || storage.COS != nil || storage.CFS.Path != "/agr-sdk-e2e/run/direct" {
			t.Fatalf("CFS Volume storage = %+v", storage)
		}
		if fixture.volumeTemplateStorage("agr-sdk-e2e/run/${reuse_key}").CFS == nil {
			t.Fatal("CFS template storage is missing")
		}
	})
}
