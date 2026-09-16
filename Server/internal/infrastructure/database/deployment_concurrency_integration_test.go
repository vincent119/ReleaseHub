//go:build integration

package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestWorkflowVersionConcurrentAppendCreatesOneDraft(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	actorID := seedConcurrencyActor(t, db, "workflow-concurrency")
	repository, _ := deployinfra.NewWorkflowDefinitionRepository(db)
	now := time.Date(2026, 9, 16, 6, 0, 0, 0, time.UTC)
	workflow := workflowAggregateNamed(t, actorID, now, "Concurrent workflow")
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "workflow-create", OccurredAt: now}
	if err := repository.Create(ctx, mutation, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	published := publishWorkflowVersion(t, workflow.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish workflow: %v", err)
	}

	blocker := holdRowLock(t, db, "release_workflows", workflow.ID)
	versions := []deploydomain.ReleaseWorkflowVersion{
		workflowVersion(t, workflow.ID, 2, actorID, now.Add(time.Minute)),
		workflowVersion(t, workflow.ID, 2, actorID, now.Add(time.Minute)),
	}
	errorsFound := runConcurrentErrors(t, 2, func(index int) error {
		change := deployapp.WorkflowMutation{
			ActorID: actorID, RequestID: fmt.Sprintf("workflow-append-%d", index), OccurredAt: now.Add(time.Minute),
		}
		return repository.AppendVersion(ctx, change, 1, versions[index])
	})
	waitForBlockedQueries(t, db, "release_workflows", 2)
	commitBlocker(t, blocker)
	assertOneSuccessOneConflict(t, errorsFound, deployapp.ErrWorkflowVersionConflict)
	assertDefinitionConcurrencyResult(t, db, "release_workflow_versions", "workflow_id", workflow.ID)
}

func TestDeploymentPlanVersionConcurrentAppendCreatesOneDraft(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	repository, _ := deployinfra.NewDeploymentPlanRepository(db)
	now := time.Date(2026, 9, 16, 7, 0, 0, 0, time.UTC)
	plan := deploymentPlanAggregate(t, ids.projectID, ids.userID, now)
	mutation := deployapp.PlanMutation{ActorID: ids.userID, RequestID: "plan-create", OccurredAt: now}
	if err := repository.Create(ctx, mutation, plan); err != nil {
		t.Fatalf("create Plan: %v", err)
	}
	published := publishDeploymentPlanVersion(t, plan.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish Plan: %v", err)
	}

	blocker := holdRowLock(t, db, "deployment_plans", plan.ID)
	versions := []deploydomain.DeploymentPlanVersion{
		deploymentPlanVersion(t, plan.ID, 2, ids.userID, now.Add(time.Minute)),
		deploymentPlanVersion(t, plan.ID, 2, ids.userID, now.Add(time.Minute)),
	}
	errorsFound := runConcurrentErrors(t, 2, func(index int) error {
		change := deployapp.PlanMutation{
			ActorID: ids.userID, RequestID: fmt.Sprintf("plan-append-%d", index), OccurredAt: now.Add(time.Minute),
		}
		return repository.AppendVersion(ctx, change, 1, versions[index])
	})
	waitForBlockedQueries(t, db, "deployment_plans", 2)
	commitBlocker(t, blocker)
	assertOneSuccessOneConflict(t, errorsFound, deployapp.ErrPlanConflict)
	assertDefinitionConcurrencyResult(t, db, "deployment_plan_versions", "plan_id", plan.ID)
}

