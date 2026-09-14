//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestDeploymentSchemaConstraintsAndIndexes(t *testing.T) {
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

	ids := seedDeploymentSchemaScope(t, db)
	ids = seedDeploymentDefinitions(t, db, ids)

	if err := db.Exec(`UPDATE release_workflow_versions SET document = '{"states":[]}'::jsonb WHERE id = ?`, ids.workflowVersionID).Error; err == nil {
		t.Fatal("published workflow document must be immutable")
	}
	if err := db.Exec(`UPDATE deployment_plan_versions SET document = '{"nodes":[]}'::jsonb WHERE id = ?`, ids.planVersionID).Error; err == nil {
		t.Fatal("published plan document must be immutable")
	}

	requestVersionID, requestApplicationID := seedDeploymentRequest(t, db, ids)
	if err := db.Exec(`UPDATE deployment_request_versions SET title = 'changed' WHERE id = ?`, requestVersionID).Error; err == nil {
		t.Fatal("request version snapshot must be immutable")
	}
	if err := db.Exec(`UPDATE deployment_request_versions SET status = 'PendingReview', lock_version = lock_version + 1 WHERE id = ?`, requestVersionID).Error; err != nil {
		t.Fatalf("request version lifecycle update should remain possible: %v", err)
	}
	if err := db.Exec(`UPDATE deployment_request_applications SET target_revision = 'revision-b' WHERE id = ?`, requestApplicationID).Error; err == nil {
		t.Fatal("request application snapshot must be immutable")
	}

	verifyDeploymentQueueConstraints(t, db, ids.applicationID, requestVersionID, ids.planVersionID)
	verifyDeploymentReviewConstraints(t, db, ids.userID, requestVersionID, ids.workflowVersionID)
	verifyDeploymentPermissions(t, db)
	verifyDeploymentIndexes(t, db)
	verifyDeploymentNotificationMigrationDownAndUp(t, cfg, db)
}

type deploymentSchemaIDs struct {
	userID            uuid.UUID
	organizationID    uuid.UUID
	projectID         uuid.UUID
	environmentID     uuid.UUID
	applicationID     uuid.UUID
	workflowVersionID uuid.UUID
	planVersionID     uuid.UUID
}

func seedDeploymentSchemaScope(t *testing.T, db *gorm.DB) deploymentSchemaIDs {
	t.Helper()
	ids := deploymentSchemaIDs{
		userID:         uuid.New(),
		organizationID: uuid.New(),
		projectID:      uuid.New(),
		environmentID:  uuid.New(),
		applicationID:  uuid.New(),
	}
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'deployment-schema-user')`, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO organizations (id, name) VALUES (?, 'deployment-schema-organization')`, ids.organizationID)
	execDeploymentSQL(t, db, `INSERT INTO projects (id, organization_id, name) VALUES (?, ?, 'deployment-schema-project')`, ids.projectID, ids.organizationID)
	execDeploymentSQL(t, db, `INSERT INTO environments (id, organization_id, project_id, name, environment_type) VALUES (?, ?, ?, 'production', 'Production')`, ids.environmentID, ids.organizationID, ids.projectID)
	execDeploymentSQL(t, db, `
		INSERT INTO applications (
			id, organization_id, project_id, environment_id, name,
			argocd_namespace, argocd_application_name, argocd_project,
			destination_server, destination_namespace,
			source_repo_url, source_target_revision, source_path
		) VALUES (?, ?, ?, ?, 'api', 'argocd', 'api-production', 'api',
			'https://kubernetes.default.svc', 'api', 'https://git.example/gitops.git', 'main', 'production/api')
	`, ids.applicationID, ids.organizationID, ids.projectID, ids.environmentID)
	return ids
}

