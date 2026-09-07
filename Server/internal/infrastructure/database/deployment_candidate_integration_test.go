//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestDeploymentRequestCreatedFromArgoCDRevisionTransaction(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 4, MaxIdleConnections: 2, ConnectionLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	execDeploymentSQL(t, db, `INSERT INTO application_onboardings (application_id, status, managed_at) VALUES (?, 'Managed', now())`, ids.applicationID)
	repository, err := deployinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	targets, err := repository.ListCandidateTargets(ctx)
	if err != nil || len(targets) != 1 {
		t.Fatalf("list candidate targets: %#v %v", targets, err)
	}
	observation := candidateObservationFixture(t, targets[0].ApplicationID, ids)
	created, err := repository.CreateDeploymentRequest(ctx, observation)
	if err != nil || !created {
		t.Fatalf("create deployment request: %v %v", created, err)
	}
	startPendingCandidateWorkflow(t, ctx, db, repository)
	created, err = repository.CreateDeploymentRequest(ctx, observation)
	if err != nil || created {
		t.Fatalf("duplicate candidate must be idempotent: %v %v", created, err)
	}
	for table, want := range map[string]int64{
		"deployment_candidate_observations": 1,
		"deployment_requests":               1,
		"deployment_request_versions":       1,
		"deployment_request_applications":   1,
		"deployment_request_images":         1,
		"deployment_workflow_instances":     1,
		"audit_logs":                        2,
		"outbox_events":                     2,
	} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s count = %d, %v; want %d", table, count, err, want)
		}
	}
	if err := db.Exec(`UPDATE deployment_candidate_observations SET target_revision = 'changed'`).Error; err == nil {
		t.Fatal("candidate observations must be immutable")
	}
}

func startPendingCandidateWorkflow(t *testing.T, ctx context.Context, db *gorm.DB, candidates *deployinfra.CandidateRepository) {
	t.Helper()
	pending, err := candidates.ListPendingWorkflowStarts(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("list pending Workflow starts: %#v %v", pending, err)
	}
	runtimeRepository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		t.Fatalf("create Workflow runtime repository: %v", err)
	}
	assignments, err := deployinfra.NewReviewAssignmentRepository(db)
	if err != nil {
		t.Fatalf("create review assignment repository: %v", err)
	}
	service, err := deployapp.NewWorkflowRuntimeService(deployapp.WorkflowRuntimeServiceOptions{
		Repository: runtimeRepository, Authorizer: allowWorkflowAuthorizer{},
		Assignments: assignments, Clock: deployapp.SystemWorkflowClock{},
	})
	if err != nil {
		t.Fatalf("create Workflow runtime service: %v", err)
	}
	if _, err := service.Start(ctx, deployapp.WorkflowStartInput{
		RequestVersionID: pending[0].RequestVersionID, IdempotencyKey: "candidate-start-test", RequestID: "candidate-start-test",
	}); err != nil {
		t.Fatalf("start pending Workflow: %v", err)
	}
	pending, err = candidates.ListPendingWorkflowStarts(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("Workflow start was not reconciled: %#v %v", pending, err)
	}
}

type allowWorkflowAuthorizer struct{}

func (allowWorkflowAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return true, nil
}

func TestNewRevisionCreatesRequestVersion(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 4, MaxIdleConnections: 2, ConnectionLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, err := deployinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	first := candidateObservationFixture(t, ids.applicationID, ids)
	if created, createErr := repository.CreateDeploymentRequest(ctx, first); createErr != nil || !created {
		t.Fatalf("create first candidate: %v %v", created, createErr)
	}
	second := nextCandidateObservation(t, first)
	if created, createErr := repository.CreateDeploymentRequest(ctx, second); createErr != nil || !created {
		t.Fatalf("create second candidate: %v %v", created, createErr)
	}
	var versions []struct {
		VersionNumber int
		Status        string
	}
	if err := db.Table("deployment_request_versions").Order("version_number").Find(&versions).Error; err != nil {
		t.Fatalf("load request versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Status != "Superseded" || versions[1].Status != "Candidate" {
		t.Fatalf("version lifecycle = %#v", versions)
	}
	for table, want := range map[string]int64{
		"deployment_requests": 1, "deployment_request_versions": 2,
		"deployment_candidate_observations": 2, "deployment_request_applications": 2,
	} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s count = %d, %v; want %d", table, count, err, want)
		}
	}
}

