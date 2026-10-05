//go:build integration

package database_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestWorkflowReviewRecoverySelectsOnlyCurrentApprovedRunningHistory(t *testing.T) {
	tests := []struct {
		name   string
		change string
		want   int
	}{
		{name: "approved_current_running", want: 1},
		{name: "pending", change: "UPDATE deployment_review_tasks SET status = 'Pending', closed_at = NULL"},
		{name: "reassignment_required", change: "UPDATE deployment_review_tasks SET status = 'ReassignmentRequired', closed_at = NULL"},
		{name: "rejected", change: "UPDATE deployment_review_tasks SET status = 'Rejected'"},
		{name: "closed", change: "UPDATE deployment_review_tasks SET status = 'Closed'"},
		{name: "completed", change: "UPDATE deployment_workflow_instances SET status = 'Completed', completed_at = now()"},
		{name: "blocked", change: "UPDATE deployment_workflow_instances SET status = 'Blocked', completed_at = now()"},
		{name: "superseded", change: "UPDATE deployment_workflow_instances SET status = 'Superseded', completed_at = now()"},
		{name: "rejected_instance", change: "UPDATE deployment_workflow_instances SET status = 'Rejected', completed_at = now()"},
		{name: "different_state", change: "UPDATE deployment_workflow_instances SET current_state_key = 'deploying'"},
		{name: "newer_review", change: `INSERT INTO deployment_review_tasks
			(workflow_instance_id, request_version_id, state_key, stage_number, policy_type, required_approvals, assignee_snapshot, status)
			SELECT workflow_instance_id, request_version_id, 'review-next', stage_number + 1, policy_type, required_approvals, assignee_snapshot, 'Pending'
			FROM deployment_review_tasks`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReviewRecoveryFixture(t, runtimeIntegrationDocument)
			if test.change != "" {
				execDeploymentSQL(t, fixture.db, test.change)
			}
			values, err := fixture.repository.ListApprovedReviewRecoveries(t.Context())
			if err != nil || len(values) != test.want || (test.want == 1 && values[0].RequestVersionID != fixture.snapshot.RequestVersionID) {
				t.Fatalf("recovery selection = %#v, %v", values, err)
			}
			if test.want == 0 {
				before := loadReviewAtomicityState(t, fixture)
				if err := fixture.service.RecoverApprovedReviews(t.Context()); err != nil {
					t.Fatalf("ineligible history error = %v", err)
				}
				if after := loadReviewAtomicityState(t, fixture); after != before {
					t.Fatal("ineligible history was changed")
				}
			}
		})
	}
}

func TestWorkflowReviewRecoveryWorkerUsesPinnedVersionAndPersistsOneIntent(t *testing.T) {
	fixture := newReviewRecoveryFixture(t, runtimeIntegrationDocument)
	before := loadReviewAtomicityState(t, fixture)
	publishNewRecoveryVersion(t, fixture)
	worker, observer, external := newRecoveryWorker(t, fixture)
	for range 2 {
		result, err := worker.ReconcileOnce(t.Context())
		if err != nil || result.CreatedRequests != 0 || result.EligibleApplications != 0 {
			t.Fatalf("recovery worker = %#v, %v", result, err)
		}
	}
	if len(observer.errors) != 2 || observer.errors[0] != nil || observer.errors[1] != nil || external.calls != 0 {
		t.Fatalf("recovery worker observer = %#v, external calls = %d", observer.errors, external.calls)
	}
	assertRecoveredReviewEffects(t, fixture, before)
	values, err := fixture.repository.ListApprovedReviewRecoveries(t.Context())
	if err != nil || len(values) != 0 {
		t.Fatalf("completed recovery still eligible = %#v, %v", values, err)
	}
}

