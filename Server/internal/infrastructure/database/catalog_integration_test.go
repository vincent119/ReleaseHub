//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestResourceCatalogTenantBoundariesAndMappings(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 5, MaxIdleConnections: 2})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	repository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		t.Fatalf("create catalog repository: %v", err)
	}
	mutation := application.Mutation{RequestID: "catalog-integration"}

	var defaultOrganizationCount int64
	if err := db.Table("organizations").Where("id = ?", "00000000-0000-0000-0000-000000000001").Count(&defaultOrganizationCount).Error; err != nil || defaultOrganizationCount != 1 {
		t.Fatalf("single-tenant default Organization must remain in the data model: %d %v", defaultOrganizationCount, err)
	}

	organizationA := mustOrganization(t, "tenant-a")
	organizationB := mustOrganization(t, "tenant-b")
	mustCreate(t, repository.CreateOrganization(ctx, mutation, organizationA))
	mustCreate(t, repository.CreateOrganization(ctx, mutation, organizationB))
	projectA := mustProject(t, organizationA.ID, "payment")
	projectB := mustProject(t, organizationB.ID, "payment")
	mustCreate(t, repository.CreateProject(ctx, mutation, projectA))
	mustCreate(t, repository.CreateProject(ctx, mutation, projectB))

	devops := mustEnvironment(t, organizationA.ID, projectA.ID, "devops", catalog.EnvironmentProduction)
	uat := mustEnvironment(t, organizationA.ID, projectA.ID, "uat-tw", catalog.EnvironmentTesting)
	otherTenantEnvironment := mustEnvironment(t, organizationB.ID, projectB.ID, "devops", catalog.EnvironmentProduction)
	mustCreate(t, repository.CreateEnvironment(ctx, mutation, devops))
	mustCreate(t, repository.CreateEnvironment(ctx, mutation, uat))
	mustCreate(t, repository.CreateEnvironment(ctx, mutation, otherTenantEnvironment))

	if err := db.Exec(`INSERT INTO environments (id, organization_id, project_id, name, environment_type) VALUES (?, ?, ?, 'invalid-tenant', 'Testing')`, uuid.New(), organizationB.ID, projectA.ID).Error; err == nil {
		t.Fatal("database should reject a cross-tenant Project ancestry")
	}
	if err := db.Exec(`INSERT INTO environments (id, organization_id, project_id, name, environment_type) VALUES (?, ?, ?, 'invalid-type', 'UAT')`, uuid.New(), organizationA.ID, projectA.ID).Error; err == nil {
		t.Fatal("database should reject an Environment type outside the fixed set")
	}

	sharedRepoApplication := mustApplication(t, organizationA.ID, projectA.ID, devops.ID, "admin", "admin-tenant-a", "https://git.example.com/platform/manifests.git", "production/admin")
	independentRepoApplication := mustApplication(t, organizationB.ID, projectB.ID, otherTenantEnvironment.ID, "admin", "admin-tenant-b", "https://git.example.com/payment/admin.git", "deploy/production")
	mustCreate(t, repository.CreateApplication(ctx, mutation, sharedRepoApplication))
	mustCreate(t, repository.CreateApplication(ctx, mutation, independentRepoApplication))

	mapping, err := catalog.NewEnvironmentLabelMapping(organizationA.ID, projectA.ID, devops.ID, "env", "Global")
	if err != nil {
		t.Fatalf("create Environment label mapping: %v", err)
	}
	mustCreate(t, repository.CreateEnvironmentLabelMapping(ctx, mutation, mapping))
	resolved, err := repository.ResolveEnvironmentLabel(ctx, organizationA.ID, projectA.ID, "env", "Global")
	if err != nil || resolved.ID != devops.ID || resolved.Name != "devops" {
		t.Fatalf("Argo CD label should resolve through explicit mapping: %#v %v", resolved, err)
	}
	if _, err := repository.ResolveEnvironmentLabel(ctx, organizationA.ID, projectA.ID, "env", "global"); err == nil {
		t.Fatal("Kubernetes label mapping should remain case-sensitive")
	}

	service, err := application.NewService(repository, catalogAuthorizer{organizationID: organizationA.ID})
	if err != nil {
		t.Fatalf("create catalog service: %v", err)
	}
	principal := application.Principal{UserID: uuid.New()}
	if _, err := service.FindApplication(ctx, principal, sharedRepoApplication.ID); err != nil {
		t.Fatalf("authorized tenant Application should be visible: %v", err)
	}
	if _, err := service.FindApplication(ctx, principal, independentRepoApplication.ID); !errors.Is(err, application.ErrResourceNotFound) {
		t.Fatalf("cross-tenant direct lookup should not reveal the Application: %v", err)
	}
	if values, err := service.ListApplications(ctx, principal, organizationB.ID, projectB.ID, otherTenantEnvironment.ID); err != nil || len(values) != 0 {
		t.Fatalf("cross-tenant list should be empty: %#v %v", values, err)
	}

	assertCount(t, db, "audit_logs", 10)
	assertCount(t, db, "outbox_events", 10)
}

func mustOrganization(t *testing.T, name string) catalog.Organization {
	t.Helper()
	value, err := catalog.NewOrganization(name)
	if err != nil {
		t.Fatalf("create Organization: %v", err)
	}
	return value
}

func mustProject(t *testing.T, organizationID uuid.UUID, name string) catalog.Project {
	t.Helper()
	value, err := catalog.NewProject(organizationID, name)
	if err != nil {
		t.Fatalf("create Project: %v", err)
	}
	return value
}

func mustEnvironment(t *testing.T, organizationID, projectID uuid.UUID, name string, environmentType catalog.EnvironmentType) catalog.Environment {
	t.Helper()
	value, err := catalog.NewEnvironment(organizationID, projectID, name, environmentType)
	if err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	return value
}

func mustApplication(t *testing.T, organizationID, projectID, environmentID uuid.UUID, name, argoName, repositoryURL, sourcePath string) catalog.Application {
	t.Helper()
	value, err := catalog.NewApplication(
		organizationID, projectID, environmentID, name,
		catalog.ArgoApplicationIdentity{Namespace: "argocd", Name: argoName},
		name, "https://kubernetes.default.svc", name,
		catalog.GitOpsSource{RepositoryURL: repositoryURL, TargetRevision: "main", Path: sourcePath},
	)
	if err != nil {
		t.Fatalf("create Application: %v", err)
	}
	return value
}

func mustCreate(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("persist catalog resource: %v", err)
	}
}

type catalogAuthorizer struct{ organizationID uuid.UUID }

func (a catalogAuthorizer) Authorize(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return request.Scope.OrganizationID == a.organizationID, nil
}

func (a catalogAuthorizer) AuthorizeFresh(ctx context.Context, request authz.AuthorizationRequest) (bool, error) {
	return a.Authorize(ctx, request)
}
