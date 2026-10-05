package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestWorkflowReviewRecoveryReportsUnusablePinnedEdges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*deploydomain.WorkflowDocument)
	}{
		{"no_edge", func(document *deploydomain.WorkflowDocument) { document.Transitions = nil }},
		{"unmatched_edge", func(document *deploydomain.WorkflowDocument) {
			document.Transitions[0].Conditions[0].Values = []string{"Rejected"}
		}},
		{"ambiguous_edges", func(document *deploydomain.WorkflowDocument) {
			second := document.Transitions[0]
			second.Key = "another-deploy"
			document.Transitions = append(document.Transitions, second)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := recoverySnapshot(t, test.change)
			repository := recoveryRepository(snapshot)
			err := mustWorkflowRuntimeService(t, repository, false).RecoverApprovedReviews(t.Context())
			if !errors.Is(err, ErrWorkflowRuntimeInvalid) || !strings.Contains(err.Error(), snapshot.RequestVersionID.String()) {
				t.Fatalf("unusable recovery error = %v", err)
			}
			if len(repository.applied) != 0 {
				t.Fatalf("unusable recovery persisted %d transitions", len(repository.applied))
			}
		})
	}
}

func TestWorkflowReviewRecoverySkipsStaleCandidates(t *testing.T) {
	tests := []struct {
		name   string
		change func(*WorkflowRuntimeSnapshot)
	}{
		{"missing_instance", func(snapshot *WorkflowRuntimeSnapshot) { snapshot.Instance = nil }},
		{"missing_review", func(snapshot *WorkflowRuntimeSnapshot) { snapshot.CurrentReview = nil }},
		{"pending_review", func(snapshot *WorkflowRuntimeSnapshot) {
			snapshot.CurrentReview.Status = deploydomain.ReviewTaskPending
		}},
		{"different_state", func(snapshot *WorkflowRuntimeSnapshot) { snapshot.Instance.CurrentStateKey = "deploying" }},
		{"completed_instance", func(snapshot *WorkflowRuntimeSnapshot) {
			snapshot.Instance.Status = deploydomain.WorkflowInstanceCompleted
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := recoverySnapshot(t, nil)
			test.change(&snapshot)
			repository := recoveryRepository(snapshot)
			if err := mustWorkflowRuntimeService(t, repository, false).RecoverApprovedReviews(t.Context()); err != nil {
				t.Fatalf("stale recovery error = %v", err)
			}
			if len(repository.applied) != 0 {
				t.Fatalf("stale recovery persisted %d transitions", len(repository.applied))
			}
		})
	}
}

func TestWorkflowReviewRecoveryPreservesValidPeersAndSystemIdentity(t *testing.T) {
	invalid := recoverySnapshot(t, func(document *deploydomain.WorkflowDocument) { document.Transitions = nil })
	valid := recoverySnapshot(t, nil)
	repository := recoveryRepository(invalid, valid)
	err := mustWorkflowRuntimeService(t, repository, false).RecoverApprovedReviews(t.Context())
	if !errors.Is(err, ErrWorkflowRuntimeInvalid) || !strings.Contains(err.Error(), invalid.RequestVersionID.String()) {
		t.Fatalf("mixed recovery error = %v", err)
	}
	if len(repository.applied) != 1 {
		t.Fatalf("mixed recovery transitions = %d", len(repository.applied))
	}
	change := repository.applied[0]
	if change.ExpectedLock != valid.Instance.LockVersion || change.Result.Instance.WorkflowVersionID != valid.Version.ID ||
		change.Result.Instance.CurrentStateKey != "deploying" || !change.Result.DeploymentIntent || change.Review != nil {
		t.Fatalf("recovered peer = %#v", change)
	}
	if change.Mutation.ActorID != uuid.Nil || change.Mutation.RequestID != "worker-workflow-review-recovery" ||
		change.Mutation.IdempotencyKey != "workflow-review-satisfied:"+valid.CurrentReview.ID.String() {
		t.Fatalf("recovery mutation = %#v", change.Mutation)
	}
}

func TestWorkflowReviewRecoveryPreservesRepositoryErrors(t *testing.T) {
	failure := errors.New("recovery persistence unavailable")
	tests := []struct {
		name      string
		listErr   error
		loadErr   error
		applyErr  error
		wantError error
		writes    int
	}{
		{name: "list_failure", listErr: failure, wantError: failure},
		{name: "load_failure", loadErr: failure, wantError: failure},
		{name: "write_failure", applyErr: failure, wantError: failure, writes: 1},
		{name: "optimistic_conflict", applyErr: ErrWorkflowRuntimeConflict, writes: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := recoverySnapshot(t, nil)
			repository := recoveryRepository(snapshot)
			repository.listErr, repository.loadErr, repository.applyErr = test.listErr, test.loadErr, test.applyErr
			err := mustWorkflowRuntimeService(t, repository, false).RecoverApprovedReviews(t.Context())
			if !errors.Is(err, test.wantError) || len(repository.applied) != test.writes {
				t.Fatalf("recovery error = %v, writes = %d", err, len(repository.applied))
			}
		})
	}
}