func publishNewRecoveryVersion(t *testing.T, fixture reviewAtomicityFixture) {
	t.Helper()
	document := runtimeIntegrationDocument(fixture.reviewerID)
	document.Transitions = nil
	version, err := deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: fixture.snapshot.Version.WorkflowID, VersionNumber: 2, ActorID: fixture.reviewerID,
		Document: document, CreatedAt: runtimeIntegrationTime(),
	})
	if err != nil {
		t.Fatalf("create newer recovery graph: %v", err)
	}
	repository, err := deployinfra.NewWorkflowDefinitionRepository(fixture.db)
	if err != nil {
		t.Fatalf("create recovery definition repository: %v", err)
	}
	mutation := deployapp.WorkflowMutation{ActorID: fixture.reviewerID, RequestID: "newer-recovery-graph", OccurredAt: runtimeIntegrationTime()}
	if err := repository.AppendVersion(t.Context(), mutation, 1, version); err != nil {
		t.Fatalf("append newer recovery graph: %v", err)
	}
	published := publishWorkflowVersion(t, version, runtimeIntegrationTime())
	if err := repository.UpdateLifecycle(t.Context(), mutation, 1, published); err != nil {
		t.Fatalf("publish newer recovery graph: %v", err)
	}
}

func TestWorkflowReviewRecoveryRejectsUnusableEdgesWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*deploydomain.WorkflowDocument)
	}{
		{"missing", func(document *deploydomain.WorkflowDocument) { document.Transitions = nil }},
		{"unmatched", func(document *deploydomain.WorkflowDocument) {
			document.Transitions[0].Conditions[0].Values = []string{"Rejected"}
		}},
		{"ambiguous", func(document *deploydomain.WorkflowDocument) {
			edge := document.Transitions[0]
			edge.Key = "another-deploy"
			document.Transitions = append(document.Transitions, edge)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReviewRecoveryFixture(t, func(reviewer uuid.UUID) deploydomain.WorkflowDocument {
				document := runtimeIntegrationDocument(reviewer)
				test.change(&document)
				return document
			})
			before := loadReviewAtomicityState(t, fixture)
			worker, observer, external := newRecoveryWorker(t, fixture)
			_, err := worker.ReconcileOnce(t.Context())
			if !errors.Is(err, deployapp.ErrWorkflowRuntimeInvalid) || !strings.Contains(err.Error(), fixture.snapshot.RequestVersionID.String()) {
				t.Fatalf("unusable recovery error = %v", err)
			}
			if len(observer.errors) != 1 || !errors.Is(observer.errors[0], deployapp.ErrWorkflowRuntimeInvalid) || external.calls != 0 {
				t.Fatalf("unusable recovery observer = %#v, external calls = %d", observer.errors, external.calls)
			}
			if after := loadReviewAtomicityState(t, fixture); after != before {
				t.Fatal("unusable edge changed historical rows")
			}
		})
	}
}

func TestWorkflowReviewRecoveryConcurrentWorkersPersistOneIntent(t *testing.T) {
	fixture := newReviewRecoveryFixture(t, runtimeIntegrationDocument)
	before := loadReviewAtomicityState(t, fixture)
	repository := &countingRecoveryRepository{WorkflowRuntimeRepository: fixture.repository}
	service := runtimeService(t, repository)
	blocker := holdRowLock(t, fixture.db, "deployment_request_versions", fixture.snapshot.RequestVersionID)
	results := runConcurrentErrors(t, 2, func(int) error { return service.RecoverApprovedReviews(t.Context()) })
	waitForBlockedQueries(t, fixture.db, "deployment_request_versions", 2)
	commitBlocker(t, blocker)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent recovery = %v", err)
		}
	}
	if repository.successes.Load() != 1 || repository.conflicts.Load() != 1 {
		t.Fatalf("recovery writes = %d, conflicts = %d", repository.successes.Load(), repository.conflicts.Load())
	}
	assertRecoveredReviewEffects(t, fixture, before)
}

