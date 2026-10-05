//go:build integration

package database_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

type reviewAtomicityOptions struct {
	automatic bool
	quorum    bool
	unmatched bool
}

type reviewAtomicityFixture struct {
	db         *gorm.DB
	repository *deployinfra.WorkflowRuntimeRepository
	service    *deployapp.WorkflowRuntimeService
	snapshot   deployapp.WorkflowRuntimeSnapshot
	reviewerID uuid.UUID
}

func newReviewAtomicityFixture(t *testing.T, options reviewAtomicityOptions) reviewAtomicityFixture {
	t.Helper()
	db := workflowTestDatabase(t, t.Context())
	ids := seedDeploymentSchemaScope(t, db)
	reviewerID := seedConcurrencyActor(t, db, "review-atomicity-reviewer")
	document := runtimeIntegrationDocument(reviewerID)
	if options.quorum {
		secondReviewer := seedConcurrencyActor(t, db, "review-atomicity-second-reviewer")
		document.States[0].ReviewPolicy.Type = deploydomain.ReviewPolicyMinimum
		document.States[0].ReviewPolicy.RequiredApprovals = 2
		document.States[0].ReviewPolicy.UserIDs = append(document.States[0].ReviewPolicy.UserIDs, secondReviewer)
	}
	if !options.automatic {
		document.Transitions = nil
	}
	if options.unmatched {
		document.Transitions[0].Conditions[0].Values = []string{"Rejected"}
	}
	version := seedReviewAtomicityWorkflow(t, db, reviewerID, document)
	versionID := seedRuntimeRequest(t, db, ids, version.ID, seedRuntimePlan(t, db, ids))
	repository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		t.Fatalf("create atomic review repository: %v", err)
	}
	service := runtimeService(t, repository)
	if _, err := service.Start(t.Context(), deployapp.WorkflowStartInput{
		RequestVersionID: versionID, IdempotencyKey: "review-atomicity-start", RequestID: "review-atomicity-start",
	}); err != nil {
		t.Fatalf("start atomic review workflow: %v", err)
	}
	snapshot, err := repository.Load(t.Context(), versionID)
	if err != nil || snapshot.Instance == nil || snapshot.CurrentReview == nil || snapshot.CurrentReview.Status != deploydomain.ReviewTaskPending {
		t.Fatalf("load pending atomic review: %#v, %v", snapshot, err)
	}
	return reviewAtomicityFixture{db: db, repository: repository, service: service, snapshot: snapshot, reviewerID: reviewerID}
}

func seedReviewAtomicityWorkflow(t *testing.T, db *gorm.DB, actorID uuid.UUID, document deploydomain.WorkflowDocument) deploydomain.ReleaseWorkflowVersion {
	t.Helper()
	repository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create atomic review definition repository: %v", err)
	}
	workflow, err := deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: "Atomic review", CreatedBy: actorID, CreatedAt: runtimeIntegrationTime(),
	}, document)
	if err != nil {
		t.Fatalf("create atomic review workflow: %v", err)
	}
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "review-atomicity-seed", OccurredAt: runtimeIntegrationTime()}
	if err := repository.Create(t.Context(), mutation, workflow); err != nil {
		t.Fatalf("persist atomic review workflow: %v", err)
	}
	published := publishWorkflowVersion(t, workflow.Versions[0], runtimeIntegrationTime())
	if err := repository.UpdateLifecycle(t.Context(), mutation, 1, published); err != nil {
		t.Fatalf("publish atomic review workflow: %v", err)
	}
	return published
}

func (fixture reviewAtomicityFixture) principal() deployapp.WorkflowPrincipal {
	return deployapp.WorkflowPrincipal{UserID: fixture.reviewerID}
}

func (fixture reviewAtomicityFixture) reviewInput(key string) deployapp.WorkflowReviewInput {
	return deployapp.WorkflowReviewInput{
		RequestVersionID: fixture.snapshot.RequestVersionID, ReviewTaskID: fixture.snapshot.CurrentReview.ID,
		ExpectedLock: fixture.snapshot.Instance.LockVersion, Decision: deploydomain.ReviewDecisionApprove,
		IdempotencyKey: key, RequestID: key,
	}
}