func TestCandidateConcurrentDuplicateCreatesOneRequest(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, _ := deployinfra.NewCandidateRepository(db)
	observation := candidateObservationFixture(t, ids.applicationID, ids)
	key := fmt.Sprintf("%s:%s:%s:%s:%s", observation.OrganizationID, observation.ProjectID,
		observation.EnvironmentID, observation.WorkflowVersionID, observation.PlanVersionID)
	blocker := db.Begin()
	if err := blocker.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, key).Error; err != nil {
		t.Fatalf("hold candidate group lock: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback().Error })

	results := runConcurrentCandidateCreates(t, repository, observation)
	waitForBlockedQueries(t, db, "pg_advisory_xact_lock", 2)
	commitBlocker(t, blocker)
	assertCandidateCreateResults(t, results)
	startPendingCandidateWorkflow(t, ctx, db, repository)
	assertCandidateConcurrencyCounts(t, db)
}

func TestWorkflowAppendRollsBackVersionAuditAndOutbox(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	actorID := seedConcurrencyActor(t, db, "workflow-rollback")
	repository, _ := deployinfra.NewWorkflowDefinitionRepository(db)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	workflow := workflowAggregateNamed(t, actorID, now, "Rollback workflow")
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "workflow-rollback", OccurredAt: now}
	if err := repository.Create(ctx, mutation, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	published := publishWorkflowVersion(t, workflow.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	installRejectingOutboxTrigger(t, db)
	auditBefore, outboxBefore := tableCount(t, db, "audit_logs"), tableCount(t, db, "outbox_events")
	err := repository.AppendVersion(ctx, mutation, 1, workflowVersion(t, workflow.ID, 2, actorID, now.Add(time.Minute)))
	if err == nil {
		t.Fatal("append workflow version should fail when Outbox insert is rejected")
	}
	assertCountWhere(t, db, "release_workflow_versions", "workflow_id = ?", workflow.ID, 1)
	if tableCount(t, db, "audit_logs") != auditBefore || tableCount(t, db, "outbox_events") != outboxBefore {
		t.Fatal("failed Workflow transaction left Audit or Outbox records")
	}
}

func TestWorkflowRuntimeConcurrentCommandsCreateOneBusinessResult(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	reviewerID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'concurrent-reviewer')`, reviewerID)
	workflowVersion := seedRuntimeWorkflow(t, ctx, db, reviewerID)
	planVersionID := seedRuntimePlan(t, db, ids)
	requestVersionID := seedRuntimeRequest(t, db, ids, workflowVersion.ID, planVersionID)
	repository, _ := deployinfra.NewWorkflowRuntimeRepository(db)
	service := runtimeService(t, repository)
	if _, err := service.Start(ctx, deployapp.WorkflowStartInput{
		RequestVersionID: requestVersionID, IdempotencyKey: "concurrent-runtime-start", RequestID: "concurrent-runtime",
	}); err != nil {
		t.Fatalf("start concurrent workflow runtime: %v", err)
	}

	snapshot, err := repository.Load(ctx, requestVersionID)
	if err != nil || snapshot.CurrentReview == nil {
		t.Fatalf("load concurrent workflow review: %#v %v", snapshot, err)
	}
	auditsBefore, outboxBefore := tableCount(t, db, "audit_logs"), tableCount(t, db, "outbox_events")
	reviewBlocker := holdRowLock(t, db, "deployment_request_versions", requestVersionID)
	reviewErrors := runConcurrentErrors(t, 2, func(_ int) error {
		_, decideErr := service.DecideReview(ctx, deployapp.WorkflowPrincipal{UserID: reviewerID}, deployapp.WorkflowReviewInput{
			RequestVersionID: requestVersionID, ReviewTaskID: snapshot.CurrentReview.ID,
			ExpectedLock: snapshot.Instance.LockVersion, Decision: deploydomain.ReviewDecisionApprove,
			IdempotencyKey: "concurrent-review", RequestID: "concurrent-runtime",
		})
		return decideErr
	})
	waitForBlockedQueries(t, db, "deployment_request_versions", 2)
	commitBlocker(t, reviewBlocker)
	assertOneSuccessOneConflict(t, reviewErrors, deployapp.ErrWorkflowRuntimeConflict)
	assertCountWhere(t, db, "deployment_review_decisions", "idempotency_key = ?", "concurrent-review", 1)
	assertTableCountDelta(t, db, "audit_logs", auditsBefore, 1)
	assertTableCountDelta(t, db, "outbox_events", outboxBefore, 1)

	snapshot, err = repository.Load(ctx, requestVersionID)
	if err != nil || snapshot.CurrentReview == nil {
		t.Fatalf("reload concurrent workflow review: %#v %v", snapshot, err)
	}
	auditsBefore, outboxBefore = tableCount(t, db, "audit_logs"), tableCount(t, db, "outbox_events")
	transitionBlocker := holdRowLock(t, db, "deployment_request_versions", requestVersionID)
	transitionErrors := runConcurrentErrors(t, 2, func(_ int) error {
		_, transitionErr := service.Transition(ctx, deployapp.WorkflowPrincipal{UserID: reviewerID}, deployapp.WorkflowTransitionInput{
			RequestVersionID: requestVersionID, ExpectedLock: snapshot.Instance.LockVersion,
			TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerReviewSatisfied,
			Facts:          map[deploydomain.WorkflowFact]string{deploydomain.WorkflowFactReviewStatus: "Approved"},
			IdempotencyKey: "concurrent-transition", RequestID: "concurrent-runtime",
		})
		return transitionErr
	})
	waitForBlockedQueries(t, db, "deployment_request_versions", 2)
	commitBlocker(t, transitionBlocker)
	assertOneSuccessOneConflict(t, transitionErrors, deployapp.ErrWorkflowRuntimeConflict)
	assertCountWhere(t, db, "deployment_workflow_transitions", "idempotency_key = ?", "concurrent-transition", 1)
	assertCountWhere(t, db, "deployment_jobs", "idempotency_key = ?", "workflow-deployment:concurrent-transition", 1)
	assertTableCountDelta(t, db, "audit_logs", auditsBefore, 1)
	assertTableCountDelta(t, db, "outbox_events", outboxBefore, 1)
}

func TestExecutionRetryConcurrentIdempotencyCreatesOneCommand(t *testing.T) {
	ctx, db, repository, execution, applications := partialExecutionFixture(t)
	if _, err := repository.Complete(ctx, execution.ID, time.Now().UTC()); err != nil {
		t.Fatalf("complete concurrent retry execution: %v", err)
	}
	var lockVersion uint64
	if err := db.Table("deployment_executions").Select("lock_version").Where("id = ?", execution.ID).Scan(&lockVersion).Error; err != nil {
		t.Fatalf("load concurrent retry version: %v", err)
	}
	actorID := deploymentActor(t, db)
	change := deployapp.ExecutionRetryChange{
		Mutation: deployapp.ExecutionMutation{
			ActorID: actorID, RequestID: "concurrent-retry", IdempotencyKey: "concurrent-retry",
			RequestHash: fmt.Sprintf("%064x", 1), OccurredAt: time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC),
		},
		WorkflowResult: executionWorkflowAction(t, db, execution.RequestVersionID, executionWorkflowActionInput{
			permission: "deployment_request.retry", status: "PartialFailed", actorID: actorID,
		}),
		RequestID: uuid.New(), RequestVersionID: execution.RequestVersionID,
		ExecutionID: execution.ID, ExpectedVersion: lockVersion, ApplicationIDs: []uuid.UUID{applications[1]},
	}
	auditsBefore, outboxBefore := tableCount(t, db, "audit_logs"), tableCount(t, db, "outbox_events")
	blocker := holdRowLock(t, db, "deployment_executions", execution.ID)
	errorsFound := runConcurrentErrors(t, 2, func(_ int) error {
		_, retryErr := repository.Retry(ctx, change)
		return retryErr
	})
	waitForBlockedQueries(t, db, "deployment_executions", 2)
	commitBlocker(t, blocker)
	assertOneSuccessOneConflict(t, errorsFound, deployapp.ErrExecutionConflict)
	assertCountWhere(t, db, "deployment_execution_commands", "idempotency_key = ?", "concurrent-retry", 1)
	assertCountWhere(t, db, "deployment_jobs", "idempotency_key = ?", "business-retry:"+execution.RequestVersionID.String()+":concurrent-retry", 1)
	assertTableCountDelta(t, db, "audit_logs", auditsBefore, 2)
	assertTableCountDelta(t, db, "outbox_events", outboxBefore, 2)

	replayed, found, err := repository.Replay(ctx, deployapp.ExecutionCommandReplay{
		RequestVersionID: execution.RequestVersionID, Command: "Retry",
		IdempotencyKey: "concurrent-retry", RequestHash: fmt.Sprintf("%064x", 1),
	})
	if err != nil || !found || replayed.Attempt != 2 {
		t.Fatalf("replay concurrent retry = %#v, %t, %v", replayed, found, err)
	}
	_, found, err = repository.Replay(ctx, deployapp.ExecutionCommandReplay{
		RequestVersionID: execution.RequestVersionID, Command: "Retry",
		IdempotencyKey: "concurrent-retry", RequestHash: fmt.Sprintf("%064x", 2),
	})
	if !errors.Is(err, deployapp.ErrExecutionConflict) || found {
		t.Fatalf("different payload replay = found %t error %v", found, err)
	}
}

type candidateCreateResult struct {
	created bool
	err     error
}

func runConcurrentErrors(t *testing.T, count int, work func(int) error) <-chan error {
	t.Helper()
	start := make(chan struct{})
	ready := make(chan struct{}, count)
	results := make(chan error, count)
	for index := 0; index < count; index++ {
		go func(index int) {
			ready <- struct{}{}
			<-start
			results <- work(index)
		}(index)
	}
	for index := 0; index < count; index++ {
		<-ready
	}
	close(start)
	return results
}

func runConcurrentCandidateCreates(t *testing.T, repository *deployinfra.CandidateRepository, observation deploydomain.CandidateObservation) <-chan candidateCreateResult {
	t.Helper()
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan candidateCreateResult, 2)
	for range 2 {
		go func() {
			ready <- struct{}{}
			<-start
			created, err := repository.CreateDeploymentRequest(context.Background(), observation)
			results <- candidateCreateResult{created: created, err: err}
		}()
	}
	<-ready
	<-ready
	close(start)
	return results
}

func holdRowLock(t *testing.T, db *gorm.DB, table string, id uuid.UUID) *gorm.DB {
	t.Helper()
	transaction := db.Begin()
	if transaction.Error != nil {
		t.Fatalf("begin blocker transaction: %v", transaction.Error)
	}
	query := fmt.Sprintf(`SELECT id FROM %s WHERE id = ? FOR UPDATE`, table)
	if err := transaction.Exec(query, id).Error; err != nil {
		t.Fatalf("hold %s row lock: %v", table, err)
	}
	t.Cleanup(func() { _ = transaction.Rollback().Error })
	return transaction
}

func waitForBlockedQueries(t *testing.T, db *gorm.DB, queryFragment string, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		err := db.Raw(`SELECT count(*) FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid()
  AND wait_event_type = 'Lock' AND query ILIKE ?`, "%"+queryFragment+"%").Scan(&count).Error
		if err != nil {
			t.Fatalf("inspect blocked PostgreSQL queries: %v", err)
		}
		if count >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("blocked query count for %q did not reach %d", queryFragment, want)
}

func commitBlocker(t *testing.T, transaction *gorm.DB) {
	t.Helper()
	if err := transaction.Commit().Error; err != nil {
		t.Fatalf("release blocker transaction: %v", err)
	}
}

func assertOneSuccessOneConflict(t *testing.T, results <-chan error, conflict error) {
	t.Helper()
	succeeded, conflicted := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, conflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent mutation error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results = success %d conflict %d", succeeded, conflicted)
	}
}

func assertDefinitionConcurrencyResult(t *testing.T, db *gorm.DB, table, foreignKey string, id uuid.UUID) {
	t.Helper()
	assertCountWhere(t, db, table, foreignKey+" = ?", id, 2)
	assertCountWhere(t, db, table, foreignKey+" = ? AND lifecycle = 'Draft'", id, 1)
	assertCountWhere(t, db, "audit_logs", "resource_id = ?", id.String(), 3)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", id.String(), 3)
}

func assertCandidateCreateResults(t *testing.T, results <-chan candidateCreateResult) {
	t.Helper()
	created, duplicate := 0, 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent Candidate creation: %v", result.err)
		}
		if result.created {
			created++
		} else {
			duplicate++
		}
	}
	if created != 1 || duplicate != 1 {
		t.Fatalf("Candidate results = created %d duplicate %d", created, duplicate)
	}
}

func assertCandidateConcurrencyCounts(t *testing.T, db *gorm.DB) {
	t.Helper()
	for table, want := range map[string]int64{
		"deployment_candidate_observations": 1,
		"deployment_requests":               1,
		"deployment_request_versions":       1,
		"deployment_workflow_instances":     1,
		"audit_logs":                        2,
		"outbox_events":                     2,
	} {
		if count := tableCount(t, db, table); count != want {
			t.Fatalf("%s count = %d, want %d", table, count, want)
		}
	}
}

func assertTableCountDelta(t *testing.T, db *gorm.DB, table string, before, delta int64) {
	t.Helper()
	if count := tableCount(t, db, table); count != before+delta {
		t.Fatalf("%s count = %d, want %d", table, count, before+delta)
	}
}

func seedConcurrencyActor(t *testing.T, db *gorm.DB, username string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, ?)`, id, username).Error; err != nil {
		t.Fatalf("seed concurrency actor: %v", err)
	}
	return id
}

func installRejectingOutboxTrigger(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE FUNCTION reject_concurrency_outbox() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'outbox failure injected by concurrency integration test';
END;
$$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create rejecting Outbox function: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_concurrency_outbox
BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_concurrency_outbox()`).Error; err != nil {
		t.Fatalf("create rejecting Outbox trigger: %v", err)
	}
}
