//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestForwardRollbackCreatesNormalRequest(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	baseTime := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	seedSuccessfulDeployment(t, db, ids, successfulDeploymentInput{
		tag: "v1.0.0", digest: historyDigest('a'), revision: "historical-revision", completedAt: baseTime,
	})
	seedSuccessfulDeployment(t, db, ids, successfulDeploymentInput{
		tag: "v2.0.0", digest: historyDigest('b'), revision: "current-revision", completedAt: baseTime.Add(time.Hour),
	})
	repository, _ := deployinfra.NewCandidateRepository(db)
	observation := forwardRollbackObservation(t, ids, baseTime.Add(2*time.Hour))
	created, err := repository.CreateDeploymentRequest(ctx, observation)
	if err != nil || !created {
		t.Fatalf("create Forward Rollback request = %v, %v", created, err)
	}
	assertForwardRollbackClassification(t, db)
}

func TestSuccessfulDeploymentHistoryUsesCursorPagination(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	baseTime := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	seedSuccessfulDeployment(t, db, ids, successfulDeploymentInput{
		tag: "v1.0.0", digest: historyDigest('a'), revision: "revision-a", completedAt: baseTime,
	})
	seedSuccessfulDeployment(t, db, ids, successfulDeploymentInput{
		tag: "v2.0.0", digest: historyDigest('b'), revision: "revision-b", completedAt: baseTime.Add(time.Hour),
	})
	repository, _ := deployinfra.NewDeploymentHistoryRepository(db)
	scope, err := repository.ResolveScope(ctx, ids.projectID, ids.environmentID)
	if err != nil {
		t.Fatalf("resolve deployment history scope: %v", err)
	}
	first, err := repository.List(ctx, scope, "", 1)
	if err != nil || len(first.Items) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first deployment history page = %#v, %v", first, err)
	}
	second, err := repository.List(ctx, scope, first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second deployment history page = %#v, %v", second, err)
	}
	if first.Items[0].Applications[0].Images[0].Digest != historyDigest('b') ||
		second.Items[0].Applications[0].Images[0].Digest != historyDigest('a') {
		t.Fatalf("deployment history order = %#v %#v", first.Items, second.Items)
	}
}

type successfulDeploymentInput struct {
	tag         string
	digest      string
	revision    string
	completedAt time.Time
}

func seedSuccessfulDeployment(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, input successfulDeploymentInput) {
	t.Helper()
	requestID, versionID, requestApplicationID, executionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO deployment_requests
        (id, organization_id, project_id, environment_id) VALUES (?, ?, ?, ?)`,
		requestID, ids.organizationID, ids.projectID, ids.environmentID)
	execDeploymentSQL(t, db, `INSERT INTO deployment_request_versions
        (id, request_id, version_number, status, fingerprint, workflow_version_id, plan_version_id,
         title, source_snapshot, created_by, created_at, updated_at)
        VALUES (?, ?, 1, 'Succeeded', ?, ?, ?, 'Successful deployment', '{}', ?, ?, ?)`,
		versionID, requestID, uuid.NewString(), ids.workflowVersionID, ids.planVersionID,
		ids.userID, input.completedAt, input.completedAt)
	seedSuccessfulRequestApplication(t, db, ids, input, versionID, requestApplicationID)
	execDeploymentSQL(t, db, `INSERT INTO deployment_executions
        (id, request_version_id, plan_version_id, status, trigger_kind, plan_snapshot, completed_at, created_at, updated_at)
        VALUES (?, ?, ?, 'Succeeded', 'Workflow', '{}', ?, ?, ?)`,
		executionID, versionID, ids.planVersionID, input.completedAt, input.completedAt, input.completedAt)
	seedSuccessfulExecutionNode(t, db, ids, input, executionID, requestApplicationID)
}

func seedSuccessfulRequestApplication(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, input successfulDeploymentInput, versionID, requestApplicationID uuid.UUID) {
	t.Helper()
	execDeploymentSQL(t, db, `INSERT INTO deployment_request_applications
        (id, request_version_id, application_id, application_key, target_revision, target_revisions,
         manifest_hash, diff_hash, diff_snapshot, source_snapshot, created_at)
        VALUES (?, ?, ?, 'api', ?, '[]', ?, ?, '{}', '{}', ?)`, requestApplicationID, versionID,
		ids.applicationID, input.revision, repeatHex('a'), repeatHex('b'), input.completedAt)
	execDeploymentSQL(t, db, `INSERT INTO deployment_request_images
        (request_application_id, image_reference, registry, repository, image_tag, image_digest, created_at)
        VALUES (?, ?, 'registry.example', 'platform/api', ?, ?, ?)`, requestApplicationID,
		"registry.example/platform/api:"+input.tag, input.tag, input.digest, input.completedAt)
}

func seedSuccessfulExecutionNode(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, input successfulDeploymentInput, executionID, requestApplicationID uuid.UUID) {
	t.Helper()
	images, _ := json.Marshal([]deploydomain.DeploymentRequestImageSnapshot{{
		ImageReference: "registry.example/platform/api:" + input.tag,
		Registry:       "registry.example", Repository: "platform/api", Tag: input.tag, Digest: input.digest,
	}})
	execDeploymentSQL(t, db, `INSERT INTO deployment_execution_nodes
        (execution_id, request_application_id, application_id, node_key, status,
         actual_revision, actual_images, completed_at, updated_at)
        VALUES (?, ?, ?, 'api', 'Succeeded', ?, ?, ?, ?)`, executionID, requestApplicationID,
		ids.applicationID, input.revision, images, input.completedAt, input.completedAt)
}

func forwardRollbackObservation(t *testing.T, ids deploymentSchemaIDs, observedAt time.Time) deploydomain.CandidateObservation {
	t.Helper()
	value := candidateObservationFixture(t, ids.applicationID, ids)
	value.TargetRevision, value.TargetRevisions = "rollback-revision", []string{"rollback-revision"}
	value.Images[0] = deploydomain.ImageSnapshot{
		ImageReference: "registry.example/platform/api:v3.0.0", Registry: "registry.example",
		Repository: "platform/api", Tag: "v3.0.0", Digest: historyDigest('a'),
	}
	value.ObservedAt = observedAt
	result, err := deploydomain.NewCandidateObservation(value)
	if err != nil {
		t.Fatalf("create Forward Rollback observation: %v", err)
	}
	return result
}

func assertForwardRollbackClassification(t *testing.T, db *gorm.DB) {
	t.Helper()
	var classification string
	err := db.Table("deployment_requests request").Joins(
		"JOIN deployment_request_versions version ON version.request_id = request.id").
		Where("version.status = 'Candidate'").Select("request.classification").Scan(&classification).Error
	if err != nil || classification != deploydomain.DeploymentClassificationForwardRollback {
		t.Fatalf("Forward Rollback classification = %q, %v", classification, err)
	}
	var audits int64
	db.Table("audit_logs").Where("action = ?", "deployment_request.forward_rollback_classified").Count(&audits)
	if audits != 1 {
		t.Fatalf("Forward Rollback audit count = %d", audits)
	}
}

func historyDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
