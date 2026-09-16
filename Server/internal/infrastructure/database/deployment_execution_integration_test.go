//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestPartialFailedHoldsOperationLock(t *testing.T) {
	ctx, db, repository, execution, applications := partialExecutionFixture(t)
	status, err := repository.Complete(ctx, execution.ID, time.Now().UTC())
	if err != nil || status != "PartialFailed" {
		t.Fatalf("complete partial execution = %s, %v", status, err)
	}
	assertExecutionLockCount(t, db, execution.ID, int64(len(applications)))
	var workflowState string
	if err := db.Table("deployment_workflow_instances").Select("current_state_key").Where("request_version_id = ?", execution.RequestVersionID).Scan(&workflowState).Error; err != nil || workflowState != "failed" {
		t.Fatalf("deployment result workflow state = %q, %v", workflowState, err)
	}
}

func TestRetryHandoffsLocksAndForceUnlockPreservesAudit(t *testing.T) {
	ctx, db, repository, execution, applications := partialExecutionFixture(t)
	if _, err := repository.Complete(ctx, execution.ID, time.Now().UTC()); err != nil {
		t.Fatalf("complete partial execution: %v", err)
	}
	var lockVersion uint64
	if err := db.Table("deployment_executions").Select("lock_version").Where("id = ?", execution.ID).Scan(&lockVersion).Error; err != nil {
		t.Fatalf("load partial execution version: %v", err)
	}
	actorID := deploymentActor(t, db)
	workflowResult := executionWorkflowAction(t, db, execution.RequestVersionID, executionWorkflowActionInput{
		permission: "deployment_request.retry", status: "PartialFailed", actorID: actorID,
	})
	retry, err := repository.Retry(ctx, deployapp.ExecutionRetryChange{
		Mutation: deploymentMutation("retry-1", actorID), WorkflowResult: workflowResult, RequestID: uuid.New(),
		RequestVersionID: execution.RequestVersionID, ExecutionID: execution.ID,
		ExpectedVersion: lockVersion, ApplicationIDs: []uuid.UUID{applications[1]},
	})
	if err != nil || retry.Attempt != 2 {
		t.Fatalf("retry execution = %#v, %v", retry, err)
	}
	assertExecutionWorkflowState(t, db, retry.RequestVersionID, "deploying")
	assertRetryNodes(t, retry, applications)
	assertExecutionLockCount(t, db, retry.ID, int64(len(applications)))
	assertExecutionLockCount(t, db, execution.ID, 0)
	locks, _ := deployinfra.NewApplicationLocks(db)
	acquireLockSet(t, ctx, locks, lockRequest(retry.ID, applications, time.Now().UTC()))
	terminateAndUnlockExecution(t, ctx, db, repository, retry, applications)
}

func TestRetryDeploymentWaitsForNextEligibleWindow(t *testing.T) {
	scheduledFor := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	ctx, db, repository, execution, applications := partialExecutionFixtureAt(t, &scheduledFor)
	if _, err := repository.Complete(ctx, execution.ID, time.Now().UTC()); err != nil {
		t.Fatalf("complete partial execution: %v", err)
	}
	var lockVersion uint64
	if err := db.Table("deployment_executions").Select("lock_version").Where("id = ?", execution.ID).
		Scan(&lockVersion).Error; err != nil {
		t.Fatalf("load partial execution version: %v", err)
	}
	actorID := deploymentActor(t, db)
	workflowResult := executionWorkflowAction(t, db, execution.RequestVersionID, executionWorkflowActionInput{
		permission: "deployment_request.retry", status: "PartialFailed", actorID: actorID,
	})
	retry, err := repository.Retry(ctx, deployapp.ExecutionRetryChange{
		Mutation: deploymentMutation("retry-scheduled", actorID), WorkflowResult: workflowResult,
		RequestID: uuid.New(), RequestVersionID: execution.RequestVersionID, ExecutionID: execution.ID,
		ExpectedVersion: lockVersion, ApplicationIDs: []uuid.UUID{applications[1]},
	})
	if err != nil {
		t.Fatalf("create scheduled retry: %v", err)
	}
	var availableAt time.Time
	if err := db.Table("deployment_jobs").Select("available_at").Where("aggregate_id = ?", retry.ID).
		Take(&availableAt).Error; err != nil {
		t.Fatalf("load scheduled retry Job: %v", err)
	}
	if !availableAt.Equal(scheduledFor) {
		t.Fatalf("scheduled retry available_at = %s, want %s", availableAt, scheduledFor)
	}
}

