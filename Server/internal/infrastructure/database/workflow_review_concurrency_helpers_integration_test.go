//go:build integration

package database_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

type workflowConcurrencyFixture struct {
	db         *gorm.DB
	repository *deployinfra.WorkflowRuntimeRepository
	service    *deployapp.WorkflowRuntimeService
	principal  deployapp.WorkflowPrincipal
	snapshot   deployapp.WorkflowRuntimeSnapshot
}

func workflowConcurrencyFixtureFor(t *testing.T, documentFor func(uuid.UUID) deploydomain.WorkflowDocument) workflowConcurrencyFixture {
	t.Helper()
	ctx := t.Context()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	actorID := seedConcurrencyActor(t, db, "concurrent-runtime-actor")
	workflow, err := deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: "Concurrent runtime", CreatedBy: actorID, CreatedAt: runtimeIntegrationTime(),
	}, documentFor(actorID))
	if err != nil {
		t.Fatalf("create concurrent workflow: %v", err)
	}
	definitions, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create concurrent workflow repository: %v", err)
	}
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "concurrent-seed", OccurredAt: runtimeIntegrationTime()}
	if err := definitions.Create(ctx, mutation, workflow); err != nil {
		t.Fatalf("persist concurrent workflow: %v", err)
	}
	published := publishWorkflowVersion(t, workflow.Versions[0], runtimeIntegrationTime())
	if err := definitions.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish concurrent workflow: %v", err)
	}
	versionID := seedRuntimeRequest(t, db, ids, published.ID, seedRuntimePlan(t, db, ids))
	repository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		t.Fatalf("create concurrent runtime repository: %v", err)
	}
	service := runtimeService(t, repository)
	if _, err := service.Start(ctx, deployapp.WorkflowStartInput{
		RequestVersionID: versionID, IdempotencyKey: "concurrent-runtime-start", RequestID: "concurrent-runtime",
	}); err != nil {
		t.Fatalf("start concurrent workflow runtime: %v", err)
	}
	snapshot, err := repository.Load(ctx, versionID)
	if err != nil || snapshot.Instance == nil {
		t.Fatalf("load concurrent workflow runtime: %#v %v", snapshot, err)
	}
	return workflowConcurrencyFixture{db, repository, service, deployapp.WorkflowPrincipal{UserID: actorID}, snapshot}
}

func assertConcurrentReviewEffects(t *testing.T, fixture workflowConcurrencyFixture, key string) {
	t.Helper()
	taskID := fixture.snapshot.CurrentReview.ID
	actorID := fixture.principal.UserID
	versionID := fixture.snapshot.RequestVersionID
	assertConcurrencyCount(t, fixture.db, "deployment_review_decisions", 1,
		"review_task_id = ?", taskID)
	assertConcurrencyCount(t, fixture.db, "deployment_review_decisions", 1,
		"review_task_id = ? AND reviewer_id = ? AND decision = 'Approve' AND idempotency_key = ?", taskID, actorID, key)
	assertConcurrencyCount(t, fixture.db, "audit_logs", 1,
		`action = 'deployment_review.decide' AND resource_type = 'deployment_review_task'
		 AND resource_id = ? AND actor_id = ? AND request_id = 'concurrent-runtime'
		 AND metadata->>'decision' = 'Approve' AND metadata->>'status' = 'Approved'`, taskID.String(), actorID)
	assertConcurrencyCount(t, fixture.db, "outbox_events", 1,
		`event_type = 'deployment.review.decided' AND aggregate_type = 'deployment_request_version'
		 AND aggregate_id = ? AND payload->>'requestVersionId' = ?
		 AND payload->>'reviewTaskId' = ? AND payload->>'status' = 'Approved'`, versionID.String(), versionID.String(), taskID.String())
	snapshot, err := fixture.repository.Load(t.Context(), versionID)
	if err != nil || snapshot.CurrentReview == nil || snapshot.CurrentReview.ID != taskID ||
		snapshot.CurrentReview.Status != deploydomain.ReviewTaskApproved || len(snapshot.CurrentReview.Decisions) != 1 {
		t.Fatalf("reload approved concurrent review: %#v %v", snapshot, err)
	}
}

func assertConcurrentTransitionEffects(t *testing.T, fixture workflowConcurrencyFixture, key string) {
	t.Helper()
	versionID := fixture.snapshot.RequestVersionID
	instance := fixture.snapshot.Instance
	assertConcurrencyCount(t, fixture.db, "deployment_workflow_transitions", 1,
		"workflow_instance_id = ?", instance.ID)
	assertConcurrencyCount(t, fixture.db, "deployment_workflow_transitions", 1,
		`workflow_instance_id = ? AND request_version_id = ? AND from_state_key = ?
		 AND to_state_key = 'deploying' AND transition_key = 'deploy' AND actor_id = ? AND idempotency_key = ?`,
		instance.ID, versionID, instance.CurrentStateKey, fixture.principal.UserID, key)
	assertConcurrencyCount(t, fixture.db, "deployment_jobs", 1, "aggregate_id = ?", versionID)
	assertConcurrencyCount(t, fixture.db, "deployment_jobs", 1,
		`job_type = 'ExecuteDeployment' AND aggregate_type = 'deployment_request_version' AND aggregate_id = ?
		 AND idempotency_key = ? AND status = 'Pending' AND payload->>'requestVersionId' = ?
		 AND payload->>'workflowInstanceId' = ? AND payload->>'workflowStateKey' = 'deploying'`,
		versionID, "workflow-deployment:"+key, versionID.String(), instance.ID.String())
	assertConcurrencyCount(t, fixture.db, "audit_logs", 1,
		`action = 'deployment_workflow.transition' AND resource_type = 'deployment_request_version'
		 AND resource_id = ? AND actor_id = ? AND request_id = 'concurrent-runtime' AND metadata->>'stateKey' = 'deploying'`,
		versionID.String(), fixture.principal.UserID)
	assertConcurrencyCount(t, fixture.db, "outbox_events", 1,
		`event_type = 'deployment.workflow.advanced' AND aggregate_type = 'deployment_request_version'
		 AND aggregate_id = ? AND payload->>'requestVersionId' = ? AND payload->>'stateKey' = 'deploying'`,
		versionID.String(), versionID.String())
	snapshot, err := fixture.repository.Load(t.Context(), versionID)
	if err != nil || snapshot.Instance == nil || snapshot.Instance.ID != instance.ID ||
		snapshot.Instance.WorkflowVersionID != instance.WorkflowVersionID || snapshot.Instance.CurrentStateKey != "deploying" ||
		snapshot.Instance.Status != deploydomain.WorkflowInstanceRunning || snapshot.Instance.LockVersion != instance.LockVersion+1 {
		t.Fatalf("reload advanced concurrent workflow: %#v %v", snapshot, err)
	}
	assertConcurrencyCount(t, fixture.db, "deployment_request_versions", 1,
		"id = ? AND lock_version = ?", versionID, snapshot.Instance.LockVersion)
}

func assertConcurrencyCount(t *testing.T, db *gorm.DB, table string, want int64, query string, args ...any) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where(query, args...).Count(&count).Error; err != nil || count != want {
		t.Fatalf("%s count = %d, want %d, error = %v", table, count, want, err)
	}
}
