//go:build integration

package database_test

import (
	"testing"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestWorkflowRuntimeConcurrentManualTransitionsCreateOneDeploymentIntent(t *testing.T) {
	fixture := workflowConcurrencyFixtureFor(t, manualConcurrencyDocument)
	if fixture.snapshot.CurrentReview != nil {
		t.Fatal("manual transition fixture unexpectedly has a review task")
	}
	auditsBefore, outboxBefore := tableCount(t, fixture.db, "audit_logs"), tableCount(t, fixture.db, "outbox_events")
	blocker := holdRowLock(t, fixture.db, "deployment_request_versions", fixture.snapshot.RequestVersionID)
	results := runConcurrentErrors(t, 2, func(_ int) error {
		_, err := fixture.service.Transition(t.Context(), fixture.principal, deployapp.WorkflowTransitionInput{
			RequestVersionID: fixture.snapshot.RequestVersionID, ExpectedLock: fixture.snapshot.Instance.LockVersion,
			TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerManual,
			IdempotencyKey: "concurrent-transition", RequestID: "concurrent-runtime",
		})
		return err
	})
	waitForBlockedQueries(t, fixture.db, "deployment_request_versions", 2)
	commitBlocker(t, blocker)
	assertOneSuccessOneConflict(t, results, deployapp.ErrWorkflowRuntimeConflict)
	assertConcurrentTransitionEffects(t, fixture, "concurrent-transition")
	assertCountWhere(t, fixture.db, "deployment_review_decisions", "idempotency_key = ?", "concurrent-transition", 0)
	assertTableCountDelta(t, fixture.db, "audit_logs", auditsBefore, 1)
	assertTableCountDelta(t, fixture.db, "outbox_events", outboxBefore, 1)
}

func manualConcurrencyDocument(uuid.UUID) deploydomain.WorkflowDocument {
	return deploydomain.WorkflowDocument{
		InitialState: "approved",
		States: []deploydomain.WorkflowState{
			{Key: "approved", Name: "Approved", Type: deploydomain.WorkflowStateManualAction},
			{Key: "deploying", Name: "Deploying", Type: deploydomain.WorkflowStateDeployment},
		},
		Transitions: []deploydomain.WorkflowTransition{{
			Key: "deploy", From: "approved", To: "deploying", Trigger: deploydomain.WorkflowTriggerManual,
			Permission: "deployment_request.deploy",
		}},
	}
}
