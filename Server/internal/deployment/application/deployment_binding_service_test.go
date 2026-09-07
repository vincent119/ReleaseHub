package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentBindingServiceUsesCurrentEnvironmentPermission(t *testing.T) {
	ids := bindingIDsFixture()
	repository := &bindingRepositoryStub{scope: ids.scope}
	service, err := NewDeploymentBindingService(repository, planAuthorizerStub{allowed: false}, planClockStub{})
	if err != nil {
		t.Fatalf("create binding service: %v", err)
	}
	_, err = service.Bind(context.Background(), PlanPrincipal{UserID: uuid.New()}, ids.input)
	if !errors.Is(err, ErrBindingForbidden) || repository.bound.ID != uuid.Nil {
		t.Fatalf("binding authorization error = %v", err)
	}
}

func TestDeploymentBindingServiceReturnsUnboundEnvironment(t *testing.T) {
	ids := bindingIDsFixture()
	service, err := NewDeploymentBindingService(&bindingRepositoryStub{scope: ids.scope}, planAuthorizerStub{allowed: true}, planClockStub{})
	if err != nil {
		t.Fatalf("create binding service: %v", err)
	}
	value, err := service.Get(context.Background(), PlanPrincipal{UserID: uuid.New()}, BindingScopeInput{
		OrganizationID: ids.scope.OrganizationID, ProjectID: ids.scope.ProjectID, EnvironmentID: ids.scope.EnvironmentID,
	})
	if err != nil || value != nil {
		t.Fatalf("unbound value = %#v, error = %v", value, err)
	}
}

type bindingFixture struct {
	scope authz.Scope
	input BindDefinitionsInput
}

func bindingIDsFixture() bindingFixture {
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	scope, _ := authz.NewEnvironmentScope(organizationID, projectID, environmentID)
	return bindingFixture{scope: scope, input: BindDefinitionsInput{
		OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID,
		WorkflowVersionID: uuid.New(), PlanVersionID: uuid.New(),
	}}
}

type bindingRepositoryStub struct {
	scope authz.Scope
	bound deploydomain.DeploymentBinding
}

func (s *bindingRepositoryStub) ResolveEnvironment(_ context.Context, input BindingScopeInput) (authz.Scope, error) {
	if input.EnvironmentID != s.scope.EnvironmentID {
		return authz.Scope{}, ErrBindingNotFound
	}
	return s.scope, nil
}

func (*bindingRepositoryStub) Get(context.Context, authz.Scope) (*deploydomain.DeploymentBinding, error) {
	return nil, nil
}

func (s *bindingRepositoryStub) Bind(_ context.Context, mutation BindingMutation, scope authz.Scope, input BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	value, err := deploydomain.NewDeploymentBinding(deploydomain.DeploymentBinding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID,
		EnvironmentID: scope.EnvironmentID, WorkflowVersionID: input.WorkflowVersionID,
		PlanVersionID: input.PlanVersionID, CreatedBy: mutation.ActorID, CreatedAt: mutation.OccurredAt,
	})
	s.bound = value
	return value, err
}