func TestDeploymentRequestUsesAccumulatedDiff(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 4, MaxIdleConnections: 2, ConnectionLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	workerID := seedCandidateApplication(t, db, ids, "worker")
	repository, err := deployinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	api := candidateObservationFixture(t, ids.applicationID, ids)
	worker := candidateObservationForApplication(t, workerID, "worker", api)
	if created, createErr := repository.CreateDeploymentRequest(ctx, api); createErr != nil || !created {
		t.Fatalf("create api candidate: %v %v", created, createErr)
	}
	if created, createErr := repository.CreateDeploymentRequest(ctx, worker); createErr != nil || !created {
		t.Fatalf("create worker candidate: %v %v", created, createErr)
	}
	assertAccumulatedRequest(t, db)
}

func TestReviewedOptionalFieldsCreateNewRequestVersion(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 4, MaxIdleConnections: 2, ConnectionLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	candidates, err := deployinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	if created, createErr := candidates.CreateDeploymentRequest(ctx, candidateObservationFixture(t, ids.applicationID, ids)); createErr != nil || !created {
		t.Fatalf("create candidate: %v %v", created, createErr)
	}
	requests, err := deployinfra.NewDeploymentRequestRepository(db)
	if err != nil {
		t.Fatalf("create request repository: %v", err)
	}
	var request struct{ ID uuid.UUID }
	if err := db.Table("deployment_requests").Select("id").Scan(&request).Error; err != nil {
		t.Fatalf("load request id: %v", err)
	}
	detail, err := requests.Load(ctx, request.ID)
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	scheduled := detail.Version.CreatedAt.Add(time.Hour)
	updated, err := requests.SupersedeMetadata(ctx, deployapp.RequestMutation{ActorID: ids.userID, RequestID: "metadata-test", OccurredAt: scheduled}, deployapp.MetadataVersionChange{
		RequestID: request.ID, RequestVersion: detail.Version.ID, ExpectedVersion: detail.Version.LockVersion,
		Metadata: deploydomain.DeploymentRequestMetadata{ChangeDescription: "Release note", IssueURL: "https://example.test/change/1", ScheduledFor: &scheduled},
	})
	if err != nil {
		t.Fatalf("supersede request metadata: %v", err)
	}
	if updated.Version.VersionNumber != 2 || updated.Version.Status != deploydomain.DeploymentRequestCandidate || len(updated.Applications) != 1 || len(updated.Applications[0].Images) != 1 {
		t.Fatalf("metadata replacement = %#v", updated)
	}
	var previousStatus string
	if err := db.Table("deployment_request_versions").Where("id = ?", detail.Version.ID).Select("status").Scan(&previousStatus).Error; err != nil {
		t.Fatalf("load superseded status: %v", err)
	}
	if previousStatus != string(deploydomain.DeploymentRequestSuperseded) {
		t.Fatalf("previous status = %s", previousStatus)
	}
}

func TestNewRevisionWaitsForActiveDeployment(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 4, MaxIdleConnections: 2, ConnectionLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, err := deployinfra.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("create candidate repository: %v", err)
	}
	first := candidateObservationFixture(t, ids.applicationID, ids)
	if created, createErr := repository.CreateDeploymentRequest(ctx, first); createErr != nil || !created {
		t.Fatalf("create first candidate: %v %v", created, createErr)
	}
	if err := db.Table("deployment_request_versions").Where("request_id IN (SELECT id FROM deployment_requests)").Update("status", "Deploying").Error; err != nil {
		t.Fatalf("mark request deploying: %v", err)
	}
	if created, createErr := repository.CreateDeploymentRequest(ctx, nextCandidateObservation(t, first)); createErr != nil || !created {
		t.Fatalf("create next candidate: %v %v", created, createErr)
	}
	var count int64
	if err := db.Table("deployment_requests").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("request count = %d, %v; want 2", count, err)
	}
	var deploying int64
	if err := db.Table("deployment_request_versions").Where("status = ?", "Deploying").Count(&deploying).Error; err != nil || deploying != 1 {
		t.Fatalf("deploying versions = %d, %v; want 1", deploying, err)
	}
	var waiting int64
	if err := db.Table("deployment_request_versions").Where("status = ?", "Candidate").Count(&waiting).Error; err != nil || waiting != 1 {
		t.Fatalf("waiting candidate versions = %d, %v; want 1", waiting, err)
	}
}

func nextCandidateObservation(t *testing.T, previous deploydomain.CandidateObservation) deploydomain.CandidateObservation {
	t.Helper()
	previous.TargetRevision = "commit-c"
	previous.TargetRevisions = []string{"commit-c"}
	previous.ManifestHash = repeatHex('f')
	previous.DiffHash = repeatHex('1')
	previous.Images[0].Digest = "sha256:" + repeatHex('2')
	previous.ObservedAt = previous.ObservedAt.Add(time.Minute)
	value, err := deploydomain.NewCandidateObservation(previous)
	if err != nil {
		t.Fatalf("create next candidate observation: %v", err)
	}
	return value
}

