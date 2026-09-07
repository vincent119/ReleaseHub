package application

import (
	"context"
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
	service, err := NewDeploymentRequestService(repository, authorizer, SystemWorkflowClock{})
	require.NoError(t, err)

	result, err := service.Get(context.Background(), RequestPrincipal{UserID: uuid.New()}, detail.Summary.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"deployment_request.review"}, result.Capabilities)
}

type capabilityRequestRepository struct{ detail deploydomain.DeploymentRequestDetail }

func (s *capabilityRequestRepository) List(context.Context, authz.Scope) ([]deploydomain.DeploymentRequestSummary, error) {
	return []deploydomain.DeploymentRequestSummary{s.detail.Summary}, nil
}

func (s *capabilityRequestRepository) Load(context.Context, uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	return s.detail, nil
}

func (s *capabilityRequestRepository) SupersedeMetadata(context.Context, RequestMutation, MetadataVersionChange) (deploydomain.DeploymentRequestDetail, error) {
	return s.detail, nil
}

type capabilityAuthorizer struct{ allowed map[authz.Permission]bool }

func (s *capabilityAuthorizer) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return s.allowed[request.Permission], nil
}

func capabilityRequestFixture() deploydomain.DeploymentRequestDetail {
	requestID := uuid.New()
	return deploydomain.DeploymentRequestDetail{
		Summary: deploydomain.DeploymentRequestSummary{ID: requestID, OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New()},
		Version: deploydomain.DeploymentRequestVersionSummary{ID: uuid.New(), RequestID: requestID, CreatedAt: time.Now().UTC()},
	}
}
