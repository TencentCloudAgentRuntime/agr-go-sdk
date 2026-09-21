package controlplane

import (
	"errors"
	"testing"

	"github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/model"
)

func TestDeploymentUnsupportedRegionClassification(t *testing.T) {
	err := cloudError(&Error{Code: "UnsupportedRegion", RequestID: "request-fixture"}, "CreateDeployment")
	var typed *model.Error
	if !errors.As(err, &typed) || typed.Code != model.InvalidArgument || typed.Reason != "CLOUD_UNSUPPORTED_REGION" || typed.Retryable || typed.RequestID != "request-fixture" {
		t.Fatalf("cloudError = %#v", err)
	}
}