func seedCandidateApplication(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, key string) uuid.UUID {
	t.Helper()
	applicationID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO applications (
			id, organization_id, project_id, environment_id, name,
			argocd_namespace, argocd_application_name, argocd_project,
			destination_server, destination_namespace,
			source_repo_url, source_target_revision, source_path
		) VALUES (?, ?, ?, ?, ?, 'argocd', ?, 'api',
			'https://kubernetes.default.svc', ?, 'https://git.example/gitops.git', 'main', ?)
	`, applicationID, ids.organizationID, ids.projectID, ids.environmentID,
		key, key+"-production", key, "production/"+key)
	return applicationID
}

func candidateObservationForApplication(t *testing.T, applicationID uuid.UUID, key string, previous deploydomain.CandidateObservation) deploydomain.CandidateObservation {
	t.Helper()
	previous.ID, previous.Fingerprint = uuid.Nil, ""
	previous.ApplicationID, previous.ApplicationKey = applicationID, key
	previous.ManifestHash, previous.DiffHash = repeatHex('1'), repeatHex('2')
	previous.Diffs[0].Name, previous.Diffs[0].Namespace = key, key
	previous.Images[0].Repository = "platform/" + key
	previous.Images[0].ImageReference = previous.Images[0].Registry + "/platform/" + key + ":v1"
	previous.Images[0].Digest = "sha256:" + repeatHex('3')
	previous.Sources[0].Path = "production/" + key
	value, err := deploydomain.NewCandidateObservation(previous)
	if err != nil {
		t.Fatalf("create candidate observation for %s: %v", key, err)
	}
	return value
}

func assertAccumulatedRequest(t *testing.T, db *gorm.DB) {
	t.Helper()
	var requests, versions int64
	if err := db.Table("deployment_requests").Count(&requests).Error; err != nil {
		t.Fatalf("count deployment requests: %v", err)
	}
	if err := db.Table("deployment_request_versions").Count(&versions).Error; err != nil {
		t.Fatalf("count deployment request versions: %v", err)
	}
	var applications []struct {
		ApplicationKey string
		ExecutionOrder int
	}
	err := db.Raw(`
		SELECT application_key, execution_order
		FROM deployment_request_applications
		WHERE request_version_id = (SELECT id FROM deployment_request_versions WHERE status = 'Candidate')
		ORDER BY execution_order
	`).Scan(&applications).Error
	if err != nil {
		t.Fatalf("load accumulated Applications: %v", err)
	}
	if requests != 1 || versions != 2 || len(applications) != 2 || applications[0].ApplicationKey != "api" || applications[1].ApplicationKey != "worker" {
		t.Fatalf("accumulated request = requests:%d versions:%d Applications:%#v", requests, versions, applications)
	}
}

func candidateObservationFixture(t *testing.T, applicationID uuid.UUID, ids deploymentSchemaIDs) deploydomain.CandidateObservation {
	t.Helper()
	value, err := deploydomain.NewCandidateObservation(deploydomain.CandidateObservation{
		ApplicationID: applicationID, OrganizationID: ids.organizationID, ProjectID: ids.projectID, EnvironmentID: ids.environmentID,
		ApplicationKey: "api", WorkflowVersionID: ids.workflowVersionID, PlanVersionID: ids.planVersionID,
		TargetRevision: "commit-b", TargetRevisions: []string{"commit-b"},
		ManifestHash: repeatHex('a'), DiffHash: repeatHex('b'),
		Diffs:   []deploydomain.ResourceDiffEvidence{{Kind: "Deployment", Namespace: "api", Name: "api", NormalizedLiveHash: repeatHex('c'), PredictedLiveHash: repeatHex('d')}},
		Sources: []deploydomain.SourceEvidence{{RepositoryURL: "https://git.example/central.git", TargetRevision: "main", Path: "production/api"}},
		Images: []deploydomain.ImageSnapshot{{
			ImageReference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1", Registry: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com",
			Repository: "platform/api", Tag: "v1", Digest: "sha256:" + repeatHex('e'),
		}},
		ObservedAt: time.Date(2026, 9, 3, 2, 3, 4, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create candidate observation fixture: %v", err)
	}
	return value
}

func repeatHex(character byte) string {
	value := make([]byte, 64)
	for index := range value {
		value[index] = character
	}
	return string(value)
}