func TestWorkflowReviewRecoveryRejectsInvalidPinnedDocument(t *testing.T) {
	snapshot := recoverySnapshot(t, nil)
	snapshot.Version.Document.States = nil
	repository := recoveryRepository(snapshot)
	err := mustWorkflowRuntimeService(t, repository, false).RecoverApprovedReviews(t.Context())
	if !errors.Is(err, ErrWorkflowRuntimeInvalid) || len(repository.applied) != 0 {
		t.Fatalf("invalid pinned recovery = %v, writes = %d", err, len(repository.applied))
	}
}

func TestWorkflowReviewRecoveryPreservesNextReviewAssignmentFailure(t *testing.T) {
	snapshot := recoverySnapshot(t, func(document *deploydomain.WorkflowDocument) {
		policy := *document.States[0].ReviewPolicy
		document.States = append(document.States, deploydomain.WorkflowState{
			Key: "review-next", Name: "Next review", Type: deploydomain.WorkflowStateReview, ReviewPolicy: &policy,
		})
		document.Transitions[0].To = "review-next"
	})
	failure := errors.New("review assignments unavailable")
	repository := recoveryRepository(snapshot)
	service, err := NewWorkflowRuntimeService(WorkflowRuntimeServiceOptions{
		Repository: repository, Authorizer: runtimeAuthorizerStub{},
		Assignments: recoveryAssignmentFailureStub{failure}, Clock: workflowClockStub{},
	})
	if err != nil {
		t.Fatalf("create recovery service: %v", err)
	}
	err = service.RecoverApprovedReviews(t.Context())
	if !errors.Is(err, failure) || len(repository.applied) != 0 {
		t.Fatalf("assignment recovery = %v, writes = %d", err, len(repository.applied))
	}
}

func TestCandidateWorkflowRecoveryReportsErrorsWithoutBlockingPendingStarts(t *testing.T) {
	failure := errors.New("approved review has no matching edge")
	workflows := &candidateRecoveryFailureStub{failure: failure}
	repository := newCandidateRepositoryStub()
	repository.pending = []PendingWorkflowStart{{RequestID: uuid.New(), RequestVersionID: uuid.New()}}
	reconciler := newCandidateReconcilerForTest(t, repository, candidateArgoStub{}, workflows)
	if _, err := reconciler.ReconcileOnce(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("candidate recovery error = %v", err)
	}
	if len(workflows.started) != 1 || workflows.calls != 1 {
		t.Fatalf("candidate starts = %d, recoveries = %d", len(workflows.started), workflows.calls)
	}
}

func recoverySnapshot(t *testing.T, change func(*deploydomain.WorkflowDocument)) WorkflowRuntimeSnapshot {
	t.Helper()
	policy := deploydomain.ReviewPolicy{Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{uuid.New()}}
	document := workflowReviewThenDeploy(policy)
	if change != nil {
		change(&document)
	}
	snapshot := runtimeSnapshot(t, document)
	instance := startRuntimeInstance(t, snapshot)
	snapshot.Instance = &instance
	task := runtimeReviewTask(t, snapshot, policy)
	task.Status = deploydomain.ReviewTaskApproved
	snapshot.CurrentReview = &task
	snapshot.NextReviewStage = 2
	return snapshot
}

type workflowRecoveryRepositoryStub struct {
	workflowRuntimeRepositoryStub
	snapshots map[uuid.UUID]WorkflowRuntimeSnapshot
	applied   []WorkflowTransitionChange
	listErr   error
	loadErr   error
	applyErr  error
}

func recoveryRepository(snapshots ...WorkflowRuntimeSnapshot) *workflowRecoveryRepositoryStub {
	repository := &workflowRecoveryRepositoryStub{snapshots: make(map[uuid.UUID]WorkflowRuntimeSnapshot)}
	for _, snapshot := range snapshots {
		repository.snapshots[snapshot.RequestVersionID] = snapshot
		repository.recoveries = append(repository.recoveries, ApprovedReviewRecovery{RequestVersionID: snapshot.RequestVersionID})
	}
	return repository
}

func (repository *workflowRecoveryRepositoryStub) Load(_ context.Context, id uuid.UUID) (WorkflowRuntimeSnapshot, error) {
	return repository.snapshots[id], repository.loadErr
}

func (repository *workflowRecoveryRepositoryStub) ListApprovedReviewRecoveries(context.Context) ([]ApprovedReviewRecovery, error) {
	return repository.recoveries, repository.listErr
}

func (repository *workflowRecoveryRepositoryStub) ApplyTransition(_ context.Context, change WorkflowTransitionChange) error {
	repository.applied = append(repository.applied, change)
	return repository.applyErr
}

type candidateRecoveryFailureStub struct {
	candidateWorkflowStarterStub
	failure error
	calls   int
}

func (starter *candidateRecoveryFailureStub) RecoverApprovedReviews(context.Context) error {
	starter.calls++
	return starter.failure
}

type recoveryAssignmentFailureStub struct{ failure error }

func (resolver recoveryAssignmentFailureStub) Resolve(context.Context, deploydomain.ReviewPolicy, authz.Scope) (deploydomain.ReviewAssignment, error) {
	return deploydomain.ReviewAssignment{}, resolver.failure
}
