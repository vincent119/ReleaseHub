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
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestDeploymentRequestRepositoryListsLatestVersionSummary(t *testing.T) {
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
	repository, err := deployinfra.NewDeploymentRequestRepository(db)
	if err != nil {
		t.Fatalf("create deployment request repository: %v", err)
	}
	scope, err := authz.NewEnvironmentScope(ids.organizationID, ids.projectID, ids.environmentID)
	if err != nil {
		t.Fatalf("create environment scope: %v", err)
	}

	empty, err := repository.List(ctx, scope)
	if err != nil || len(empty) != 0 {
		t.Fatalf("list empty deployment request scope: %#v %v", empty, err)
	}

	firstVersionID, _ := seedDeploymentRequest(t, db, ids)
	secondVersionID := seedSecondDeploymentRequestVersion(t, db, firstVersionID)
	cloneDeploymentRequestApplication(t, db, firstVersionID, secondVersionID)

	values, err := repository.List(ctx, scope)
	if err != nil {
		t.Fatalf("list deployment requests: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("deployment request count = %d, want 1", len(values))
	}
	if values[0].ActiveVersionNumber != 2 || values[0].Title != "Deploy API v2" || values[0].ApplicationCount != 1 {
		t.Fatalf("latest deployment request summary = %#v", values[0])
	}
}

func seedSecondDeploymentRequestVersion(t *testing.T, db *gorm.DB, firstVersionID uuid.UUID) uuid.UUID {
	t.Helper()
	secondVersionID := uuid.New()
	execDeploymentSQL(t, db, `UPDATE deployment_request_versions SET status = 'Superseded' WHERE id = ?`, firstVersionID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_versions (
			id, request_id, version_number, status, fingerprint,
			workflow_version_id, plan_version_id, title, source_snapshot, created_by
		)
		SELECT ?, request_id, 2, 'Candidate', ?, workflow_version_id, plan_version_id,
		       'Deploy API v2', source_snapshot, created_by
		FROM deployment_request_versions WHERE id = ?
	`, secondVersionID, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", firstVersionID)
	return secondVersionID
}

func cloneDeploymentRequestApplication(t *testing.T, db *gorm.DB, firstVersionID, secondVersionID uuid.UUID) {
	t.Helper()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_applications (
			id, request_version_id, application_id, application_key,
			live_revision, target_revision, target_revisions,
			manifest_hash, diff_hash, diff_snapshot, source_snapshot, execution_order
		)
		SELECT ?, ?, application_id, application_key, live_revision, 'revision-c', '["revision-c"]',
		       manifest_hash, diff_hash, diff_snapshot, source_snapshot, execution_order
		FROM deployment_request_applications WHERE request_version_id = ?
	`, uuid.New(), secondVersionID, firstVersionID)
}