func seedDeploymentDefinitions(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) deploymentSchemaIDs {
	t.Helper()
	workflowID := uuid.New()
	ids.workflowVersionID = uuid.New()
	planID := uuid.New()
	ids.planVersionID = uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO release_workflows (id, name, created_by) VALUES (?, 'production approval', ?)`, workflowID, ids.userID)
	execDeploymentSQL(t, db, `
		INSERT INTO release_workflow_versions (
			id, workflow_id, version_number, lifecycle, document, created_by, published_at
		) VALUES (?, ?, 1, 'Published', '{"initialState":"start","states":[{"key":"start","name":"Start","type":"Start"}],"transitions":[]}', ?, now())
	`, ids.workflowVersionID, workflowID, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO deployment_plans (id, owner_kind, owner_project_id, name, created_by) VALUES (?, 'project', ?, 'api plan', ?)`, planID, ids.projectID, ids.userID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_plan_versions (
			id, plan_id, version_number, lifecycle, document, created_by, published_at
		) VALUES (?, ?, 1, 'Published', '{
			"nodes":[
				{"key":"api","applicationKey":"api","order":0,"successCondition":{"syncStatuses":["Synced"],"healthStatuses":["Healthy"],"stabilizationSeconds":0}},
				{"key":"worker","applicationKey":"worker","order":1,"successCondition":{"syncStatuses":["Synced"],"healthStatuses":["Healthy"],"stabilizationSeconds":0}}
			],
			"edges":[]
		}', ?, now())
	`, ids.planVersionID, planID, ids.userID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_bindings (
			organization_id, project_id, environment_id, workflow_version_id, plan_version_id, created_by
		) VALUES (?, ?, ?, ?, ?, ?)
	`, ids.organizationID, ids.projectID, ids.environmentID, ids.workflowVersionID, ids.planVersionID, ids.userID)
	return ids
}

func seedDeploymentRequest(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) (uuid.UUID, uuid.UUID) {
	t.Helper()
	requestID := uuid.New()
	requestVersionID := uuid.New()
	requestApplicationID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_requests (id, organization_id, project_id, environment_id)
		VALUES (?, ?, ?, ?)
	`, requestID, ids.organizationID, ids.projectID, ids.environmentID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_versions (
			id, request_id, version_number, status, fingerprint,
			workflow_version_id, plan_version_id, title, source_snapshot, created_by
		) VALUES (?, ?, 1, 'Candidate', ?, ?, ?, 'Deploy API', '{}', ?)
	`, requestVersionID, requestID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ids.workflowVersionID, ids.planVersionID, ids.userID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_applications (
			id, request_version_id, application_id, application_key,
			live_revision, target_revision, target_revisions,
			manifest_hash, diff_hash, diff_snapshot, source_snapshot
		) VALUES (?, ?, ?, 'api', 'revision-a', 'revision-b', '["revision-b"]', ?, ?, '{}', '{}')
	`, requestApplicationID, requestVersionID, ids.applicationID,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_images (
			request_application_id, image_reference, registry, repository, image_tag, image_digest
		) VALUES (?, 'registry.example/api:v1.0.0', 'registry.example', 'api', 'v1.0.0', ?)
	`, requestApplicationID, "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	return requestVersionID, requestApplicationID
}