func partialExecutionFixture(t *testing.T) (context.Context, *gorm.DB, *deployinfra.ExecutionRepository, deployapp.ExecutionSnapshot, []uuid.UUID) {
	return partialExecutionFixtureAt(t, nil)
}

func partialExecutionFixtureAt(t *testing.T, scheduledFor *time.Time) (context.Context, *gorm.DB, *deployinfra.ExecutionRepository, deployapp.ExecutionSnapshot, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	ids = seedExecutionResultWorkflow(t, db, ids)
	versionID, _ := seedDeploymentRequestAt(t, db, ids, scheduledFor)
	seedExecutionWorkflowInstance(t, db, ids, versionID)
	workerID := seedExecutionApplication(t, db, ids, versionID)
	repository, _ := deployinfra.NewExecutionRepository(db)
	execution, err := repository.Prepare(ctx, versionID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare partial execution: %v", err)
	}
	applications := []uuid.UUID{ids.applicationID, workerID}
	locks, _ := deployinfra.NewApplicationLocks(db)
	acquireLockSet(t, ctx, locks, lockRequest(execution.ID, applications, time.Now().UTC()))
	if err := repository.MarkRunning(ctx, execution.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark partial execution running: %v", err)
	}
	updateExecutionResult(t, ctx, repository, execution.ID, applications[0], "Succeeded")
	updateExecutionResult(t, ctx, repository, execution.ID, applications[1], "Failed")
	return ctx, db, repository, execution, applications
}

func seedExecutionWorkflowInstance(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, versionID uuid.UUID) {
	t.Helper()
	execDeploymentSQL(t, db, `INSERT INTO deployment_workflow_instances (
		id, request_version_id, workflow_version_id, current_state_key, status
	) VALUES (?, ?, ?, 'deploying', 'Running')`, uuid.New(), versionID, ids.workflowVersionID)
}

func seedExecutionResultWorkflow(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) deploymentSchemaIDs {
	t.Helper()
	workflowID := uuid.New()
	ids.workflowVersionID = uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO release_workflows (id, name, created_by)
		VALUES (?, 'execution result workflow', ?)`, workflowID, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO release_workflow_versions (
		id, workflow_id, version_number, lifecycle, document, created_by, published_at
	) VALUES (?, ?, 1, 'Published', '{
		"initialState":"deploying",
		"states":[
			{"key":"deploying","name":"Deploying","type":"Deployment"},
			{"key":"succeeded","name":"Succeeded","type":"Terminal"},
			{"key":"failed","name":"Failed","type":"ManualAction"},
			{"key":"terminated","name":"Terminated","type":"Terminal"}
		],
		"transitions":[
			{"key":"succeed","from":"deploying","to":"succeeded","trigger":"DeploymentResult","permission":"deployment_request.update","conditions":[{"fact":"deployment.status","operator":"Equals","values":["Succeeded"]}]},
			{"key":"fail","from":"deploying","to":"failed","trigger":"DeploymentResult","permission":"deployment_request.update","conditions":[{"fact":"deployment.status","operator":"In","values":["Failed","PartialFailed"]}]},
			{"key":"retry","from":"failed","to":"deploying","trigger":"Manual","permission":"deployment_request.retry","conditions":[{"fact":"deployment.status","operator":"In","values":["Failed","PartialFailed"]}]},
			{"key":"terminate-failed","from":"failed","to":"terminated","trigger":"Manual","permission":"deployment_request.terminate","conditions":[{"fact":"deployment.status","operator":"In","values":["Running","PartialFailed"]}]},
			{"key":"terminate-running","from":"deploying","to":"terminated","trigger":"Manual","permission":"deployment_request.terminate","conditions":[{"fact":"deployment.status","operator":"Equals","values":["Running"]}]}
		]
	}', ?, now())`, ids.workflowVersionID, workflowID, ids.userID)
	return ids
}

