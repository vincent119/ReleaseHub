package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

func TestTenantResourceIsolation(t *testing.T) {
	organizationA, organizationB := uuid.New(), uuid.New()
	projectA, projectB := uuid.New(), uuid.New()
	environmentA, environmentB := uuid.New(), uuid.New()
	applicationA := catalogApplication(organizationA, projectA, environmentA, "admin")
	applicationB := catalogApplication(organizationB, projectB, environmentB, "admin")
	repository := &repositoryStub{applications: map[uuid.UUID]domain.Application{applicationA.ID: applicationA, applicationB.ID: applicationB}}
	service, err := application.NewService(repository, authorizerStub{allowedOrganizationID: organizationA})
	if err != nil {
		t.Fatalf("create catalog service: %v", err)
	}
	principal := application.Principal{UserID: uuid.New()}

	if _, err := service.FindApplication(context.Background(), principal, applicationA.ID); err != nil {
		t.Fatalf("authorized Organization should be visible: %v", err)
	}
	if _, err := service.FindApplication(context.Background(), principal, applicationB.ID); !errors.Is(err, application.ErrResourceNotFound) {
		t.Fatalf("cross-tenant lookup should be indistinguishable from missing data, got %v", err)
	}
	if _, err := service.FindApplication(context.Background(), principal, uuid.New()); !errors.Is(err, application.ErrResourceNotFound) {
		t.Fatalf("missing lookup should use the same error, got %v", err)
	}
	visible, err := service.ListApplications(context.Background(), principal, organizationB, projectB, environmentB)
	if err != nil || len(visible) != 0 {
		t.Fatalf("unauthorized Environment list should be empty: %#v %v", visible, err)
	}
}

func TestVisibleApplicationsFiltersEachApplicationScope(t *testing.T) {
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	visible := catalogApplication(organizationID, projectID, environmentID, "api")
	hidden := catalogApplication(uuid.New(), uuid.New(), uuid.New(), "admin")
	service, err := application.NewService(&repositoryStub{applications: map[uuid.UUID]domain.Application{visible.ID: visible, hidden.ID: hidden}}, authorizerStub{allowedOrganizationID: organizationID})
	if err != nil {
		t.Fatalf("create catalog service: %v", err)
	}
	values, err := service.ListVisibleApplications(context.Background(), application.Principal{UserID: uuid.New()})
	if err != nil || len(values) != 1 || values[0].ID != visible.ID {
		t.Fatalf("visible Applications = %#v, %v", values, err)
	}
}

func TestResourceTreeIncludesOnlyAuthorizedAncestors(t *testing.T) {
	organizationA, organizationB := uuid.New(), uuid.New()
	projectA, projectB := uuid.New(), uuid.New()
	environmentA, environmentB := uuid.New(), uuid.New()
	applicationA := catalogApplication(organizationA, projectA, environmentA, "api")
	applicationB := catalogApplication(organizationB, projectB, environmentB, "admin")
	repository := &repositoryStub{
		organizations: []domain.Organization{{ID: organizationA, Name: "Tenant A", Active: true}, {ID: organizationB, Name: "Tenant B", Active: true}},
		projects:      []domain.Project{{ID: projectA, OrganizationID: organizationA, Name: "Payment", Active: true}, {ID: projectB, OrganizationID: organizationB, Name: "Admin", Active: true}},
		environments:  []domain.Environment{{ID: environmentA, OrganizationID: organizationA, ProjectID: projectA, Name: "production", Active: true}, {ID: environmentB, OrganizationID: organizationB, ProjectID: projectB, Name: "production", Active: true}},
		applications:  map[uuid.UUID]domain.Application{applicationA.ID: applicationA, applicationB.ID: applicationB},
	}
	service, err := application.NewService(repository, authorizerStub{allowedOrganizationID: organizationA})
	if err != nil {
		t.Fatalf("create catalog service: %v", err)
	}

	tree, err := service.ListResourceTree(context.Background(), application.Principal{UserID: uuid.New()})
	if err != nil {
		t.Fatalf("list resource tree: %v", err)
	}
	if len(tree.Organizations) != 1 || tree.Organizations[0].Organization.ID != organizationA || len(tree.Organizations[0].Projects) != 1 || len(tree.Organizations[0].Projects[0].Environments) != 1 {
		t.Fatalf("resource tree leaked or omitted ancestors: %#v", tree)
	}
}

