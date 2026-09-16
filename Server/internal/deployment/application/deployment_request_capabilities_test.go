package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentRequestProjectsCurrentCapabilities(t *testing.T) {
	detail := capabilityRequestFixture()
	repository := &capabilityRequestRepository{detail: detail}
	authorizer := &capabilityAuthorizer{allowed: map[authz.Permission]bool{
		"deployment_request.view": true, "deployment_request.review": true,
	}}
	service, err := NewDeploymentRequestService(repository, &requestScheduleReader{}, authorizer, SystemWorkflowClock{})
	require.NoError(t, err)

	result, err := service.Get(context.Background(), RequestPrincipal{UserID: uuid.New()}, detail.Summary.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"deployment_request.review"}, result.Capabilities)
}

func TestDeploymentRequestProjectsEveryAllowedCapability(t *testing.T) {
	detail := capabilityRequestFixture()
	repository := &capabilityRequestRepository{detail: detail}
	allowed := map[authz.Permission]bool{"deployment_request.view": true}
	for _, key := range deploymentRequestCapabilityKeys {
		allowed[authz.Permission(key)] = true
	}
	authorizer := &capabilityAuthorizer{allowed: allowed}
	service, err := NewDeploymentRequestService(repository, &requestScheduleReader{}, authorizer, SystemWorkflowClock{})
	require.NoError(t, err)

	result, err := service.Get(context.Background(), RequestPrincipal{UserID: uuid.New()}, detail.Summary.ID)
	require.NoError(t, err)
	require.Equal(t, deploymentRequestCapabilityKeys, result.Capabilities)
	require.Len(t, authorizer.requests, len(deploymentRequestCapabilityKeys)+1)
	for _, request := range authorizer.requests {
		require.Equal(t, authz.ScopeEnvironment, request.Scope.Kind)
		require.Equal(t, detail.Summary.EnvironmentID, request.Scope.EnvironmentID)
	}
}

func TestDeploymentRequestCommandRechecksPermissionAfterCapabilityProjection(t *testing.T) {
	detail := capabilityRequestFixture()
	repository := &capabilityRequestRepository{detail: detail}
	authorizer := &capabilityAuthorizer{allowed: map[authz.Permission]bool{
		"deployment_request.view": true, "deployment_request.update": true,
	}}
	service, err := NewDeploymentRequestService(repository, &requestScheduleReader{}, authorizer, SystemWorkflowClock{})
	require.NoError(t, err)
	principal := RequestPrincipal{UserID: uuid.New()}

	projected, err := service.Get(context.Background(), principal, detail.Summary.ID)
	require.NoError(t, err)
	require.Contains(t, projected.Capabilities, "deployment_request.update")

	authorizer.allowed["deployment_request.update"] = false
	_, err = service.UpdateMetadata(context.Background(), principal, MetadataVersionChange{
		RequestID: detail.Summary.ID, RequestVersion: detail.Version.ID,
		ExpectedVersion: detail.Version.LockVersion,
		Metadata:        deploydomain.DeploymentRequestMetadata{ChangeDescription: "updated after policy revision"},
	}, "AUTHZ-REQUEST-UPDATE-STALE-CAPABILITY")
	require.True(t, errors.Is(err, ErrRequestForbidden))
	require.Zero(t, repository.supersedeCalls)
}

type capabilityRequestRepository struct {
	detail         deploydomain.DeploymentRequestDetail
	supersedeCalls int
}

type requestScheduleReader struct {
	policy *deploydomain.DeploymentSchedulePolicy
	err    error
}

func (s *requestScheduleReader) Get(context.Context, uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	return s.policy, s.err
}

func (s *capabilityRequestRepository) List(context.Context, authz.Scope) ([]deploydomain.DeploymentRequestSummary, error) {
	return []deploydomain.DeploymentRequestSummary{s.detail.Summary}, nil
}

func (s *capabilityRequestRepository) Load(context.Context, uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	return s.detail, nil
}

func (s *capabilityRequestRepository) SupersedeMetadata(context.Context, RequestMutation, MetadataVersionChange) (deploydomain.DeploymentRequestDetail, error) {
	s.supersedeCalls++
	return s.detail, nil
}

type capabilityAuthorizer struct {
	allowed  map[authz.Permission]bool
	requests []authz.AuthorizationRequest
}

func (s *capabilityAuthorizer) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	s.requests = append(s.requests, request)
	if request.Disabled {
		return false, nil
	}
	return s.allowed[request.Permission], nil
}

func capabilityRequestFixture() deploydomain.DeploymentRequestDetail {
	requestID := uuid.New()
	return deploydomain.DeploymentRequestDetail{
		Summary: deploydomain.DeploymentRequestSummary{ID: requestID, OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New()},
		Version: deploydomain.DeploymentRequestVersionSummary{
			ID: uuid.New(), RequestID: requestID, Status: deploydomain.DeploymentRequestPendingReview,
			LockVersion: 1, CreatedAt: time.Now().UTC(),
		},
	}
}