func assertReviewAtomicityProjection(t *testing.T, fixture reviewAtomicityFixture, stateKey string, status deploydomain.ReviewTaskStatus) {
	t.Helper()
	snapshot, err := fixture.repository.Load(t.Context(), fixture.snapshot.RequestVersionID)
	if err != nil || snapshot.Instance == nil || snapshot.CurrentReview == nil {
		t.Fatalf("reload atomic review: %#v, %v", snapshot, err)
	}
	instance := snapshot.Instance
	if instance.ID != fixture.snapshot.Instance.ID || instance.WorkflowVersionID != fixture.snapshot.Version.ID ||
		instance.CurrentStateKey != stateKey || instance.Status != deploydomain.WorkflowInstanceRunning ||
		instance.LockVersion != fixture.snapshot.Instance.LockVersion+1 || instance.CompletedAt != nil {
		t.Fatalf("atomic review instance = %#v", instance)
	}
	if snapshot.CurrentReview.ID != fixture.snapshot.CurrentReview.ID || snapshot.CurrentReview.Status != status || len(snapshot.CurrentReview.Decisions) != 1 ||
		snapshot.CurrentReview.StateKey != fixture.snapshot.CurrentReview.StateKey || snapshot.CurrentReview.StageNumber != fixture.snapshot.CurrentReview.StageNumber ||
		!reflect.DeepEqual(snapshot.CurrentReview.Policy, fixture.snapshot.CurrentReview.Policy) ||
		!reflect.DeepEqual(snapshot.CurrentReview.Assignment, fixture.snapshot.CurrentReview.Assignment) {
		t.Fatalf("atomic review task = %#v", snapshot.CurrentReview)
	}
	requestRepository, err := deployinfra.NewDeploymentRequestRepository(fixture.db)
	if err != nil {
		t.Fatalf("create atomic review request repository: %v", err)
	}
	detail, err := requestRepository.Load(t.Context(), snapshot.RequestID)
	if err != nil || len(detail.Reviews) != 1 || detail.Reviews[0].ID != snapshot.CurrentReview.ID ||
		detail.Reviews[0].Status != status || detail.Version.LockVersion != instance.LockVersion {
		t.Fatalf("atomic request review projection = %#v, %v", detail, err)
	}
	var projection struct{ ClosedAt *time.Time }
	if err := fixture.db.Table("deployment_review_tasks").Select("closed_at").Where("id = ?", snapshot.CurrentReview.ID).Take(&projection).Error; err != nil {
		t.Fatalf("load atomic review close time: %v", err)
	}
	closedAt := projection.ClosedAt
	if status == deploydomain.ReviewTaskPending {
		if closedAt != nil {
			t.Fatalf("pending review closed_at = %s", closedAt)
		}
	} else if closedAt == nil || !closedAt.Equal(runtimeIntegrationTime()) {
		t.Fatalf("approved review closed_at = %v", closedAt)
	}
}

func assertReviewAtomicityDecision(t *testing.T, fixture reviewAtomicityFixture, input deployapp.WorkflowReviewInput, status deploydomain.ReviewTaskStatus) {
	t.Helper()
	assertCountWhere(t, fixture.db, "deployment_review_decisions", "review_task_id = ?", input.ReviewTaskID, 1)
	assertReviewAtomicityCount(t, fixture.db, "deployment_review_decisions", 1,
		`review_task_id = ? AND reviewer_id = ? AND decision = ? AND idempotency_key = ? AND decided_at = ?
		AND permission_snapshot = '{"permission":"deployment_request.review","authorized":true}'::jsonb`,
		input.ReviewTaskID, fixture.reviewerID, "Approve", input.IdempotencyKey, runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 1,
		`request_id = ? AND action = ? AND resource_type = ? AND resource_id = ? AND actor_id = ?
		AND metadata = jsonb_build_object('decision', ?::text, 'status', ?::text) AND occurred_at = ?`,
		input.RequestID, "deployment_review.decide", "deployment_review_task", input.ReviewTaskID.String(), fixture.reviewerID, "Approve", string(status), runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 1,
		`event_type = ? AND aggregate_type = ? AND aggregate_id = ?
		AND payload = jsonb_build_object('requestVersionId', ?::text, 'reviewTaskId', ?::text, 'status', ?::text) AND occurred_at = ?`,
		"deployment.review.decided", "deployment_request_version", input.RequestVersionID.String(), input.RequestVersionID.String(), input.ReviewTaskID.String(), string(status), runtimeIntegrationTime())
}

