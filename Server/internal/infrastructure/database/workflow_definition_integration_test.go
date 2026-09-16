//go:build integration

package database_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestWorkflowDefinitionRepositoryPreservesVersionLifecycle(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, 'workflow-admin')`, actorID).Error; err != nil {
		t.Fatalf("seed workflow administrator: %v", err)
	}
	repository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}
	now := time.Date(2026, 9, 3, 3, 4, 5, 0, time.UTC)
	workflow := workflowAggregate(t, actorID, now)
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "workflow-integration", OccurredAt: now}
	if err := repository.Create(ctx, mutation, workflow); err != nil {
		t.Fatalf("persist workflow: %v", err)
	}
	assertDraftAppendConflict(t, repository, mutation, workflow, actorID, now)
	published := publishWorkflowVersion(t, workflow.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish workflow version: %v", err)
	}
	second := workflowVersion(t, workflow.ID, 2, actorID, now.Add(time.Minute))
	if err := repository.AppendVersion(ctx, mutation, 2, second); !errors.Is(err, deployapp.ErrWorkflowVersionConflict) {
		t.Fatalf("append workflow with stale version error = %v", err)
	}
	if err := repository.AppendVersion(ctx, mutation, 1, second); err != nil {
		t.Fatalf("append workflow version: %v", err)
	}
	loaded, err := repository.Load(ctx, workflow.ID)
	if err != nil || len(loaded.Versions) != 2 || loaded.Versions[0].Lifecycle != deploydomain.DefinitionPublished {
		t.Fatalf("loaded workflow = %#v, error = %v", loaded, err)
	}
	assertCountWhere(t, db, "audit_logs", "request_id = ?", mutation.RequestID, 3)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", workflow.ID.String(), 3)
}

func TestWorkflowDefinitionRepositoryMapsDuplicateNameConflict(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, 'workflow-name-admin')`, actorID).Error; err != nil {
		t.Fatalf("seed workflow administrator: %v", err)
	}
	repository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}
	now := time.Date(2026, 9, 15, 6, 0, 0, 0, time.UTC)
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "workflow-name-conflict", OccurredAt: now}
	if err := repository.Create(ctx, mutation, workflowAggregateNamed(t, actorID, now, "Production approval")); err != nil {
		t.Fatalf("create first workflow: %v", err)
	}

	err = repository.Create(ctx, mutation, workflowAggregateNamed(t, actorID, now, "production APPROVAL"))
	if !errors.Is(err, deployapp.ErrWorkflowNameConflict) {
		t.Fatalf("duplicate workflow name error = %v", err)
	}
}

