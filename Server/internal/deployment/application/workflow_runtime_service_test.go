package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestWorkflowRuntimeCreatesReviewFromPinnedPolicy(t *testing.T) {
	policy := deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()},
	}
	snapshot := runtimeSnapshot(t, workflowStartingWithReview(policy))
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	service := mustWorkflowRuntimeService(t, repository, true)
	result, err := service.Start(context.Background(), WorkflowStartInput{
		RequestVersionID: snapshot.RequestVersionID, IdempotencyKey: "start-review",
	})
	if err != nil || result.DeploymentIntent || repository.started.Review == nil {
		t.Fatalf("start review workflow: %#v %v", repository.started, err)
	}
	policy.UserIDs[0] = uuid.New()
	if repository.started.Review.Policy.UserIDs[0] == policy.UserIDs[0] {
		t.Fatal("review task did not preserve the pinned policy snapshot")
	}
}

func TestWorkflowRuntimePersistsDeploymentIntentOnlyAfterAllowedTransition(t *testing.T) {
	snapshot := runtimeSnapshot(t, workflowWithoutRuntimeReview())
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	denied := mustWorkflowRuntimeService(t, repository, false)
	input := runtimeTransitionInput(snapshot, instance)
	if _, err := denied.Transition(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, input); !errors.Is(err, ErrWorkflowForbidden) {
		t.Fatalf("denied transition error = %v", err)
	}
	if repository.transitioned.Result.DeploymentIntent {
		t.Fatal("denied transition persisted a deployment intent")
	}
	allowed := mustWorkflowRuntimeService(t, repository, true)
	result, err := allowed.Transition(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, input)
	if err != nil || !result.DeploymentIntent || !repository.transitioned.Result.DeploymentIntent {
		t.Fatalf("allowed transition: %#v %v", result, err)
	}
}

func TestWorkflowRuntimeReviewRechecksCurrentPermission(t *testing.T) {
	policy := deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()},
	}
	snapshot := runtimeSnapshot(t, workflowStartingWithReview(policy))
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	task := runtimeReviewTask(t, snapshot, policy)
	snapshot.CurrentReview = &task
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	service := mustWorkflowRuntimeService(t, repository, false)
	_, err := service.DecideReview(context.Background(), WorkflowPrincipal{UserID: policy.UserIDs[0]}, WorkflowReviewInput{
		RequestVersionID: snapshot.RequestVersionID, ReviewTaskID: task.ID,
		ExpectedLock: instance.LockVersion, Decision: deploydomain.ReviewDecisionApprove,
		IdempotencyKey: "review-1",
	})
	if !errors.Is(err, ErrWorkflowForbidden) {
		t.Fatalf("review permission error = %v", err)
	}
}

func TestWorkflowRuntimeRejectsClosedReviewAsConflict(t *testing.T) {
	policy := deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()},
	}
	snapshot := runtimeSnapshot(t, workflowStartingWithReview(policy))
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	task := runtimeReviewTask(t, snapshot, policy)
	task.Status = deploydomain.ReviewTaskApproved
	snapshot.CurrentReview = &task
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	service := mustWorkflowRuntimeService(t, repository, true)
	_, err := service.DecideReview(context.Background(), WorkflowPrincipal{UserID: policy.UserIDs[0]}, WorkflowReviewInput{
		RequestVersionID: snapshot.RequestVersionID, ReviewTaskID: task.ID,
		ExpectedLock: instance.LockVersion, Decision: deploydomain.ReviewDecisionApprove,
		IdempotencyKey: "closed-review",
	})
	if !errors.Is(err, ErrWorkflowRuntimeConflict) {
		t.Fatalf("closed review error = %v", err)
	}
}

func TestWorkflowRuntimeRejectsCallerProvidedReviewStatus(t *testing.T) {
	policy := deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()},
	}
	snapshot := runtimeSnapshot(t, workflowReviewThenDeploy(policy))
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	task := runtimeReviewTask(t, snapshot, policy)
	snapshot.CurrentReview = &task
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	service := mustWorkflowRuntimeService(t, repository, true)
	_, err := service.Transition(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, WorkflowTransitionInput{
		RequestVersionID: snapshot.RequestVersionID, ExpectedLock: instance.LockVersion,
		TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerReviewSatisfied,
		Facts:          map[deploydomain.WorkflowFact]string{deploydomain.WorkflowFactReviewStatus: "Approved"},
		IdempotencyKey: "forged-review-status",
	})
	if !errors.Is(err, ErrWorkflowRuntimeInvalid) || repository.transitioned.Result.DeploymentIntent {
		t.Fatalf("forged review status error = %v", err)
	}
}