func assertReviewAtomicityDeployment(t *testing.T, fixture reviewAtomicityFixture, input deployapp.WorkflowReviewInput) {
	t.Helper()
	assertCountWhere(t, fixture.db, "deployment_workflow_transitions", "workflow_instance_id = ?", fixture.snapshot.Instance.ID, 1)
	assertReviewAtomicityCount(t, fixture.db, "deployment_workflow_transitions", 1,
		"workflow_instance_id = ? AND request_version_id = ? AND from_state_key = ? AND to_state_key = ? AND transition_key = ? AND idempotency_key = ? AND actor_id = ? AND occurred_at = ?",
		fixture.snapshot.Instance.ID, input.RequestVersionID, "review", "deploying", "deploy", input.IdempotencyKey, fixture.reviewerID, runtimeIntegrationTime())
	assertCountWhere(t, fixture.db, "deployment_jobs", "aggregate_id = ?", input.RequestVersionID, 1)
	assertReviewAtomicityCount(t, fixture.db, "deployment_jobs", 1,
		`job_type = ? AND aggregate_type = ? AND aggregate_id = ? AND status = ? AND idempotency_key = ? AND available_at = ? AND max_attempts = ?
		AND payload = jsonb_build_object('requestVersionId', ?::text, 'workflowInstanceId', ?::text, 'workflowStateKey', ?::text)
		AND created_at = ? AND updated_at = ? AND attempts = 0 AND fencing_token = 0 AND lease_owner IS NULL AND lease_expires_at IS NULL`,
		"ExecuteDeployment", "deployment_request_version", input.RequestVersionID, "Pending", "workflow-deployment:"+input.IdempotencyKey, runtimeIntegrationTime(), 10,
		input.RequestVersionID.String(), fixture.snapshot.Instance.ID.String(), "deploying", runtimeIntegrationTime(), runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 1,
		`request_id = ? AND action = ? AND resource_type = ? AND resource_id = ? AND actor_id = ?
		AND metadata = jsonb_build_object('stateKey', ?::text) AND occurred_at = ?`,
		input.RequestID, "deployment_workflow.transition", "deployment_request_version", input.RequestVersionID.String(), fixture.reviewerID, "deploying", runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 1,
		`event_type = ? AND aggregate_type = ? AND aggregate_id = ?
		AND payload = jsonb_build_object('requestVersionId', ?::text, 'stateKey', ?::text) AND occurred_at = ?`,
		"deployment.workflow.advanced", "deployment_request_version", input.RequestVersionID.String(), input.RequestVersionID.String(), "deploying", runtimeIntegrationTime())
	assertCountWhere(t, fixture.db, "audit_logs", "request_id = ?", input.RequestID, 2)
	assertCountWhere(t, fixture.db, "outbox_events", "aggregate_id = ?", input.RequestVersionID.String(), 3)
}

func assertReviewAtomicityNoDecision(t *testing.T, fixture reviewAtomicityFixture, input deployapp.WorkflowReviewInput) {
	t.Helper()
	assertCountWhere(t, fixture.db, "deployment_review_decisions", "review_task_id = ?", input.ReviewTaskID, 0)
	assertCountWhere(t, fixture.db, "audit_logs", "request_id = ?", input.RequestID, 0)
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 0, "event_type = ? AND aggregate_id = ?", "deployment.review.decided", input.RequestVersionID.String())
}

