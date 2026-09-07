//go:build integration

package database_test

import (
	"context"
	"errors"
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
	if err := repository.AppendVersion(context.Background(), mutation, 1, second); !errors.Is(err, deployapp.ErrWorkflowConflict) {
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