func TestWorkflowDefinitionRepositoryRestrictsDeletionToUnusedDrafts(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}
	now := time.Date(2026, 9, 10, 4, 5, 6, 0, time.UTC)

	deletable := workflowAggregateNamed(t, ids.userID, now, "unused draft")
	createWorkflowForDeletion(t, repository, deletable, ids.userID, now)
	deleteMutation := deployapp.WorkflowMutation{ActorID: ids.userID, RequestID: "delete-unused-draft", OccurredAt: now}
	if err := repository.DeleteUnusedDraft(ctx, deleteMutation, deletable.ID, 1); err != nil {
		t.Fatalf("delete unused draft: %v", err)
	}
	if _, err := repository.Load(ctx, deletable.ID); !errors.Is(err, deployapp.ErrWorkflowNotFound) {
		t.Fatalf("load deleted workflow error = %v", err)
	}
	assertCountWhere(t, db, "audit_logs", "request_id = ?", deleteMutation.RequestID, 1)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", deletable.ID.String(), 2)

	stale := workflowAggregateNamed(t, ids.userID, now, "stale draft")
	createWorkflowForDeletion(t, repository, stale, ids.userID, now)
	assertWorkflowDeleteConflict(t, db, repository, stale.ID, 2, ids.userID, "delete-stale")

	publishedWorkflow := workflowAggregateNamed(t, ids.userID, now, "published workflow")
	createWorkflowForDeletion(t, repository, publishedWorkflow, ids.userID, now)
	published := publishWorkflowVersion(t, publishedWorkflow.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, deployapp.WorkflowMutation{ActorID: ids.userID, RequestID: "publish-delete-test", OccurredAt: now}, 1, published); err != nil {
		t.Fatalf("publish deletion test workflow: %v", err)
	}
	assertWorkflowDeleteConflict(t, db, repository, publishedWorkflow.ID, 1, ids.userID, "delete-published")

	referenced := workflowAggregateNamed(t, ids.userID, now, "bound draft")
	createWorkflowForDeletion(t, repository, referenced, ids.userID, now)
	if err := db.Model(&deploymentBindingReference{}).
		Where("environment_id = ?", ids.environmentID).
		Update("workflow_version_id", referenced.Versions[0].ID).Error; err != nil {
		t.Fatalf("reference draft workflow: %v", err)
	}
	assertWorkflowDeleteConflict(t, db, repository, referenced.ID, 1, ids.userID, "delete-referenced")
	if err := db.Model(&deploymentBindingReference{}).
		Where("environment_id = ?", ids.environmentID).
		Update("workflow_version_id", ids.workflowVersionID).Error; err != nil {
		t.Fatalf("restore workflow binding: %v", err)
	}

	requestReferenced := workflowAggregateNamed(t, ids.userID, now, "request referenced draft")
	createWorkflowForDeletion(t, repository, requestReferenced, ids.userID, now)
	requestIDs := ids
	requestIDs.workflowVersionID = requestReferenced.Versions[0].ID
	seedDeploymentRequest(t, db, requestIDs)
	assertWorkflowDeleteConflict(t, db, repository, requestReferenced.ID, 1, ids.userID, "delete-request-referenced")

	instanceReferenced := workflowAggregateNamed(t, ids.userID, now, "instance referenced draft")
	createWorkflowForDeletion(t, repository, instanceReferenced, ids.userID, now)
	instanceRequestVersionID, _ := seedDeploymentRequest(t, db, ids)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_workflow_instances (
			request_version_id, workflow_version_id, current_state_key, status
		) VALUES (?, ?, 'start', 'Running')
	`, instanceRequestVersionID, instanceReferenced.Versions[0].ID)
	assertWorkflowDeleteConflict(t, db, repository, instanceReferenced.ID, 1, ids.userID, "delete-instance-referenced")

	observationReferenced := workflowAggregateNamed(t, ids.userID, now, "observation referenced draft")
	createWorkflowForDeletion(t, repository, observationReferenced, ids.userID, now)
	observationRequestVersionID, _ := seedDeploymentRequest(t, db, ids)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_candidate_observations (
			application_id, request_version_id, workflow_version_id, plan_version_id,
			target_revision, target_revisions, manifest_hash, diff_hash,
			diff_snapshot, source_snapshot, image_snapshot, fingerprint, observed_at
		) VALUES (?, ?, ?, ?, 'revision-b', '["revision-b"]', ?, ?, '[]', '[]', '[]', ?, ?)
	`, ids.applicationID, observationRequestVersionID, observationReferenced.Versions[0].ID,
		ids.planVersionID, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), now)
	assertWorkflowDeleteConflict(t, db, repository, observationReferenced.ID, 1, ids.userID, "delete-observation-referenced")

	constraintReferenced := workflowAggregateNamed(t, ids.userID, now, "constraint referenced draft")
	createWorkflowForDeletion(t, repository, constraintReferenced, ids.userID, now)
	execDeploymentSQL(t, db, `
		CREATE TABLE workflow_delete_integration_guard (
			workflow_version_id UUID PRIMARY KEY REFERENCES release_workflow_versions(id)
		)
	`)
	execDeploymentSQL(t, db, `
		INSERT INTO workflow_delete_integration_guard (workflow_version_id) VALUES (?)
	`, constraintReferenced.Versions[0].ID)
	assertWorkflowDeleteConflict(t, db, repository, constraintReferenced.ID, 1, ids.userID, "delete-constraint-referenced")
}

type deploymentBindingReference struct{}

func (deploymentBindingReference) TableName() string { return "deployment_bindings" }

func workflowAggregateNamed(t *testing.T, actorID uuid.UUID, now time.Time, name string) deploydomain.ReleaseWorkflow {
	t.Helper()
	value, err := deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: name, CreatedBy: actorID, CreatedAt: now,
	}, integrationWorkflowDocument())
	if err != nil {
		t.Fatalf("create workflow aggregate: %v", err)
	}
	return value
}