func TestProjectManagerCreatesEnvironmentWithAuditedActor(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	repository := &repositoryStub{applications: map[uuid.UUID]domain.Application{}}
	service, err := application.NewService(repository, authorizerStub{allowedOrganizationID: organizationID})
	if err != nil {
		t.Fatalf("create catalog service: %v", err)
	}
	principal := application.Principal{UserID: uuid.New()}
	value, err := service.CreateEnvironment(context.Background(), principal, application.Mutation{RequestID: "create-environment"}, organizationID, projectID, "uat-tw", domain.EnvironmentTesting)
	if err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	if value.Name != "uat-tw" || repository.environmentCreates != 1 || repository.lastMutation.ActorID == nil || *repository.lastMutation.ActorID != principal.UserID {
		t.Fatalf("Environment mutation was not authorized and attributed: %#v %#v", value, repository.lastMutation)
	}
}

type repositoryStub struct {
	organizations      []domain.Organization
	projects           []domain.Project
	environments       []domain.Environment
	applications       map[uuid.UUID]domain.Application
	environmentCreates int
	lastMutation       application.Mutation
}

func (r *repositoryStub) ListActiveOrganizations(context.Context) ([]domain.Organization, error) {
	return r.organizations, nil
}
func (r *repositoryStub) ListActiveProjects(context.Context) ([]domain.Project, error) {
	return r.projects, nil
}
func (r *repositoryStub) ListActiveEnvironments(context.Context) ([]domain.Environment, error) {
	return r.environments, nil
}

func (*repositoryStub) CreateOrganization(context.Context, application.Mutation, domain.Organization) error {
	return nil
}
func (*repositoryStub) CreateProject(context.Context, application.Mutation, domain.Project) error {
	return nil
}

func (r *repositoryStub) CreateEnvironment(_ context.Context, mutation application.Mutation, _ domain.Environment) error {
	r.environmentCreates++
	r.lastMutation = mutation
	return nil
}
func (*repositoryStub) CreateApplication(context.Context, application.Mutation, domain.Application) error {
	return nil
}
func (*repositoryStub) CreateEnvironmentLabelMapping(context.Context, application.Mutation, domain.EnvironmentLabelMapping) error {
	return nil
}
func (r *repositoryStub) FindApplication(_ context.Context, id uuid.UUID) (domain.Application, error) {
	value, ok := r.applications[id]
	if !ok {
		return domain.Application{}, errors.New("not found")
	}
	return value, nil
}
func (r *repositoryStub) ListActiveApplications(_ context.Context) ([]domain.Application, error) {
	result := make([]domain.Application, 0, len(r.applications))
	for _, value := range r.applications {
		result = append(result, value)
	}
	return result, nil
}
func (r *repositoryStub) ListApplications(_ context.Context, organizationID, projectID, environmentID uuid.UUID) ([]domain.Application, error) {
	result := make([]domain.Application, 0)
	for _, value := range r.applications {
		if value.OrganizationID == organizationID && value.ProjectID == projectID && value.EnvironmentID == environmentID {
			result = append(result, value)
		}
	}
	return result, nil
}
func (*repositoryStub) ResolveEnvironmentLabel(context.Context, uuid.UUID, uuid.UUID, string, string) (domain.Environment, error) {
	return domain.Environment{}, nil
}

type authorizerStub struct{ allowedOrganizationID uuid.UUID }

func (a authorizerStub) Authorize(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return request.Scope.OrganizationID == a.allowedOrganizationID, nil
}

func (a authorizerStub) AuthorizeFresh(ctx context.Context, request authz.AuthorizationRequest) (bool, error) {
	return a.Authorize(ctx, request)
}

func catalogApplication(organizationID, projectID, environmentID uuid.UUID, name string) domain.Application {
	return domain.Application{ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID, Name: name}
}