func TestWorkflowReviewRecoveryConcurrentManualTransitionPersistsOneIntent(t *testing.T) {
	fixture := newReviewRecoveryFixture(t, func(reviewer uuid.UUID) deploydomain.WorkflowDocument {
		document := runtimeIntegrationDocument(reviewer)
		document.Transitions = append(document.Transitions, deploydomain.WorkflowTransition{
			Key: "manual-deploy", From: "review", To: "deploying", Trigger: deploydomain.WorkflowTriggerManual,
			Permission: "deployment_request.deploy",
		})
		return document
	})
	before := loadReviewAtomicityState(t, fixture)
	repository := &countingRecoveryRepository{WorkflowRuntimeRepository: fixture.repository}
	service := runtimeService(t, repository)
	blocker := holdRowLock(t, fixture.db, "deployment_request_versions", fixture.snapshot.RequestVersionID)
	results := runConcurrentErrors(t, 2, func(index int) error {
		if index == 0 {
			return service.RecoverApprovedReviews(t.Context())
		}
		_, err := service.Transition(t.Context(), fixture.principal(), deployapp.WorkflowTransitionInput{
			RequestVersionID: fixture.snapshot.RequestVersionID, ExpectedLock: fixture.snapshot.Instance.LockVersion,
			TransitionKey: "manual-deploy", Trigger: deploydomain.WorkflowTriggerManual,
			IdempotencyKey: "manual-recovery-race", RequestID: "manual-recovery-race",
		})
		return err
	})
	waitForBlockedQueries(t, fixture.db, "deployment_request_versions", 2)
	commitBlocker(t, blocker)
	for range 2 {
		if err := <-results; err != nil && !errors.Is(err, deployapp.ErrWorkflowRuntimeConflict) {
			t.Fatalf("recovery/manual race = %v", err)
		}
	}
	if repository.successes.Load() != 1 || repository.conflicts.Load() != 1 {
		t.Fatalf("recovery/manual writes = %d, conflicts = %d", repository.successes.Load(), repository.conflicts.Load())
	}
	assertRecoverySingleIntent(t, fixture)
	assertRecoveryReviewUnchanged(t, fixture, before)
	assertReviewAtomicityCount(t, fixture.db, "audit_logs", 1,
		`action = 'deployment_workflow.transition' AND resource_type = 'deployment_request_version' AND resource_id = ?
		AND metadata = '{"stateKey":"deploying"}'::jsonb AND occurred_at = ?
		AND ((request_id = 'worker-workflow-review-recovery' AND actor_id IS NULL)
		OR (request_id = 'manual-recovery-race' AND actor_id = ?))`,
		fixture.snapshot.RequestVersionID.String(), runtimeIntegrationTime(), fixture.reviewerID)
	assertCountWhere(t, fixture.db, "audit_logs", "resource_id = ?", fixture.snapshot.RequestVersionID.String(), 2)
	assertReviewAtomicityCount(t, fixture.db, "outbox_events", 1,
		"event_type = 'deployment.workflow.advanced' AND aggregate_id = ? AND payload->>'stateKey' = 'deploying'", fixture.snapshot.RequestVersionID.String())
}

func TestWorkflowReviewRecoveryDoesNotAdvanceSupersededRequestVersion(t *testing.T) {
	fixture := newReviewRecoveryFixture(t, runtimeIntegrationDocument)
	execDeploymentSQL(t, fixture.db, "UPDATE deployment_request_versions SET status = 'Superseded' WHERE id = ?", fixture.snapshot.RequestVersionID)
	before := loadReviewAtomicityState(t, fixture)
	repository := &countingRecoveryRepository{WorkflowRuntimeRepository: fixture.repository}
	if err := runtimeService(t, repository).RecoverApprovedReviews(t.Context()); err != nil {
		t.Fatalf("superseded version recovery = %v", err)
	}
	if repository.successes.Load() != 0 || repository.conflicts.Load() != 1 {
		t.Fatalf("superseded version writes = %d, conflicts = %d", repository.successes.Load(), repository.conflicts.Load())
	}
	if after := loadReviewAtomicityState(t, fixture); after != before {
		t.Fatal("superseded version changed historical rows")
	}
}

