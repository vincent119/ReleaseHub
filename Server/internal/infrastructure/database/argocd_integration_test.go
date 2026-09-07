//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestCandidateAssignmentCreatesOneProductionApplication(t *testing.T) {
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
	catalogRepository, _ := cataloginfra.NewCatalogRepository(db)
	mutation := catalogapp.Mutation{RequestID: "candidate-assignment"}
	organization := mustOrganization(t, "candidate-tenant")
	project := mustProject(t, organization.ID, "payment")
	environment := mustEnvironment(t, organization.ID, project.ID, "production", catalog.EnvironmentProduction)
	development := mustEnvironment(t, organization.ID, project.ID, "development", catalog.EnvironmentDevelopment)
	mustCreate(t, catalogRepository.CreateOrganization(ctx, mutation, organization))
	mustCreate(t, catalogRepository.CreateProject(ctx, mutation, project))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, environment))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, development))
	reconciliationRepository, _ := argoinfra.NewReconciliationRepository(db)
	observed := mustObservedApplication(t, "new-payment-production", "https://git.example.com/manifests.git", "production/new-payment", true)
	if _, err := reconciliationRepository.Reconcile(ctx, []argodomain.Application{observed}, time.Now().UTC()); err != nil {
		t.Fatalf("persist candidate: %v", err)
	}
	candidateRepository, _ := argoinfra.NewCandidateRepository(db)
	scopes, err := candidateRepository.ListAssignmentScopes(ctx)
	if err != nil || len(scopes) != 1 || scopes[0].EnvironmentID != environment.ID {
		t.Fatalf("candidate assignment scopes = %#v, error = %v", scopes, err)
	}
	queue, err := candidateRepository.ListPresent(ctx)
	if err != nil || len(queue) != 1 {
		t.Fatalf("candidate queue = %#v, error = %v", queue, err)
	}
	candidateID := queue[0].ID
	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, 'candidate-admin')`, actorID).Error; err != nil {
		t.Fatalf("create candidate administrator: %v", err)
	}
	service, err := argoapp.NewCandidateService(candidateRepository, allowOnboardingAuthorizer{})
	if err != nil {
		t.Fatalf("create candidate service: %v", err)
	}
	assigned, err := service.Assign(ctx, argoapp.OnboardingPrincipal{UserID: actorID}, argoapp.AssignCandidateInput{CandidateID: queue[0].ID, ExpectedVersion: queue[0].Version, OrganizationID: organization.ID, ProjectID: project.ID, EnvironmentID: environment.ID, ApplicationName: "new-payment", Source: catalog.GitOpsSource{RepositoryURL: observed.Sources[0].RepositoryURL, TargetRevision: observed.Sources[0].TargetRevision, Path: observed.Sources[0].Path}, RequestID: "assign-candidate"})
	if err != nil {
		t.Fatalf("assign candidate: %v", err)
	}
	queue, err = candidateRepository.ListPresent(ctx)
	if err != nil || len(queue) != 0 {
		t.Fatalf("assigned candidate remained visible: %#v %v", queue, err)
	}
	var linkedCandidateID string
	if err := db.Table("applications").Select("candidate_id::text").Where("id = ?", assigned.ID).Scan(&linkedCandidateID).Error; err != nil || linkedCandidateID != candidateID.String() {
		t.Fatalf("candidate link = %s, error = %v", linkedCandidateID, err)
	}
}

func TestArgoCDOnboardingSupportsRepositoryTopologies(t *testing.T) {
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

	catalogRepository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		t.Fatalf("create catalog repository: %v", err)
	}
	mutation := catalogapp.Mutation{RequestID: "argocd-reconciliation-integration"}
	organization := mustOrganization(t, "reconciliation-tenant")
	project := mustProject(t, organization.ID, "payment")
	environment := mustEnvironment(t, organization.ID, project.ID, "production", catalog.EnvironmentProduction)
	mustCreate(t, catalogRepository.CreateOrganization(ctx, mutation, organization))
	mustCreate(t, catalogRepository.CreateProject(ctx, mutation, project))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, environment))
	independent := mustApplication(t, organization.ID, project.ID, environment.ID, "api", "payment-api-production", "https://git.example.com/payment/api.git", "deploy/production")
	shared := mustApplication(t, organization.ID, project.ID, environment.ID, "worker", "payment-worker-production", "https://git.example.com/platform/manifests.git", "production/payment-worker")
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, independent))
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, shared))

	repository, err := argoinfra.NewReconciliationRepository(db)
	if err != nil {
		t.Fatalf("create reconciliation repository: %v", err)
	}
	observedAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	values := []argodomain.Application{
		mustObservedApplication(t, independent.Argo.Name, independent.Source.RepositoryURL, independent.Source.Path, true),
		mustObservedApplication(t, shared.Argo.Name, shared.Source.RepositoryURL, shared.Source.Path, false),
		mustUnmanagedApplication(t),
	}
	result, err := repository.Reconcile(ctx, values, observedAt)
	if err != nil {
		t.Fatalf("persist reconciliation: %v", err)
	}
	if result != (argoapp.Result{ObservedApplications: 3, Candidates: 2, TrackedApplications: 2, NewCandidates: 2}) {
		t.Fatalf("unexpected reconciliation result: %#v", result)
	}

	assertCount(t, db, "argocd_application_candidates", 2)
	assertCount(t, db, "argocd_application_snapshots", 2)
	assertCount(t, db, "audit_logs", 7)
	assertCount(t, db, "outbox_events", 7)
	candidateRepository, err := argoinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	queue, err := candidateRepository.ListPresent(ctx)
	if err != nil || len(queue) != 0 {
		t.Fatalf("already mapped Applications leaked into candidate queue: %#v %v", queue, err)
	}

	var sourcePaths []string
	if err := db.Raw(`SELECT source->>'path' FROM argocd_application_candidates, jsonb_array_elements(sources) source ORDER BY source->>'path'`).Scan(&sourcePaths).Error; err != nil {
		t.Fatalf("read candidate source topology: %v", err)
	}
	if len(sourcePaths) != 2 || sourcePaths[0] != "deploy/production" || sourcePaths[1] != "production/payment-worker" {
		t.Fatalf("GitOps source paths were not preserved: %#v", sourcePaths)
	}

	for index := range values[:2] {
		values[index].Labels = map[string]string{"releasehub.io/managed": "false"}
	}
	result, err = repository.Reconcile(ctx, values, observedAt.Add(30*time.Second))
	if err != nil {
		t.Fatalf("persist label removal: %v", err)
	}
	if result.Candidates != 0 || result.TrackedApplications != 2 || result.NewCandidates != 0 {
		t.Fatalf("label removal result mismatch: %#v", result)
	}
	var missingCandidates, snapshotsWithoutLabel int64
	if err := db.Table("argocd_application_candidates").Where("discovery_status = 'missing' AND missing_since IS NOT NULL").Count(&missingCandidates).Error; err != nil {
		t.Fatalf("count missing candidates: %v", err)
	}
	if err := db.Table("argocd_application_snapshots").Where("NOT managed_label_present AND missing_since IS NULL").Count(&snapshotsWithoutLabel).Error; err != nil {
		t.Fatalf("count tracked label drift observations: %v", err)
	}
	if missingCandidates != 2 || snapshotsWithoutLabel != 2 {
		t.Fatalf("label drift observation mismatch: candidates=%d snapshots=%d", missingCandidates, snapshotsWithoutLabel)
	}
	assertCount(t, db, "audit_logs", 7)
	assertCount(t, db, "outbox_events", 7)
}

func mustObservedApplication(t *testing.T, name, repositoryURL, path string, automated bool) argodomain.Application {
	t.Helper()
	value, err := argodomain.NewApplication(argodomain.Application{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: name},
		Project:  "payment", Labels: map[string]string{argodomain.ManagedLabelKey: argodomain.ManagedLabelValue},
		Sources:       []argodomain.Source{{RepositoryURL: repositoryURL, TargetRevision: "main", Path: path}},
		Destination:   argodomain.Destination{Server: "https://kubernetes.default.svc", Namespace: "payment"},
		AutomatedSync: automated, SyncStatus: "OutOfSync", HealthStatus: "Healthy", OperationPhase: "Succeeded",
		ResolvedRevision: "commit-a", ResolvedRevisions: []string{"commit-a"}, ResourceVersion: "42",
	})
	if err != nil {
		t.Fatalf("create observed Application: %v", err)
	}
	return value
}

func mustUnmanagedApplication(t *testing.T) argodomain.Application {
	t.Helper()
	value, err := argodomain.NewApplication(argodomain.Application{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "unmanaged"},
		Labels:   map[string]string{argodomain.ManagedLabelKey: "True"},
	})
	if err != nil {
		t.Fatalf("create unmanaged Application observation: %v", err)
	}
	return value
}
