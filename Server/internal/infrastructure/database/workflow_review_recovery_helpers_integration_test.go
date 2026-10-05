//go:build integration

package database_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

func newReviewRecoveryFixture(t *testing.T, documentFor func(uuid.UUID) deploydomain.WorkflowDocument) reviewAtomicityFixture {
	t.Helper()
	fixture := workflowConcurrencyFixtureFor(t, documentFor)
	decision := deploydomain.ReviewDecision{
		ReviewerID: fixture.principal.UserID, Decision: deploydomain.ReviewDecisionApprove, DecidedAt: runtimeIntegrationTime(),
	}
	task, err := fixture.snapshot.CurrentReview.Decide(decision)
	if err != nil {
		t.Fatalf("create historical approval: %v", err)
	}
	// Historical fixtures reproduce the legacy decision-only write in an isolated test database.
	err = fixture.repository.ApplyReview(t.Context(), deployapp.WorkflowReviewChange{
		Mutation: deployapp.WorkflowRuntimeMutation{
			ActorID: fixture.principal.UserID, RequestID: "legacy-review-approval",
			IdempotencyKey: "legacy-review-approval", OccurredAt: runtimeIntegrationTime(),
		},
		ExpectedLock: fixture.snapshot.Instance.LockVersion, Task: task, Decision: decision,
	})
	if err != nil {
		t.Fatalf("persist historical approval: %v", err)
	}
	snapshot, err := fixture.repository.Load(t.Context(), fixture.snapshot.RequestVersionID)
	if err != nil || snapshot.CurrentReview == nil || snapshot.CurrentReview.Status != deploydomain.ReviewTaskApproved ||
		snapshot.Instance == nil || snapshot.Instance.CurrentStateKey != "review" {
		t.Fatalf("load historical approval: %#v, %v", snapshot, err)
	}
	return reviewAtomicityFixture{fixture.db, fixture.repository, fixture.service, snapshot, fixture.principal.UserID}
}

func reviewRecoveryKey(fixture reviewAtomicityFixture) string {
	return "workflow-review-satisfied:" + fixture.snapshot.CurrentReview.ID.String()
}

func assertRecoveredReviewEffects(t *testing.T, fixture reviewAtomicityFixture, before reviewAtomicityState) {
	t.Helper()
	versionID, instance := fixture.snapshot.RequestVersionID, fixture.snapshot.Instance
	key := reviewRecoveryKey(fixture)
	assertRecoverySingleIntent(t, fixture)
	assertReviewAtomicityCount(t, fixture.db, "deployment_workflow_transitions", 1,
		`workflow_instance_id = ? AND request_version_id = ? AND from_state_key = 'review'
		AND to_state_key = 'deploying' AND transition_key = 'deploy' AND actor_id IS NULL
		AND idempotency_key = ? AND occurred_at = ?`, instance.ID, versionID, key, runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "deployment_jobs", 1,
		`aggregate_id = ? AND idempotency_key = ? AND available_at = ?`,
		versionID, "workflow-deployment:"+key, runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 1,
		`action = 'deployment_workflow.transition' AND resource_type = 'deployment_request_version'
		AND resource_id = ? AND actor_id IS NULL AND request_id = 'worker-workflow-review-recovery'
		AND metadata = '{"stateKey":"deploying"}'::jsonb AND occurred_at = ?`, versionID.String(), runtimeIntegrationTime())
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 1,
		`event_type = 'deployment.workflow.advanced' AND aggregate_type = 'deployment_request_version'
		AND aggregate_id = ? AND payload = jsonb_build_object('requestVersionId', ?::text, 'stateKey', 'deploying')
		AND occurred_at = ?`, versionID.String(), versionID.String(), runtimeIntegrationTime())
	assertRecoveryReviewUnchanged(t, fixture, before)
}

