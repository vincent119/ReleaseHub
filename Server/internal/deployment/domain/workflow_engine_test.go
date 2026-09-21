package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWorkflowWithoutManualReview(t *testing.T) {
	engine := mustWorkflowEngine(t, workflowWithoutReview())
	started, err := engine.Start(uuid.New(), uuid.New(), workflowTestTime())
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	result, err := engine.Transition(started.Instance, WorkflowTransitionCommand{
		TransitionKey: "deploy", Trigger: WorkflowTriggerAutomatic,
		PermissionGranted: true, OccurredAt: workflowTestTime(),
	})
	if err != nil {
		t.Fatalf("transition workflow: %v", err)
	}
	if !result.DeploymentIntent || result.Instance.CurrentStateKey != "deploying" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDeploymentBlockedUntilWorkflowAllows(t *testing.T) {
	engine := mustWorkflowEngine(t, twoStageReviewWorkflow())
	started, err := engine.Start(uuid.New(), uuid.New(), workflowTestTime())
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	first := transitionCommand("submit", WorkflowTriggerManual, nil)
	firstResult, err := engine.Transition(started.Instance, first)
	if err != nil || firstResult.ReviewPolicy == nil {
		t.Fatalf("enter first review: %#v %v", firstResult, err)
	}
	pending := transitionCommand("first-approved", WorkflowTriggerReviewSatisfied, map[WorkflowFact]string{WorkflowFactReviewStatus: "Pending"})
	if _, err := engine.Transition(firstResult.Instance, pending); err == nil {
		t.Fatal("expected pending review to block transition")
	}
	approved := transitionCommand("first-approved", WorkflowTriggerReviewSatisfied, map[WorkflowFact]string{WorkflowFactReviewStatus: "Approved"})
	secondResult, err := engine.Transition(firstResult.Instance, approved)
	if err != nil || secondResult.ReviewPolicy == nil {
		t.Fatalf("enter second review: %#v %v", secondResult, err)
	}
	if secondResult.DeploymentIntent {
		t.Fatal("first review stage allowed deployment")
	}
}

func TestWorkflowEngineAppliesApprovedReviewResult(t *testing.T) {
	engine := mustWorkflowEngine(t, twoStageReviewWorkflow())
	instance := WorkflowInstance{ID: uuid.New(), RequestVersionID: uuid.New(), WorkflowVersionID: uuid.New(),
		CurrentStateKey: "review-one", Status: WorkflowInstanceRunning, LockVersion: 1, StartedAt: workflowTestTime()}
	result, found, err := engine.ApplyReviewResult(instance, ReviewTaskApproved, workflowTestTime())
	if err != nil || !found || result.Instance.CurrentStateKey != "review-two" || result.ReviewPolicy == nil {
		t.Fatalf("apply approved review result = %#v, %t, %v", result, found, err)
	}
	if _, found, err := engine.ApplyReviewResult(instance, ReviewTaskRejected, workflowTestTime()); err != nil || found {
		t.Fatalf("rejected review result = found %t, error %v", found, err)
	}
}

func TestWorkflowEngineLeavesApprovedReviewWithoutAutomaticEdge(t *testing.T) {
	engine := mustWorkflowEngine(t, workflowStartingReviewWithoutTransition())
	instance := WorkflowInstance{ID: uuid.New(), RequestVersionID: uuid.New(), WorkflowVersionID: uuid.New(),
		CurrentStateKey: "review", Status: WorkflowInstanceRunning, LockVersion: 1, StartedAt: workflowTestTime()}
	if _, found, err := engine.ApplyReviewResult(instance, ReviewTaskApproved, workflowTestTime()); err != nil || found {
		t.Fatalf("review without automatic edge = found %t, error %v", found, err)
	}
}

func TestWorkflowDocumentAllowsNonTerminatingGraph(t *testing.T) {
	_, err := NewWorkflowDocument(WorkflowDocument{
		InitialState: "start",
		States:       []WorkflowState{{Key: "start", Name: "Start", Type: WorkflowStateStart}},
		Transitions: []WorkflowTransition{{
			Key: "repeat", From: "start", To: "start", Trigger: WorkflowTriggerManual,
			Permission: "deployment_request.update",
		}},
	})
	if err != nil {
		t.Fatalf("non-terminating workflow was rejected: %v", err)
	}
}

func TestDeploymentResultSelectsExactlyOneConditionalTransition(t *testing.T) {
	engine := mustWorkflowEngine(t, deploymentResultWorkflow())
	instance := WorkflowInstance{ID: uuid.New(), RequestVersionID: uuid.New(), WorkflowVersionID: uuid.New(),
		CurrentStateKey: "deploying", Status: WorkflowInstanceRunning, LockVersion: 2, StartedAt: workflowTestTime()}
	result, err := engine.ApplyDeploymentResult(instance, ExecutionPartialFailed, workflowTestTime())
	if err != nil || result.Instance.CurrentStateKey != "failed" {
		t.Fatalf("apply deployment result = %#v, %v", result, err)
	}

	ambiguous := deploymentResultWorkflow()
	ambiguous.Transitions[0].Conditions = nil
	ambiguous.Transitions[1].Conditions = nil
	if _, err := mustWorkflowEngine(t, ambiguous).ApplyDeploymentResult(instance, ExecutionSucceeded, workflowTestTime()); err == nil {
		t.Fatal("ambiguous deployment result must be rejected")
	}
}

func TestPermissionActionSelectsExactlyOneConditionalTransition(t *testing.T) {
	document := deploymentResultWorkflow()
	document.Transitions = append(document.Transitions, WorkflowTransition{
		Key: "retry", From: "failed", To: "deploying", Trigger: WorkflowTriggerManual,
		Permission: "deployment_request.retry", Conditions: []WorkflowCondition{{
			Fact: WorkflowFactDeploymentStatus, Operator: WorkflowOperatorIn,
			Values: []string{"Failed", "PartialFailed"},
		}},
	})
	engine := mustWorkflowEngine(t, document)
	instance := WorkflowInstance{ID: uuid.New(), RequestVersionID: uuid.New(), WorkflowVersionID: uuid.New(),
		CurrentStateKey: "failed", Status: WorkflowInstanceRunning, LockVersion: 3, StartedAt: workflowTestTime()}
	command := WorkflowTransitionCommand{Permission: "deployment_request.retry", ActorID: uuid.New(),
		PermissionGranted: true, Facts: map[WorkflowFact]string{WorkflowFactDeploymentStatus: "PartialFailed"}, OccurredAt: workflowTestTime()}
	result, err := engine.ApplyPermissionAction(instance, command)
	if err != nil || result.Instance.CurrentStateKey != "deploying" || !result.DeploymentIntent {
		t.Fatalf("apply permission action = %#v, %v", result, err)
	}
	ambiguous := document.Transitions[len(document.Transitions)-1]
	ambiguous.Key = "retry-again"
	document.Transitions = append(document.Transitions, ambiguous)
	if _, err := mustWorkflowEngine(t, document).ApplyPermissionAction(instance, command); err == nil {
		t.Fatal("ambiguous permission action must be rejected")
	}
}

func deploymentResultWorkflow() WorkflowDocument {
	succeeded := WorkflowCondition{Fact: WorkflowFactDeploymentStatus, Operator: WorkflowOperatorEquals, Values: []string{"Succeeded"}}
	failed := WorkflowCondition{Fact: WorkflowFactDeploymentStatus, Operator: WorkflowOperatorIn, Values: []string{"Failed", "PartialFailed"}}
	return WorkflowDocument{InitialState: "deploying", States: []WorkflowState{
		{Key: "deploying", Name: "Deploying", Type: WorkflowStateDeployment},
		{Key: "succeeded", Name: "Succeeded", Type: WorkflowStateTerminal},
		{Key: "failed", Name: "Failed", Type: WorkflowStateManualAction},
	}, Transitions: []WorkflowTransition{
		{Key: "succeed", From: "deploying", To: "succeeded", Trigger: WorkflowTriggerDeploymentResult, Permission: "deployment_request.update", Conditions: []WorkflowCondition{succeeded}},
		{Key: "fail", From: "deploying", To: "failed", Trigger: WorkflowTriggerDeploymentResult, Permission: "deployment_request.update", Conditions: []WorkflowCondition{failed}},
	}}
}

func mustWorkflowEngine(t *testing.T, document WorkflowDocument) *WorkflowEngine {
	t.Helper()
	engine, err := NewWorkflowEngine(document)
	if err != nil {
		t.Fatalf("create workflow engine: %v", err)
	}
	return engine
}

func workflowWithoutReview() WorkflowDocument {
	return WorkflowDocument{
		InitialState: "start",
		States: []WorkflowState{
			{Key: "start", Name: "Start", Type: WorkflowStateStart},
			{Key: "deploying", Name: "Deploying", Type: WorkflowStateDeployment},
		},
		Transitions: []WorkflowTransition{{
			Key: "deploy", From: "start", To: "deploying", Trigger: WorkflowTriggerAutomatic,
			Permission: "deployment_request.deploy",
		}},
	}
}

func workflowStartingReviewWithoutTransition() WorkflowDocument {
	policy := ReviewPolicy{Type: ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()}}
	return WorkflowDocument{InitialState: "review", States: []WorkflowState{{
		Key: "review", Name: "Review", Type: WorkflowStateReview, ReviewPolicy: &policy,
	}}}
}

