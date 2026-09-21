package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type DeploymentState string

const (
	DeploymentActive       DeploymentState = "ACTIVE"
	DeploymentDeleting     DeploymentState = "DELETING"
	DeploymentDeleteFailed DeploymentState = "DELETE_FAILED"
)

type DeploymentScalingInput struct {
	MinInstances, MaxInstances, MaxConcurrency *int64
}

type DeploymentLifecycleInput struct {
	IdleTimeoutSeconds *int64
	IdleAction         *string
}

type DeploymentAffinityInput struct {
	Mode, HeaderName string
}

type DeploymentScaling struct {
	MinInstances, MaxInstances, MaxConcurrency int64
}

type DeploymentLifecycle struct {
	IdleTimeoutSeconds int64
	IdleAction         string
}

type DeploymentAffinity struct {
	Mode, HeaderName string
}

type DeploymentTag struct{ Key, Value string }

type Deployment struct {
	ID, Name, ToolID, StatusReason string
	State                          DeploymentState
	Scaling                        DeploymentScaling
	Lifecycle                      DeploymentLifecycle
	Affinity                       *DeploymentAffinity
	Tags                           []DeploymentTag
	CreatedAt, UpdatedAt           time.Time
}

type CreateDeploymentRequest struct {
	Name, ToolID string
	Scaling      *DeploymentScalingInput
	Lifecycle    *DeploymentLifecycleInput
	Affinity     *DeploymentAffinityInput
	Tags         []DeploymentTag
}

type UpdateDeploymentRequest struct {
	Scaling   *DeploymentScalingInput
	Lifecycle *DeploymentLifecycleInput
	Tags      *[]DeploymentTag
}

type DeploymentFilter struct {
	Name   string
	Values []string
}

type DeploymentListRequest struct {
	Offset, Limit int
	Filters       []DeploymentFilter
}

type DeploymentResult struct {
	Deployment Deployment
	RequestID  string
}

type DeploymentPage struct {
	Items      []Deployment
	TotalCount int64
	RequestID  string
}

type DeploymentToken struct {
	Token, RequestID string
	ExpiresAt        time.Time
}

func (s *Service) CreateDeployment(ctx context.Context, request CreateDeploymentRequest) (DeploymentResult, error) {
	out, err := s.ags.CreateDeployment(ctx, request)
	return out, cloudError(err, "CreateDeployment")
}

func (s *Service) GetDeployment(ctx context.Context, id string) (DeploymentResult, error) {
	out, err := s.ags.GetDeployment(ctx, id)
	return out, cloudError(err, "DescribeDeployment")
}

func (s *Service) ListDeployments(ctx context.Context, request DeploymentListRequest) (DeploymentPage, error) {
	out, err := s.ags.ListDeployments(ctx, request)
	return out, cloudError(err, "DescribeDeploymentList")
}

func (s *Service) UpdateDeployment(ctx context.Context, id string, request UpdateDeploymentRequest) (DeploymentResult, error) {
	out, err := s.ags.UpdateDeployment(ctx, id, request)
	return out, cloudError(err, "ModifyDeployment")
}

func (s *Service) DeleteDeployment(ctx context.Context, id string) error {
	return cloudError(s.ags.DeleteDeployment(ctx, id), "DeleteDeployment")
}

func (s *Service) AcquireDeploymentToken(ctx context.Context, id string) (DeploymentToken, error) {
	out, err := s.ags.AcquireDeploymentToken(ctx, id)
	return out, cloudError(err, "AcquireDeploymentToken")
}

func deploymentScalingInput(value *DeploymentScalingInput) *ags.ScalingConfiguration {
	if value == nil {
		return nil
	}
	return &ags.ScalingConfiguration{MinInstanceCount: value.MinInstances, MaxInstanceCount: value.MaxInstances, MaxInstanceRequestConcurrency: value.MaxConcurrency}
}

func deploymentLifecycleInput(value *DeploymentLifecycleInput) *ags.LifecycleConfiguration {
	if value == nil {
		return nil
	}
	return &ags.LifecycleConfiguration{IdleTimeoutSeconds: value.IdleTimeoutSeconds, IdleAction: value.IdleAction}
}

func deploymentAffinityInput(value *DeploymentAffinityInput) *ags.AffinityConfiguration {
	if value == nil {
		return nil
	}
	return &ags.AffinityConfiguration{Mode: stringPtr(value.Mode), HeaderName: stringPtr(value.HeaderName)}
}

func deploymentTags(values []DeploymentTag) []*ags.Tag {
	var out []*ags.Tag
	for _, value := range values {
		key, item := value.Key, value.Value
		out = append(out, &ags.Tag{Key: &key, Value: &item})
	}
	return out
}

func deploymentFrom(value *ags.Deployment) (Deployment, error) {
	out := Deployment{ID: valueString(value.DeploymentId), Name: valueString(value.DeploymentName), ToolID: valueString(value.ToolId), State: DeploymentState(valueString(value.Status)), StatusReason: valueString(value.StatusReason)}
	if value.ScalingConfiguration != nil {
		out.Scaling = DeploymentScaling{MinInstances: valueInt64(value.ScalingConfiguration.MinInstanceCount), MaxInstances: valueInt64(value.ScalingConfiguration.MaxInstanceCount), MaxConcurrency: valueInt64(value.ScalingConfiguration.MaxInstanceRequestConcurrency)}
	}
	if value.LifecycleConfiguration != nil {
		out.Lifecycle = DeploymentLifecycle{IdleTimeoutSeconds: valueInt64(value.LifecycleConfiguration.IdleTimeoutSeconds), IdleAction: valueString(value.LifecycleConfiguration.IdleAction)}
	}
	if value.AffinityConfiguration != nil {
		out.Affinity = &DeploymentAffinity{Mode: valueString(value.AffinityConfiguration.Mode), HeaderName: valueString(value.AffinityConfiguration.HeaderName)}
	}
	for _, tag := range value.Tags {
		if tag != nil {
			out.Tags = append(out.Tags, DeploymentTag{Key: valueString(tag.Key), Value: valueString(tag.Value)})
		}
	}
	var err error
	if raw := valueString(value.CreatedTime); raw != "" {
		out.CreatedAt, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return Deployment{}, malformedDeploymentResponse("Deployment", "CreatedTime is not RFC3339")
		}
	}
	if raw := valueString(value.UpdatedTime); raw != "" {
		out.UpdatedAt, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return Deployment{}, malformedDeploymentResponse("Deployment", "UpdatedTime is not RFC3339")
		}
	}
	return out, nil
}

func malformedDeploymentResponse(action, reason string) error {
	return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New(fmt.Sprintf("%s %s", action, reason))}
}
