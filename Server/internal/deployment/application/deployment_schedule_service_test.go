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

func TestDeploymentScheduleGetAllowsViewAndProjectsManagementCapability(t *testing.T) {
	repository := &scheduleRepositoryStub{scope: scheduleTestScope(), policy: scheduleTestPolicy(t)}
	authorizer := &scheduleAuthorizerStub{allowed: map[authz.Permission]bool{"deployment_request.view": true}}
	service := newScheduleService(t, repository, authorizer)

	view, err := service.Get(context.Background(), PlanPrincipal{UserID: uuid.New()}, repository.scope.EnvironmentID)
	require.NoError(t, err)
	require.False(t, view.CanManage)
	require.Equal(t, uint64(3), view.Policy.Version)
	require.Equal(t, []authz.Permission{"deployment_schedule.manage", "deployment_request.view"}, authorizer.calls)
}

func TestDeploymentScheduleMasksMissingScopeAndDeniedPrincipal(t *testing.T) {
	tests := []struct {
		name       string
		repository *scheduleRepositoryStub
		authorizer *scheduleAuthorizerStub
	}{
		{name: "missing scope", repository: &scheduleRepositoryStub{resolveErr: ErrDeploymentScheduleNotFound}, authorizer: &scheduleAuthorizerStub{}},
		{name: "explicit deny", repository: &scheduleRepositoryStub{scope: scheduleTestScope()}, authorizer: &scheduleAuthorizerStub{}},
		{name: "disabled principal", repository: &scheduleRepositoryStub{scope: scheduleTestScope()}, authorizer: &scheduleAuthorizerStub{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newScheduleService(t, tt.repository, tt.authorizer)
			_, err := service.Get(context.Background(), PlanPrincipal{UserID: uuid.New(), Disabled: tt.name == "disabled principal"}, uuid.New())
			require.ErrorIs(t, err, ErrDeploymentScheduleNotFound)
		})
	}
}

func TestDeploymentSchedulePutRequiresManageAndDelegatesOptimisticCommand(t *testing.T) {
	repository := &scheduleRepositoryStub{scope: scheduleTestScope()}
	authorizer := &scheduleAuthorizerStub{allowed: map[authz.Permission]bool{"deployment_schedule.manage": true}}
	service := newScheduleService(t, repository, authorizer)
	principal := PlanPrincipal{UserID: uuid.New()}

	view, err := service.Put(context.Background(), principal, UpdateDeploymentScheduleInput{
		EnvironmentID: repository.scope.EnvironmentID, Enabled: true, TimeZone: "UTC",
		WeeklyWindows:   []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: 60, EndMinute: 120}},
		ExpectedVersion: 3, IdempotencyKey: "schedule-1", RequestID: "request-1",
	})
	require.NoError(t, err)
	require.True(t, view.CanManage)
	require.Equal(t, uint64(4), repository.putPolicy.Version)
	require.Equal(t, uint64(3), repository.mutation.ExpectedVersion)
	require.Equal(t, principal.UserID, repository.mutation.ActorID)
	require.Equal(t, "schedule-1", repository.mutation.IdempotencyKey)
}

func TestDeploymentSchedulePutRejectsInvalidDocumentBeforePersistence(t *testing.T) {
	repository := &scheduleRepositoryStub{scope: scheduleTestScope()}
	service := newScheduleService(t, repository, &scheduleAuthorizerStub{allowed: map[authz.Permission]bool{"deployment_schedule.manage": true}})

	_, err := service.Put(context.Background(), PlanPrincipal{UserID: uuid.New()}, UpdateDeploymentScheduleInput{
		EnvironmentID: repository.scope.EnvironmentID, Enabled: true, TimeZone: "UTC+8", IdempotencyKey: "schedule-1",
	})
	require.ErrorIs(t, err, ErrDeploymentScheduleInvalid)
	require.Equal(t, uuid.Nil, repository.putPolicy.EnvironmentID)
}

func newScheduleService(t *testing.T, repository DeploymentScheduleRepository, authorizer WorkflowAuthorizer) *DeploymentScheduleService {
	t.Helper()
	service, err := NewDeploymentScheduleService(repository, authorizer, scheduleClock{now: time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	return service
}

func scheduleTestScope() authz.Scope {
	scope, err := authz.NewEnvironmentScope(uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		panic(err)
	}
	return scope
}

func scheduleTestPolicy(t *testing.T) *deploydomain.DeploymentSchedulePolicy {
	t.Helper()
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: uuid.New(), Enabled: true, TimeZone: "UTC", Version: 3,
		WeeklyWindows: []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: 60, EndMinute: 120}},
	})
	require.NoError(t, err)
	return &policy
}

type scheduleRepositoryStub struct {
	scope      authz.Scope
	resolveErr error
	policy     *deploydomain.DeploymentSchedulePolicy
	mutation   DeploymentScheduleMutation
	putPolicy  deploydomain.DeploymentSchedulePolicy
	putErr     error
}

func (s *scheduleRepositoryStub) ResolveEnvironment(context.Context, uuid.UUID) (authz.Scope, error) {
	return s.scope, s.resolveErr
}

func (s *scheduleRepositoryStub) Get(context.Context, uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	return s.policy, nil
}

func (s *scheduleRepositoryStub) Put(_ context.Context, mutation DeploymentScheduleMutation, policy deploydomain.DeploymentSchedulePolicy) (deploydomain.DeploymentSchedulePolicy, error) {
	s.mutation, s.putPolicy = mutation, policy
	if s.putErr != nil {
		return deploydomain.DeploymentSchedulePolicy{}, s.putErr
	}
	return policy, nil
}

type scheduleAuthorizerStub struct {
	allowed map[authz.Permission]bool
	calls   []authz.Permission
}

type scheduleClock struct{ now time.Time }

func (s scheduleClock) Now() time.Time { return s.now }

func (s *scheduleAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	s.calls = append(s.calls, request.Permission)
	return s.allowed[request.Permission] && !request.Disabled, nil
}