func verifyDeploymentQueueConstraints(t *testing.T, db *gorm.DB, applicationID, requestVersionID, planVersionID uuid.UUID) {
	t.Helper()
	executionID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_executions (
			id, request_version_id, plan_version_id, status, trigger_kind, plan_snapshot
		) VALUES (?, ?, ?, 'Queued', 'Workflow', '{}')
	`, executionID, requestVersionID, planVersionID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_application_locks (application_id, execution_id, owner_token, fencing_token)
		VALUES (?, ?, ?, 1)
	`, applicationID, executionID, uuid.New())
	if err := db.Exec(`
		INSERT INTO deployment_application_locks (application_id, execution_id, owner_token, fencing_token)
		VALUES (?, ?, ?, 2)
	`, applicationID, executionID, uuid.New()).Error; err == nil {
		t.Fatal("an Application must not be locked twice")
	}
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_jobs (job_type, aggregate_type, aggregate_id, payload, idempotency_key)
		VALUES ('ExecuteDeployment', 'DeploymentExecution', ?, '{}', 'execute-request-version-1')
	`, executionID)
	if err := db.Exec(`
		INSERT INTO deployment_jobs (job_type, aggregate_type, aggregate_id, payload, idempotency_key)
		VALUES ('ExecuteDeployment', 'DeploymentExecution', ?, '{}', 'execute-request-version-1')
	`, executionID).Error; err == nil {
		t.Fatal("duplicate deployment job idempotency key must be rejected")
	}
}

func verifyDeploymentReviewConstraints(t *testing.T, db *gorm.DB, userID, requestVersionID, workflowVersionID uuid.UUID) {
	t.Helper()
	instanceID := uuid.New()
	reviewTaskID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_workflow_instances (
			id, request_version_id, workflow_version_id, current_state_key, status
		) VALUES (?, ?, ?, 'review', 'Running')
	`, instanceID, requestVersionID, workflowVersionID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_review_tasks (
			id, workflow_instance_id, request_version_id, state_key, stage_number,
			policy_type, required_approvals, assignee_snapshot, status
		) VALUES (?, ?, ?, 'review', 1, 'AnyApprover', 1, '{}', 'Pending')
	`, reviewTaskID, instanceID, requestVersionID)
	if err := db.Exec(`
		INSERT INTO deployment_review_decisions (
			review_task_id, reviewer_id, decision, reason, permission_snapshot, idempotency_key
		) VALUES (?, ?, 'Reject', '', '{}', 'review-1')
	`, reviewTaskID, userID).Error; err == nil {
		t.Fatal("review rejection must require a reason")
	}
	decisionID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_review_decisions (
			id, review_task_id, reviewer_id, decision, permission_snapshot, idempotency_key
		) VALUES (?, ?, ?, 'Approve', '{}', 'review-1')
	`, decisionID, reviewTaskID, userID)
	if err := db.Exec(`UPDATE deployment_review_decisions SET reason = 'changed' WHERE id = ?`, decisionID).Error; err == nil {
		t.Fatal("review decision must be append-only")
	}
}

func verifyDeploymentPermissions(t *testing.T, db *gorm.DB) {
	t.Helper()
	var count int64
	if err := db.Table("authorization_permissions").Where("key LIKE ? OR key IN ?", "deployment_%", []string{"workflow.manage", "notification.view", "notification.mark_read"}).Count(&count).Error; err != nil {
		t.Fatalf("count deployment permissions: %v", err)
	}
	if count != 13 {
		t.Fatalf("deployment permission count mismatch, got %d want 13", count)
	}
}

func verifyDeploymentIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	expected := []string{
		"deployment_application_locks_execution_idx",
		"deployment_jobs_claim_idx",
		"deployment_jobs_expired_lease_idx",
		"deployment_notifications_recipient_idx",
		"deployment_notifications_recipient_event_idx",
		"deployment_review_reassignments_task_idx",
		"deployment_request_versions_active_idx",
	}
	var count int64
	if err := db.Table("pg_indexes").Where("schemaname = 'public' AND indexname IN ?", expected).Count(&count).Error; err != nil {
		t.Fatalf("count deployment indexes: %v", err)
	}
	if count != int64(len(expected)) {
		t.Fatalf("deployment index count mismatch, got %d want %d", count, len(expected))
	}
}

func verifyDeploymentNotificationMigrationDownAndUp(t *testing.T, cfg config.DatabaseConfig, db *gorm.DB) {
	t.Helper()
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, database.URL(cfg))
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	if err := migrator.Migrate(20260907001300); err != nil {
		t.Fatalf("roll back deployment migration: %v", err)
	}
	var indexName *string
	if err := db.Raw(`SELECT to_regclass('public.deployment_notifications_recipient_event_idx')::text`).Scan(&indexName).Error; err != nil {
		t.Fatalf("check rolled back notification index: %v", err)
	}
	if indexName != nil {
		t.Fatalf("notification index still exists after rollback: %s", *indexName)
	}
	var tableName *string
	if err := db.Raw(`SELECT to_regclass('public.deployment_execution_commands')::text`).Scan(&tableName).Error; err != nil || tableName == nil {
		t.Fatalf("previous deployment schema must remain after one rollback: %v", err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatalf("reapply deployment migration: %v", err)
	}
}

func execDeploymentSQL(t *testing.T, db *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := db.Exec(query, args...).Error; err != nil {
		t.Fatalf("execute deployment schema fixture: %v", err)
	}
}