func TestWorkflowRuntimeReassignmentRechecksManagementPermission(t *testing.T) {
	original, replacement := uuid.New(), uuid.New()
	policy := deploydomain.ReviewPolicy{Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{original}}
	snapshot := runtimeSnapshot(t, workflowStartingWithReview(policy))
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	task := runtimeReviewTask(t, snapshot, policy)
	snapshot.CurrentReview = &task
	repository := &workflowRuntimeRepositoryStub{snapshot: snapshot}
	input := WorkflowReviewReassignmentInput{
		RequestVersionID: snapshot.RequestVersionID, ReviewTaskID: task.ID,
		ExpectedLock: instance.LockVersion, UserIDs: []uuid.UUID{replacement},
		Reason: "reviewer unavailable", IdempotencyKey: "reassign-1",
	}
	if _, err := mustWorkflowRuntimeService(t, repository, false).ReassignReview(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, input); !errors.Is(err, ErrWorkflowForbidden) {
		t.Fatalf("reassignment permission error = %v", err)
	}
	updated, err := mustWorkflowRuntimeService(t, repository, true).ReassignReview(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, input)
	if err != nil || updated.Policy.UserIDs[0] != replacement || repository.reassigned.Reassignment.Reason != "reviewer unavailable" {
		t.Fatalf("reassign review: %#v %v", repository.reassigned, err)
	}
}

func mustWorkflowRuntimeService(t *testing.T, repository WorkflowRuntimeRepository, allowed bool) *WorkflowRuntimeService {
	t.Helper()
	service, err := NewWorkflowRuntimeService(WorkflowRuntimeServiceOptions{
		Repository: repository, Authorizer: runtimeAuthorizerStub{allowed: allowed},
		Assignments: reviewAssignmentResolverStub{}, Clock: workflowClockStub{},
	})
	if err != nil {
		t.Fatalf("create workflow runtime service: %v", err)
	}
	return service
}

func runtimeSnapshot(t *testing.T, document deploydomain.WorkflowDocument) WorkflowRuntimeSnapshot {
	t.Helper()
	version := workflowVersionFixture(t, document)
	scope, err := authz.NewEnvironmentScope(uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("create runtime scope: %v", err)
	}
	return WorkflowRuntimeSnapshot{
		RequestID: uuid.New(), RequestVersionID: uuid.New(), RequestCreatorID: uuid.New(),
		Scope: scope, Version: version, NextReviewStage: 1,
	}
}

func workflowVersionFixture(t *testing.T, document deploydomain.WorkflowDocument) deploydomain.ReleaseWorkflowVersion {
	t.Helper()
	version, err := deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: uuid.New(), VersionNumber: 1, ActorID: uuid.New(),
		Document: document, CreatedAt: workflowClockStub{}.Now(),
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}
	published, err := version.ChangeLifecycle(deploydomain.DefinitionPublished, 1, workflowClockStub{}.Now())
	if err != nil {
		t.Fatalf("publish workflow version: %v", err)
	}
	return published
}

func startRuntimeInstance(t *testing.T, snapshot WorkflowRuntimeSnapshot) deploydomain.WorkflowInstance {
	t.Helper()
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Version.Document)
	if err != nil {
		t.Fatalf("create workflow engine: %v", err)
	}
	result, err := engine.Start(snapshot.RequestVersionID, snapshot.Version.ID, workflowClockStub{}.Now())
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	return result.Instance
}

func runtimeTransitionInput(snapshot WorkflowRuntimeSnapshot, instance deploydomain.WorkflowInstance) WorkflowTransitionInput {
	return WorkflowTransitionInput{
		RequestVersionID: snapshot.RequestVersionID, ExpectedLock: instance.LockVersion,
		TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerManual,
		IdempotencyKey: "deploy-1",
	}
}