func assertRecoverySingleIntent(t *testing.T, fixture reviewAtomicityFixture) {
	t.Helper()
	versionID, instance := fixture.snapshot.RequestVersionID, fixture.snapshot.Instance
	assertCountWhere(t, fixture.db, "deployment_workflow_transitions", "workflow_instance_id = ?", instance.ID, 1)
	assertCountWhere(t, fixture.db, "deployment_jobs", "aggregate_id = ?", versionID, 1)
	assertReviewAtomicityCount(t, fixture.db, "deployment_jobs", 1,
		`aggregate_id = ? AND job_type = 'ExecuteDeployment' AND aggregate_type = 'deployment_request_version'
		AND status = 'Pending' AND attempts = 0 AND fencing_token = 0 AND lease_owner IS NULL
		AND payload = jsonb_build_object('requestVersionId', ?::text, 'workflowInstanceId', ?::text, 'workflowStateKey', 'deploying')
		AND idempotency_key = (SELECT 'workflow-deployment:' || idempotency_key FROM deployment_workflow_transitions WHERE workflow_instance_id = ?)`,
		versionID, versionID.String(), instance.ID.String(), instance.ID)
	snapshot, err := fixture.repository.Load(t.Context(), versionID)
	if err != nil || snapshot.Instance == nil || snapshot.Instance.ID != instance.ID ||
		snapshot.Instance.WorkflowVersionID != fixture.snapshot.Version.ID || snapshot.Instance.CurrentStateKey != "deploying" ||
		snapshot.Instance.Status != deploydomain.WorkflowInstanceRunning || snapshot.Instance.LockVersion != instance.LockVersion+1 {
		t.Fatalf("recovered instance = %#v, %v", snapshot.Instance, err)
	}
	assertReviewAtomicityCount(t, fixture.db, "deployment_request_versions", 1,
		"id = ? AND lock_version = ? AND workflow_version_id = ?", versionID, snapshot.Instance.LockVersion, fixture.snapshot.Version.ID)
}

func assertRecoveryReviewUnchanged(t *testing.T, fixture reviewAtomicityFixture, before reviewAtomicityState) {
	t.Helper()
	after := loadReviewAtomicityState(t, fixture)
	if after.Task != before.Task || after.Decisions != before.Decisions {
		t.Fatalf("recovery changed historical review evidence: %#v", after)
	}
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 1,
		"action = 'deployment_review.decide' AND resource_id = ?", fixture.snapshot.CurrentReview.ID.String())
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 1,
		"event_type = 'deployment.review.decided' AND aggregate_id = ?", fixture.snapshot.RequestVersionID.String())
}

type countingRecoveryRepository struct {
	*deployinfra.WorkflowRuntimeRepository
	successes atomic.Int64
	conflicts atomic.Int64
}

func (repository *countingRecoveryRepository) ApplyTransition(ctx context.Context, change deployapp.WorkflowTransitionChange) error {
	err := repository.WorkflowRuntimeRepository.ApplyTransition(ctx, change)
	if err == nil {
		repository.successes.Add(1)
	} else if errors.Is(err, deployapp.ErrWorkflowRuntimeConflict) {
		repository.conflicts.Add(1)
	}
	return err
}

type recoveryWorkerObserver struct{ errors []error }

func (observer *recoveryWorkerObserver) ObserveCandidateReconciliation(_ string, _ time.Duration, _ deployapp.CandidateResult, err error) {
	observer.errors = append(observer.errors, err)
}

type recoveryExternalStub struct{ calls int }

func (stub *recoveryExternalStub) GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	stub.calls++
	return argodomain.Application{}, errors.New("unexpected recovery external read")
}

func (stub *recoveryExternalStub) GetTargetManifestsAtRevision(context.Context, argodomain.ApplicationIdentity, string, string) ([]string, string, error) {
	stub.calls++
	return nil, "", errors.New("unexpected recovery manifest read")
}

func (stub *recoveryExternalStub) GetManagedResourceDiffs(context.Context, argodomain.ApplicationIdentity, string) ([]argodomain.ResourceDiff, error) {
	stub.calls++
	return nil, errors.New("unexpected recovery diff read")
}

func (stub *recoveryExternalStub) Lock(context.Context, uuid.UUID, string, []string) ([]ecrdomain.DigestSnapshot, error) {
	stub.calls++
	return nil, errors.New("unexpected recovery image read")
}

func newRecoveryWorker(t *testing.T, fixture reviewAtomicityFixture) (*deployapp.CandidateReconciler, *recoveryWorkerObserver, *recoveryExternalStub) {
	t.Helper()
	repository, err := deployinfra.NewCandidateRepository(fixture.db)
	if err != nil {
		t.Fatalf("create recovery candidate repository: %v", err)
	}
	observer, external := &recoveryWorkerObserver{}, &recoveryExternalStub{}
	worker, err := deployapp.NewCandidateReconciler(deployapp.CandidateReconcilerOptions{
		Repository: repository, Argo: external, Images: external, Observer: observer,
		Workflows: fixture.service, Interval: time.Minute,
	})
	if err != nil {
		t.Fatalf("create recovery worker: %v", err)
	}
	return worker, observer, external
}
