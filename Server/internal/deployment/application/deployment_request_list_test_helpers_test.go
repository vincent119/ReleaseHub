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

type requestListRepository struct {
	capabilityRequestRepository
	values []deploydomain.DeploymentRequestSummary
	err    error
	calls  int
	ctx    context.Context
	scope  authz.Scope
	filter DeploymentRequestListFilter
}

func (r *requestListRepository) List(ctx context.Context, scope authz.Scope, filter DeploymentRequestListFilter) ([]deploydomain.DeploymentRequestSummary, error) {
	r.calls++
	r.ctx, r.scope, r.filter = ctx, scope, filter
	return r.values, r.err
}

type requestListScheduleReader struct {
	requestScheduleReader
	calls         int
	ctx           context.Context
	environmentID uuid.UUID
}

func (r *requestListScheduleReader) Get(ctx context.Context, environmentID uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	r.calls++
	r.ctx, r.environmentID = ctx, environmentID
	return r.requestScheduleReader.Get(ctx, environmentID)
}

type requestListAuthorizer struct {
	capabilityAuthorizer
	ctx context.Context
	err error
}

func (a *requestListAuthorizer) AuthorizeFresh(ctx context.Context, request authz.AuthorizationRequest) (bool, error) {
	a.ctx = ctx
	allowed, err := a.capabilityAuthorizer.AuthorizeFresh(ctx, request)
	if a.err != nil {
		return false, a.err
	}
	return allowed, err
}

func requestListTestScope(t *testing.T) authz.Scope {
	t.Helper()
	detail := capabilityRequestFixture()
	scope, err := authz.NewEnvironmentScope(detail.Summary.OrganizationID, detail.Summary.ProjectID, detail.Summary.EnvironmentID)
	require.NoError(t, err)
	return scope
}

func requestListTestValues(scope authz.Scope, count int) []deploydomain.DeploymentRequestSummary {
	values := make([]deploydomain.DeploymentRequestSummary, count)
	for index := range values {
		values[index] = deploydomain.DeploymentRequestSummary{
			ID: uuid.New(), OrganizationID: scope.OrganizationID,
			ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID,
			Title: "Release", Status: deploydomain.DeploymentRequestApproved,
			UpdatedAt: time.Date(2026, 10, 1, 2, 3, 4, 987654321, time.UTC).Add(-time.Duration(index) * time.Second),
		}
	}
	return values
}

func requestListTestService(t *testing.T, repository *requestListRepository, schedules *requestListScheduleReader, authorizer *requestListAuthorizer) *DeploymentRequestService {
	t.Helper()
	service, err := NewDeploymentRequestService(repository, schedules, authorizer,
		scheduleClock{now: time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	return service
}

func requestListAllowedAuthorizer() *requestListAuthorizer {
	return &requestListAuthorizer{capabilityAuthorizer: capabilityAuthorizer{
		allowed: map[authz.Permission]bool{"deployment_request.view": true},
	}}
}

func requestListInt(value int) *int { return &value }

func requestListString(value string) *string { return &value }