func assertReviewAtomicityNoDeployment(t *testing.T, fixture reviewAtomicityFixture, input deployapp.WorkflowReviewInput) {
	t.Helper()
	assertCountWhere(t, fixture.db, "deployment_workflow_transitions", "workflow_instance_id = ?", fixture.snapshot.Instance.ID, 0)
	assertCountWhere(t, fixture.db, "deployment_jobs", "aggregate_id = ?", input.RequestVersionID, 0)
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 0, "request_id = ? AND action = ?", input.RequestID, "deployment_workflow.transition")
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 0,
		"event_type = ? AND aggregate_id = ? AND payload ->> 'stateKey' = ?", "deployment.workflow.advanced", input.RequestVersionID.String(), "deploying")
}

func assertReviewAtomicityCount(t *testing.T, db *gorm.DB, table string, want int64, predicate string, args ...any) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where(predicate, args...).Count(&count).Error; err != nil {
		t.Fatalf("count atomic review %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("atomic review %s matching %q = %d, want %d", table, predicate, count, want)
	}
}

type reviewAtomicityState struct {
	Version     string
	Instance    string
	Task        string
	Decisions   string
	Transitions string
	Jobs        string
	Audits      string
	Outbox      string
}

func loadReviewAtomicityState(t *testing.T, fixture reviewAtomicityFixture) reviewAtomicityState {
	t.Helper()
	var state reviewAtomicityState
	err := fixture.db.Raw(`SELECT
		(SELECT to_jsonb(v)::text FROM deployment_request_versions v WHERE id = ?) AS version,
		(SELECT to_jsonb(i)::text FROM deployment_workflow_instances i WHERE id = ?) AS instance,
		(SELECT to_jsonb(t)::text FROM deployment_review_tasks t WHERE id = ?) AS task,
		(SELECT COALESCE(jsonb_agg(to_jsonb(d) ORDER BY id), '[]'::jsonb)::text FROM deployment_review_decisions d WHERE review_task_id = ?) AS decisions,
		(SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY id), '[]'::jsonb)::text FROM deployment_workflow_transitions r WHERE request_version_id = ?) AS transitions,
		(SELECT COALESCE(jsonb_agg(to_jsonb(j) ORDER BY id), '[]'::jsonb)::text FROM deployment_jobs j WHERE aggregate_id = ?) AS jobs,
		(SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY id), '[]'::jsonb)::text FROM audit_logs a WHERE resource_id IN (?, ?)) AS audits,
		(SELECT COALESCE(jsonb_agg(to_jsonb(o) ORDER BY id), '[]'::jsonb)::text FROM outbox_events o WHERE aggregate_id = ?) AS outbox`,
		fixture.snapshot.RequestVersionID, fixture.snapshot.Instance.ID, fixture.snapshot.CurrentReview.ID,
		fixture.snapshot.CurrentReview.ID, fixture.snapshot.RequestVersionID, fixture.snapshot.RequestVersionID,
		fixture.snapshot.RequestVersionID.String(), fixture.snapshot.CurrentReview.ID.String(), fixture.snapshot.RequestVersionID.String()).Scan(&state).Error
	if err != nil || state.Version == "" || state.Instance == "" || state.Task == "" {
		t.Fatalf("load atomic review persisted state = %#v, %v", state, err)
	}
	return state
}

func installReviewAtomicityRejection(t *testing.T, db *gorm.DB, table, condition string) {
	t.Helper()
	if err := db.Exec(`CREATE FUNCTION reject_review_atomicity() RETURNS trigger AS $$
	BEGIN
		RAISE EXCEPTION 'review atomicity failure injected';
	END;
	$$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create atomic review rejection function: %v", err)
	}
	query := fmt.Sprintf(`CREATE TRIGGER reject_review_atomicity BEFORE INSERT ON %s
		FOR EACH ROW WHEN (%s) EXECUTE FUNCTION reject_review_atomicity()`, table, condition)
	if err := db.Exec(query).Error; err != nil {
		t.Fatalf("create atomic review rejection trigger: %v", err)
	}
}