func runtimeReviewTask(t *testing.T, snapshot WorkflowRuntimeSnapshot, policy deploydomain.ReviewPolicy) deploydomain.ReviewTask {
	t.Helper()
	task, err := deploydomain.NewReviewTask(deploydomain.ReviewTaskDraft{
		RequestVersionID: snapshot.RequestVersionID, StateKey: "review", StageNumber: 1,
		RequestCreatorID: snapshot.RequestCreatorID, Policy: policy,
		Assignment: deploydomain.ReviewAssignment{UserIDs: policy.UserIDs},
	})
	if err != nil {
		t.Fatalf("create review task: %v", err)
	}
	return task
}

func workflowWithoutRuntimeReview() deploydomain.WorkflowDocument {
	return deploydomain.WorkflowDocument{
		InitialState: "start",
		States: []deploydomain.WorkflowState{
			{Key: "start", Name: "Start", Type: deploydomain.WorkflowStateStart},
			{Key: "deploying", Name: "Deploying", Type: deploydomain.WorkflowStateDeployment},
		},
		Transitions: []deploydomain.WorkflowTransition{{
			Key: "deploy", From: "start", To: "deploying",
			Trigger: deploydomain.WorkflowTriggerManual, Permission: "deployment_request.deploy",
		}},
	}
}

func workflowStartingWithReview(policy deploydomain.ReviewPolicy) deploydomain.WorkflowDocument {
	return deploydomain.WorkflowDocument{
		InitialState: "review",
		States: []deploydomain.WorkflowState{{
			Key: "review", Name: "Review", Type: deploydomain.WorkflowStateReview, ReviewPolicy: &policy,
		}},
		Transitions: []deploydomain.WorkflowTransition{},
	}
}

func workflowReviewThenDeploy(policy deploydomain.ReviewPolicy) deploydomain.WorkflowDocument {
	document := workflowStartingWithReview(policy)
	document.States = append(document.States, deploydomain.WorkflowState{
		Key: "deploying", Name: "Deploying", Type: deploydomain.WorkflowStateDeployment,
	})
	document.Transitions = append(document.Transitions, deploydomain.WorkflowTransition{
		Key: "deploy", From: "review", To: "deploying",
		Trigger:    deploydomain.WorkflowTriggerReviewSatisfied,
		Permission: "deployment_request.deploy",
		Conditions: []deploydomain.WorkflowCondition{{
			Fact:     deploydomain.WorkflowFactReviewStatus,
			Operator: deploydomain.WorkflowOperatorEquals, Values: []string{"Approved"},
		}},
	})
	return document
}

type workflowRuntimeRepositoryStub struct {
	snapshot     WorkflowRuntimeSnapshot
	started      WorkflowStartChange
	transitioned WorkflowTransitionChange
	reviewed     WorkflowReviewChange
	reassigned   WorkflowReviewReassignmentChange
}

func (s *workflowRuntimeRepositoryStub) Load(context.Context, uuid.UUID) (WorkflowRuntimeSnapshot, error) {
	return s.snapshot, nil
}

func (s *workflowRuntimeRepositoryStub) Start(_ context.Context, change WorkflowStartChange) error {
	s.started = change
	return nil
}

func (s *workflowRuntimeRepositoryStub) ApplyTransition(_ context.Context, change WorkflowTransitionChange) error {
	s.transitioned = change
	return nil
}

func (s *workflowRuntimeRepositoryStub) ApplyReview(_ context.Context, change WorkflowReviewChange) error {
	s.reviewed = change
	return nil
}

func (s *workflowRuntimeRepositoryStub) ApplyReviewReassignment(_ context.Context, change WorkflowReviewReassignmentChange) error {
	s.reassigned = change
	return nil
}

type runtimeAuthorizerStub struct{ allowed bool }

func (s runtimeAuthorizerStub) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return s.allowed, nil
}

type reviewAssignmentResolverStub struct{}

func (reviewAssignmentResolverStub) Resolve(_ context.Context, policy deploydomain.ReviewPolicy, _ authz.Scope) (deploydomain.ReviewAssignment, error) {
	return deploydomain.ReviewAssignment{UserIDs: policy.UserIDs, RoleMembers: map[uuid.UUID][]uuid.UUID{}}, nil
}