func twoStageReviewWorkflow() WorkflowDocument {
	reviewerID := uuid.New()
	policy := ReviewPolicy{Type: ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{reviewerID}}
	approved := WorkflowCondition{Fact: WorkflowFactReviewStatus, Operator: WorkflowOperatorEquals, Values: []string{"Approved"}}
	return WorkflowDocument{
		InitialState: "start",
		States: []WorkflowState{
			{Key: "start", Name: "Start", Type: WorkflowStateStart},
			{Key: "review-one", Name: "First review", Type: WorkflowStateReview, ReviewPolicy: &policy},
			{Key: "review-two", Name: "Second review", Type: WorkflowStateReview, ReviewPolicy: &policy},
			{Key: "deploying", Name: "Deploying", Type: WorkflowStateDeployment},
		},
		Transitions: []WorkflowTransition{
			{Key: "submit", From: "start", To: "review-one", Trigger: WorkflowTriggerManual, Permission: "deployment_request.update"},
			{Key: "first-approved", From: "review-one", To: "review-two", Trigger: WorkflowTriggerReviewSatisfied, Permission: "deployment_request.review", Conditions: []WorkflowCondition{approved}},
			{Key: "deploy", From: "review-two", To: "deploying", Trigger: WorkflowTriggerReviewSatisfied, Permission: "deployment_request.deploy", Conditions: []WorkflowCondition{approved}},
		},
	}
}

func transitionCommand(key string, trigger WorkflowTrigger, facts map[WorkflowFact]string) WorkflowTransitionCommand {
	return WorkflowTransitionCommand{TransitionKey: key, Trigger: trigger, PermissionGranted: true, Facts: facts, OccurredAt: workflowTestTime()}
}

func workflowTestTime() time.Time {
	return time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)
}