func createWorkflowForDeletion(t *testing.T, repository *deployinfra.WorkflowDefinitionRepository, workflow deploydomain.ReleaseWorkflow, actorID uuid.UUID, now time.Time) {
	t.Helper()
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "create-" + workflow.ID.String(), OccurredAt: now}
	if err := repository.Create(context.Background(), mutation, workflow); err != nil {
		t.Fatalf("create workflow for deletion: %v", err)
	}
}

func assertWorkflowDeleteConflict(t *testing.T, db *gorm.DB, repository *deployinfra.WorkflowDefinitionRepository, workflowID uuid.UUID, expected uint64, actorID uuid.UUID, requestID string) {
	t.Helper()
	auditBefore := tableCount(t, db, "audit_logs")
	outboxBefore := tableCount(t, db, "outbox_events")
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: requestID, OccurredAt: time.Now().UTC()}
	if err := repository.DeleteUnusedDraft(context.Background(), mutation, workflowID, expected); !errors.Is(err, deployapp.ErrWorkflowConflict) {
		t.Fatalf("delete workflow conflict error = %v", err)
	}
	if _, err := repository.Load(context.Background(), workflowID); err != nil {
		t.Fatalf("load workflow after rejected deletion: %v", err)
	}
	if tableCount(t, db, "audit_logs") != auditBefore || tableCount(t, db, "outbox_events") != outboxBefore {
		t.Fatal("rejected workflow deletion must roll back audit and outbox records")
	}
}

func workflowTestDatabase(t *testing.T, ctx context.Context) *gorm.DB {
	t.Helper()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 5, MaxIdleConnections: 2})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	return db
}

func workflowAggregate(t *testing.T, actorID uuid.UUID, now time.Time) deploydomain.ReleaseWorkflow {
	t.Helper()
	value, err := deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: "Production approval", CreatedBy: actorID, CreatedAt: now,
	}, integrationWorkflowDocument())
	if err != nil {
		t.Fatalf("create workflow aggregate: %v", err)
	}
	return value
}

func workflowVersion(t *testing.T, workflowID uuid.UUID, number uint64, actorID uuid.UUID, now time.Time) deploydomain.ReleaseWorkflowVersion {
	t.Helper()
	value, err := deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: workflowID, VersionNumber: number, ActorID: actorID,
		Document: integrationWorkflowDocument(), CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}
	return value
}

func publishWorkflowVersion(t *testing.T, version deploydomain.ReleaseWorkflowVersion, now time.Time) deploydomain.ReleaseWorkflowVersion {
	t.Helper()
	value, err := version.ChangeLifecycle(deploydomain.DefinitionPublished, 1, now)
	if err != nil {
		t.Fatalf("publish workflow version: %v", err)
	}
	return value
}

func assertDraftAppendConflict(t *testing.T, repository *deployinfra.WorkflowDefinitionRepository, mutation deployapp.WorkflowMutation, workflow deploydomain.ReleaseWorkflow, actorID uuid.UUID, now time.Time) {
	t.Helper()
	second := workflowVersion(t, workflow.ID, 2, actorID, now.Add(time.Minute))
	if err := repository.AppendVersion(context.Background(), mutation, 1, second); !errors.Is(err, deployapp.ErrWorkflowDraftExists) {
		t.Fatalf("append with active draft error = %v", err)
	}
}

func integrationWorkflowDocument() deploydomain.WorkflowDocument {
	return deploydomain.WorkflowDocument{
		InitialState: "start",
		States: []deploydomain.WorkflowState{
			{Key: "start", Name: "Start", Type: deploydomain.WorkflowStateStart},
			{Key: "done", Name: "Done", Type: deploydomain.WorkflowStateTerminal},
		},
		Transitions: []deploydomain.WorkflowTransition{{
			Key: "finish", From: "start", To: "done",
			Trigger: deploydomain.WorkflowTriggerManual, Permission: "deployment_request.update",
		}},
	}
}

func assertCountWhere(t *testing.T, db *gorm.DB, table, query string, value any, expected int64) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where(query, value).Count(&count).Error; err != nil || count != expected {
		t.Fatalf("%s count = %d, error = %v", table, count, err)
	}
}
