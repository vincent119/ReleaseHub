//go:build integration

package database_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestWorkflowReviewApprovalPersistsAutomaticDeployment(t *testing.T) {
	fixture := newReviewAtomicityFixture(t, reviewAtomicityOptions{automatic: true})
	input := fixture.reviewInput("review-atomic-approve")
	task, err := fixture.service.DecideReview(t.Context(), fixture.principal(), input)
	if err != nil || task.Status != deploydomain.ReviewTaskApproved {
		t.Fatalf("approve review = %#v, %v", task, err)
	}
	assertReviewAtomicityProjection(t, fixture, "deploying", deploydomain.ReviewTaskApproved)
	assertReviewAtomicityDecision(t, fixture, input, deploydomain.ReviewTaskApproved)
	assertReviewAtomicityDeployment(t, fixture, input)
}

func TestWorkflowReviewDecisionWithoutSatisfiedAutomaticEdgeDoesNotDeploy(t *testing.T) {
	tests := []struct {
		name    string
		options reviewAtomicityOptions
		status  deploydomain.ReviewTaskStatus
	}{
		{
			name:    "first_quorum_approval",
			options: reviewAtomicityOptions{automatic: true, quorum: true},
			status:  deploydomain.ReviewTaskPending,
		},
		{
			name:   "approved_without_automatic_edge",
			status: deploydomain.ReviewTaskApproved,
		},
		{
			name:    "approved_without_matching_automatic_edge",
			options: reviewAtomicityOptions{automatic: true, unmatched: true},
			status:  deploydomain.ReviewTaskApproved,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReviewAtomicityFixture(t, test.options)
			input := fixture.reviewInput("review-no-automatic-deployment")
			task, err := fixture.service.DecideReview(t.Context(), fixture.principal(), input)
			if err != nil || task.Status != test.status {
				t.Fatalf("review decision = %#v, %v", task, err)
			}
			assertReviewAtomicityProjection(t, fixture, "review", test.status)
			assertReviewAtomicityDecision(t, fixture, input, test.status)
			assertReviewAtomicityNoDeployment(t, fixture, input)
		})
	}
}

func TestWorkflowReviewApprovalRollsBackWhenAutomaticPersistenceFails(t *testing.T) {
	tests := []struct {
		name      string
		table     string
		condition string
	}{
		{name: "transition", table: "deployment_workflow_transitions", condition: "TRUE"},
		{
			name: "deployment_intent", table: "deployment_jobs",
			condition: "NEW.job_type = 'ExecuteDeployment'",
		},
		{
			name: "transition_audit", table: "audit_logs",
			condition: "NEW.action = 'deployment_workflow.transition'",
		},
		{
			name: "transition_outbox", table: "outbox_events",
			condition: "NEW.event_type = 'deployment.workflow.advanced' AND NEW.payload ->> 'stateKey' = 'deploying'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReviewAtomicityFixture(t, reviewAtomicityOptions{automatic: true})
			before := loadReviewAtomicityState(t, fixture)
			installReviewAtomicityRejection(t, fixture.db, test.table, test.condition)
			input := fixture.reviewInput("review-rejected-persistence")
			_, err := fixture.service.DecideReview(t.Context(), fixture.principal(), input)
			var postgresError *pgconn.PgError
			if !errors.As(err, &postgresError) || postgresError.Code != "P0001" || postgresError.Message != "review atomicity failure injected" {
				t.Fatalf("review persistence rejection = %v", err)
			}
			if after := loadReviewAtomicityState(t, fixture); after != before {
				t.Fatalf("failed review transaction changed persisted state:\nbefore %#v\nafter %#v", before, after)
			}
			assertReviewAtomicityNoDecision(t, fixture, input)
			assertReviewAtomicityNoDeployment(t, fixture, input)
		})
	}
}

func TestWorkflowReviewClientFactsCannotAdvancePendingReview(t *testing.T) {
	fixture := newReviewAtomicityFixture(t, reviewAtomicityOptions{automatic: true})
	before := loadReviewAtomicityState(t, fixture)
	_, err := fixture.service.Transition(t.Context(), fixture.principal(), deployapp.WorkflowTransitionInput{
		RequestVersionID: fixture.snapshot.RequestVersionID, ExpectedLock: fixture.snapshot.Instance.LockVersion,
		TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerReviewSatisfied,
		Facts:          map[deploydomain.WorkflowFact]string{deploydomain.WorkflowFactReviewStatus: "Approved"},
		IdempotencyKey: "forged-review-result", RequestID: "forged-review-result",
	})
	if !errors.Is(err, deployapp.ErrWorkflowRuntimeInvalid) {
		t.Fatalf("client review facts error = %v", err)
	}
	if after := loadReviewAtomicityState(t, fixture); after != before {
		t.Fatalf("client review facts changed persisted state: %#v", after)
	}
}