func TestWorkflowReviewRecoveryRollsBackPersistenceAndRetriesSafely(t *testing.T) {
	tests := []struct{ table, condition string }{
		{"deployment_workflow_transitions", "TRUE"},
		{"deployment_jobs", "NEW.job_type = 'ExecuteDeployment'"},
		{"audit_logs", "NEW.action = 'deployment_workflow.transition'"},
		{"outbox_events", "NEW.event_type = 'deployment.workflow.advanced'"},
	}
	for _, test := range tests {
		t.Run(test.table, func(t *testing.T) {
			fixture := newReviewRecoveryFixture(t, runtimeIntegrationDocument)
			before := loadReviewAtomicityState(t, fixture)
			installReviewAtomicityRejection(t, fixture.db, test.table, test.condition)
			err := fixture.service.RecoverApprovedReviews(t.Context())
			var failure *pgconn.PgError
			if !errors.As(err, &failure) || failure.Code != "P0001" || failure.Message != "review atomicity failure injected" {
				t.Fatalf("recovery persistence rejection = %v", err)
			}
			if after := loadReviewAtomicityState(t, fixture); after != before {
				t.Fatal("failed recovery changed historical rows")
			}
			execDeploymentSQL(t, fixture.db, fmt.Sprintf("DROP TRIGGER reject_review_atomicity ON %s", test.table))
			for range 2 {
				if err := fixture.service.RecoverApprovedReviews(t.Context()); err != nil {
					t.Fatalf("recovery retry = %v", err)
				}
			}
			assertRecoveredReviewEffects(t, fixture, before)
		})
	}
}

func TestWorkflowReviewRecoveryPreservesHistoryWhenEnteringNextReview(t *testing.T) {
	fixture := newReviewRecoveryFixture(t, func(reviewer uuid.UUID) deploydomain.WorkflowDocument {
		document := runtimeIntegrationDocument(reviewer)
		policy := *document.States[0].ReviewPolicy
		document.States = append(document.States, deploydomain.WorkflowState{
			Key: "review-next", Name: "Next review", Type: deploydomain.WorkflowStateReview, ReviewPolicy: &policy,
		})
		document.Transitions[0].To = "review-next"
		return document
	})
	before := loadReviewAtomicityState(t, fixture)
	for range 2 {
		if err := fixture.service.RecoverApprovedReviews(t.Context()); err != nil {
			t.Fatalf("recover into next review = %v", err)
		}
	}
	assertRecoveryReviewUnchanged(t, fixture, before)
	assertCountWhere(t, fixture.db, "deployment_review_tasks", "workflow_instance_id = ?", fixture.snapshot.Instance.ID, 2)
	assertReviewAtomicityCount(t, fixture.db, "deployment_review_tasks", 1,
		"workflow_instance_id = ? AND stage_number = 2 AND state_key = 'review-next' AND status = 'Pending' AND closed_at IS NULL", fixture.snapshot.Instance.ID)
	assertCountWhere(t, fixture.db, "deployment_workflow_transitions", "workflow_instance_id = ?", fixture.snapshot.Instance.ID, 1)
	assertCountWhere(t, fixture.db, "deployment_jobs", "aggregate_id = ?", fixture.snapshot.RequestVersionID, 0)
	snapshot, err := fixture.repository.Load(t.Context(), fixture.snapshot.RequestVersionID)
	if err != nil || snapshot.Instance.CurrentStateKey != "review-next" || snapshot.CurrentReview.StageNumber != 2 ||
		snapshot.CurrentReview.Status != deploydomain.ReviewTaskPending || len(snapshot.CurrentReview.Decisions) != 0 {
		t.Fatalf("next review snapshot = %#v, %v", snapshot, err)
	}
}