func seedExecutionApplication(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, versionID uuid.UUID) uuid.UUID {
	t.Helper()
	applicationID := seedLockApplication(t, db, ids, "worker")
	requestApplicationID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO deployment_request_applications (
		id, request_version_id, application_id, application_key, live_revision, target_revision,
		target_revisions, manifest_hash, diff_hash, diff_snapshot, source_snapshot
	) VALUES (?, ?, ?, 'worker', 'revision-a', 'revision-b', '["revision-b"]', ?, ?, '{}', '{}')`,
		requestApplicationID, versionID, applicationID, strings.Repeat("e", 64), strings.Repeat("f", 64))
	execDeploymentSQL(t, db, `INSERT INTO deployment_request_images (
		request_application_id, image_reference, registry, repository, image_tag, image_digest
	) VALUES (?, 'registry.example/worker:v1.0.0', 'registry.example', 'worker', 'v1.0.0', ?)`,
		requestApplicationID, "sha256:"+strings.Repeat("1", 64))
	return applicationID
}

func updateExecutionResult(t *testing.T, ctx context.Context, repository *deployinfra.ExecutionRepository, executionID, applicationID uuid.UUID, status string) {
	t.Helper()
	err := repository.UpdateNode(ctx, deployapp.ExecutionNodeUpdate{
		ExecutionID: executionID, ApplicationID: applicationID, Status: status,
		ActualRevision: "revision-b", UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("update execution node %s: %v", status, err)
	}
}

func deploymentMutation(key string, actorID uuid.UUID) deployapp.ExecutionMutation {
	return deployapp.ExecutionMutation{ActorID: actorID, RequestID: "request-1",
		IdempotencyKey: key, RequestHash: strings.Repeat("a", 64), OccurredAt: time.Now().UTC()}
}

func deploymentActor(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	var actorValue string
	if err := db.Table("users").Select("id::text").Limit(1).Scan(&actorValue).Error; err != nil {
		t.Fatalf("load deployment actor: %v", err)
	}
	actorID, err := uuid.Parse(actorValue)
	if err != nil {
		t.Fatalf("parse deployment actor: %v", err)
	}
	return actorID
}

func assertRetryNodes(t *testing.T, retry deployapp.DeploymentExecution, applications []uuid.UUID) {
	t.Helper()
	statuses := map[uuid.UUID]string{}
	for _, node := range retry.Nodes {
		statuses[node.ApplicationID] = node.Status
	}
	if statuses[applications[0]] != "Succeeded" || statuses[applications[1]] != "Waiting" {
		t.Fatalf("retry node statuses = %#v", statuses)
	}
}

func terminateAndUnlockExecution(t *testing.T, ctx context.Context, db *gorm.DB, repository *deployinfra.ExecutionRepository, retry deployapp.DeploymentExecution, applications []uuid.UUID) {
	t.Helper()
	if err := repository.MarkRunning(ctx, retry.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark retry running: %v", err)
	}
	actorID := deploymentActor(t, db)
	workflowResult := executionWorkflowAction(t, db, retry.RequestVersionID, executionWorkflowActionInput{
		permission: "deployment_request.terminate", status: "Running", actorID: actorID,
	})
	terminated, err := repository.Terminate(ctx, deployapp.ExecutionTerminateChange{
		Mutation: deploymentMutation("terminate-1", actorID), WorkflowResult: workflowResult, ExecutionID: retry.ID,
		ExpectedVersion: retry.LockVersion, Reason: "operator stop",
	})
	if err != nil || terminated.Status != "Terminated" {
		t.Fatalf("terminate execution = %#v, %v", terminated, err)
	}
	assertExecutionWorkflowState(t, db, retry.RequestVersionID, "terminated")
	states := []deployapp.ActualState{{ApplicationID: applications[0], Revision: "revision-b"},
		{ApplicationID: applications[1], Revision: "revision-a"}}
	_, err = repository.Unlock(ctx, deployapp.ExecutionUnlockChange{
		Mutation: deploymentMutation("unlock-1", deploymentActor(t, db)), ExecutionID: retry.ID,
		ExpectedVersion: terminated.LockVersion, Reason: "actual state verified", ActualStates: states,
	})
	if err != nil {
		t.Fatalf("unlock execution: %v", err)
	}
	assertExecutionLockCount(t, db, retry.ID, 0)
	assertExecutionCommandAudit(t, db, retry.ID)
}

func assertExecutionWorkflowState(t *testing.T, db *gorm.DB, versionID uuid.UUID, expected string) {
	t.Helper()
	var actual string
	if err := db.Table("deployment_workflow_instances").Select("current_state_key").
		Where("request_version_id = ?", versionID).Scan(&actual).Error; err != nil || actual != expected {
		t.Fatalf("workflow state = %q, want %q, error %v", actual, expected, err)
	}
}

type executionWorkflowActionInput struct {
	permission string
	status     string
	actorID    uuid.UUID
}

func executionWorkflowAction(t *testing.T, db *gorm.DB, versionID uuid.UUID, input executionWorkflowActionInput) deploydomain.WorkflowResult {
	t.Helper()
	repository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		t.Fatalf("create workflow runtime repository: %v", err)
	}
	snapshot, err := repository.Load(context.Background(), versionID)
	if err != nil || snapshot.Instance == nil {
		t.Fatalf("load workflow runtime action: %#v, %v", snapshot, err)
	}
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Version.Document)
	if err != nil {
		t.Fatalf("create workflow action engine: %v", err)
	}
	result, err := engine.ApplyPermissionAction(*snapshot.Instance, deploydomain.WorkflowTransitionCommand{
		Permission: input.permission, ActorID: input.actorID, PermissionGranted: true,
		Facts:      map[deploydomain.WorkflowFact]string{deploydomain.WorkflowFactDeploymentStatus: input.status},
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("apply workflow action: %v", err)
	}
	return result
}

func assertExecutionCommandAudit(t *testing.T, db *gorm.DB, executionID uuid.UUID) {
	t.Helper()
	var commands, audits int64
	if err := db.Table("deployment_execution_commands").Where("execution_id = ?", executionID).Count(&commands).Error; err != nil {
		t.Fatalf("count execution commands: %v", err)
	}
	if err := db.Table("audit_logs").Where("resource_id = ? AND action IN ?", executionID.String(), []string{"deployment_execution.terminate", "deployment_execution.unlock"}).Count(&audits).Error; err != nil {
		t.Fatalf("count execution command audits: %v", err)
	}
	if commands != 2 || audits != 2 {
		t.Fatalf("execution evidence = commands %d audits %d", commands, audits)
	}
	if err := db.Exec("UPDATE deployment_execution_commands SET reason = 'changed' WHERE execution_id = ?", executionID).Error; err == nil {
		t.Fatal("deployment execution commands must be immutable")
	}
}

func TestExecutionRepositoryPreparesImmutableSnapshotIdempotently(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	versionID, applicationID := seedDeploymentRequest(t, db, ids)
	repository, _ := deployinfra.NewExecutionRepository(db)
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

	first, err := repository.Prepare(ctx, versionID, now)
	if err != nil {
		t.Fatalf("prepare deployment execution: %v", err)
	}
	second, err := repository.Prepare(ctx, versionID, now.Add(time.Minute))
	if err != nil || second.ID != first.ID {
		t.Fatalf("reload deployment execution: %#v %v", second, err)
	}
	if first.Status != "Preflight" || len(first.Targets) != 1 {
		t.Fatalf("execution snapshot mismatch: %#v", first)
	}
	if first.Targets[0].Preflight.Identity.Name != "api-production" || first.Targets[0].Node.Key != "api" {
		t.Fatalf("execution target mismatch: %#v", first.Targets[0])
	}
	if err := repository.MarkRunning(ctx, first.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark execution running: %v", err)
	}
	operationID := first.ID.String() + ":api"
	if err := repository.UpdateNode(ctx, deployapp.ExecutionNodeUpdate{
		ExecutionID: first.ID, ApplicationID: ids.applicationID,
		Status: "Syncing", OperationID: operationID, UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("persist deployment operation identity: %v", err)
	}
	recovered, err := repository.Prepare(ctx, versionID, now.Add(2*time.Minute))
	if err != nil || recovered.Status != "Running" || recovered.Targets[0].OperationID != operationID {
		t.Fatalf("recover deployment operation: %#v %v", recovered, err)
	}
	assertExecutionRows(t, db, first.ID, versionID, 1, "Deploying")

	if err := repository.Block(ctx, deployapp.ExecutionBlock{
		ExecutionID: first.ID, RequestApplicationID: applicationID,
		Code: "image_digest_drift", OccurredAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("block deployment execution: %v", err)
	}
	assertExecutionRows(t, db, first.ID, versionID, 1, "Superseded")
}

func assertExecutionRows(t *testing.T, db *gorm.DB, executionID, versionID uuid.UUID, expectedNodes int64, versionStatus string) {
	t.Helper()
	var nodes int64
	if err := db.Table("deployment_execution_nodes").Where("execution_id = ?", executionID).Count(&nodes).Error; err != nil {
		t.Fatalf("count deployment execution nodes: %v", err)
	}
	var status string
	if err := db.Table("deployment_request_versions").Select("status").Where("id = ?", versionID).Scan(&status).Error; err != nil {
		t.Fatalf("load deployment Request Version status: %v", err)
	}
	if nodes != expectedNodes || status != versionStatus {
		t.Fatalf("execution rows = nodes %d status %s", nodes, status)
	}
}
