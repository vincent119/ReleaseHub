package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestRetryExecutionSelectsFailedApplicationsOnly(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	failedID := repository.snapshot.Execution.Nodes[1].ApplicationID
	result, err := service.Retry(context.Background(), principal, RetryExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		ApplicationIDs: []uuid.UUID{failedID}, ExpectedVersion: 2, IdempotencyKey: "retry-1",
	})
	if err != nil || result.TriggerKind != "Retry" {
		t.Fatalf("Retry() = %#v, %v", result, err)
	}
	if len(repository.retry.ApplicationIDs) != 1 || repository.retry.ApplicationIDs[0] != failedID {
		t.Fatalf("retry selected unexpected Applications: %#v", repository.retry.ApplicationIDs)
	}
	if repository.retry.WorkflowResult.Instance.CurrentStateKey != "deploy" {
		t.Fatalf("retry workflow result = %#v", repository.retry.WorkflowResult)
	}
}

func TestRetryExecutionRejectsSucceededApplication(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	succeededID := repository.snapshot.Execution.Nodes[0].ApplicationID
	_, err := service.Retry(context.Background(), principal, RetryExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		ApplicationIDs: []uuid.UUID{succeededID}, ExpectedVersion: 2, IdempotencyKey: "retry-2",
	})
	if err != ErrExecutionInvalid || repository.retry.ExecutionID != uuid.Nil {
		t.Fatalf("succeeded Application retry = %v, %#v", err, repository.retry)
	}
}

func TestRetryExecutionReplaysImmutableCommandResult(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	repository.replayFound = true
	repository.replayResult = DeploymentExecution{ID: uuid.New(), TriggerKind: "Retry", Attempt: 2}
	result, err := service.Retry(context.Background(), principal, RetryExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		ApplicationIDs:  []uuid.UUID{repository.snapshot.Execution.Nodes[1].ApplicationID},
		ExpectedVersion: 2, IdempotencyKey: "retry-replay",
	})
	if err != nil || result.ID != repository.replayResult.ID || repository.retry.ExecutionID != uuid.Nil {
		t.Fatalf("retry replay = %#v, %v", result, err)
	}
}

func TestTerminateExecutionRequiresCurrentPermission(t *testing.T) {
	repository, service, principal := executionControlFixture(t, false)
	_, err := service.Terminate(context.Background(), principal, TerminateExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID, Reason: "operator requested stop",
		ExpectedVersion: 2, IdempotencyKey: "terminate-1",
	})
	if err != ErrExecutionForbidden {
		t.Fatalf("Terminate() error = %v", err)
	}
}

func TestTerminateExecutionStopsOnlyActiveArgoOperations(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	repository.snapshot.Execution.Status = "Running"
	repository.snapshot.Execution.Nodes[0].Status = "Syncing"
	terminator := service.terminator.(*operationTerminatorStub)
	_, err := service.Terminate(context.Background(), principal, TerminateExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		Reason: "operator requested stop", ExpectedVersion: 2, IdempotencyKey: "terminate-2",
	})
	if err != nil || terminator.calls != 1 || repository.terminate.ExecutionID == uuid.Nil {
		t.Fatalf("Terminate() = calls %d change %#v error %v", terminator.calls, repository.terminate, err)
	}
	if repository.terminate.WorkflowResult.Instance.CurrentStateKey != "done" {
		t.Fatalf("terminate workflow result = %#v", repository.terminate.WorkflowResult)
	}
}

func TestUnlockExecutionRequiresMatchingFreshActualState(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	repository.snapshot.Execution.Status = "Terminated"
	state := ActualState{ApplicationID: repository.snapshot.Execution.Nodes[0].ApplicationID,
		Revision: "different", Images: nil}
	_, err := service.Unlock(context.Background(), principal, UnlockExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		Reason: "confirmed inconsistent state", ExpectedVersion: 2,
		ActualStates:   []ActualState{state, {ApplicationID: repository.snapshot.Execution.Nodes[1].ApplicationID, Revision: "different"}},
		IdempotencyKey: "unlock-1",
	})
	if err != ErrExecutionConflict || repository.unlock.ExecutionID != uuid.Nil {
		t.Fatalf("Unlock() mismatch = %v, %#v", err, repository.unlock)
	}
}

func TestUnlockExecutionPersistsFreshActualState(t *testing.T) {
	repository, service, principal := executionControlFixture(t, true)
	repository.snapshot.Execution.Status = "Terminated"
	states := service.actualStates.(*actualStateReaderStub).states
	_, err := service.Unlock(context.Background(), principal, UnlockExecutionInput{
		RequestID: repository.snapshot.RequestID, RequestVersionID: repository.snapshot.Execution.RequestVersionID,
		Reason: "actual state confirmed", ExpectedVersion: 2,
		ActualStates: states, IdempotencyKey: "unlock-2",
	})
	if err != nil || len(repository.unlock.ActualStates) != len(states) {
		t.Fatalf("Unlock() = %#v, %v", repository.unlock, err)
	}
}

func executionControlFixture(t *testing.T, allowed bool) (*executionControlRepositoryStub, *ExecutionControlService, RequestPrincipal) {
	t.Helper()
	requestID, versionID := uuid.New(), uuid.New()
	workflow := actionWorkflowDocument(t)
	scope, _ := authz.NewEnvironmentScope(uuid.New(), uuid.New(), uuid.New())
	now := time.Now().UTC()
	repository := &executionControlRepositoryStub{snapshot: ExecutionControlSnapshot{
		RequestID: requestID, Scope: scope, Workflow: workflow, Classification: "Standard",
		WorkflowInstance: deploydomain.WorkflowInstance{
			ID: uuid.New(), RequestVersionID: versionID, WorkflowVersionID: uuid.New(),
			CurrentStateKey: "result", Status: deploydomain.WorkflowInstanceRunning,
			LockVersion: 2, StartedAt: now,
		},
		Execution: DeploymentExecution{ID: uuid.New(), RequestVersionID: versionID, Status: "PartialFailed", LockVersion: 2,
			Nodes: []DeploymentExecutionNode{{ApplicationID: uuid.New(), Status: "Succeeded"}, {ApplicationID: uuid.New(), Status: "Failed"}}},
	}}
	actual := &actualStateReaderStub{states: []ActualState{
		{ApplicationID: repository.snapshot.Execution.Nodes[0].ApplicationID, Revision: "revision-a"},
		{ApplicationID: repository.snapshot.Execution.Nodes[1].ApplicationID, Revision: "revision-b"},
	}}
	service, err := NewExecutionControlService(ExecutionControlServiceOptions{
		Repository: repository, Authorizer: executionAuthorizerStub{allowed: allowed},
		ActualStates: actual, Terminator: &operationTerminatorStub{}, Clock: executionClockStub{now: now},
	})
	if err != nil {
		t.Fatalf("NewExecutionControlService(): %v", err)
	}
	return repository, service, RequestPrincipal{UserID: uuid.New()}
}

func actionWorkflowDocument(t *testing.T) deploydomain.WorkflowDocument {
	t.Helper()
	document, err := deploydomain.NewWorkflowDocument(deploydomain.WorkflowDocument{
		InitialState: "result",
		States:       []deploydomain.WorkflowState{{Key: "result", Name: "Result", Type: deploydomain.WorkflowStateManualAction}, {Key: "deploy", Name: "Deploy", Type: deploydomain.WorkflowStateDeployment}, {Key: "done", Name: "Done", Type: deploydomain.WorkflowStateTerminal}},
		Transitions: []deploydomain.WorkflowTransition{
			{Key: "retry", From: "result", To: "deploy", Trigger: deploydomain.WorkflowTriggerManual, Permission: "deployment_request.retry", Conditions: []deploydomain.WorkflowCondition{{Fact: deploydomain.WorkflowFactDeploymentStatus, Operator: deploydomain.WorkflowOperatorIn, Values: []string{"Failed", "PartialFailed"}}}},
			{Key: "terminate", From: "result", To: "done", Trigger: deploydomain.WorkflowTriggerManual, Permission: "deployment_request.terminate", Conditions: []deploydomain.WorkflowCondition{{Fact: deploydomain.WorkflowFactDeploymentStatus, Operator: deploydomain.WorkflowOperatorIn, Values: []string{"Running", "PartialFailed"}}}},
		},
	})
	if err != nil {
		t.Fatalf("NewWorkflowDocument(): %v", err)
	}
	return document
}

type executionControlRepositoryStub struct {
	snapshot     ExecutionControlSnapshot
	retry        ExecutionRetryChange
	terminate    ExecutionTerminateChange
	unlock       ExecutionUnlockChange
	replayResult DeploymentExecution
	replayFound  bool
}

func (s *executionControlRepositoryStub) LoadByID(context.Context, uuid.UUID) (ExecutionControlSnapshot, error) {
	return s.snapshot, nil
}
func (s *executionControlRepositoryStub) LoadLatest(_ context.Context, requestID, versionID uuid.UUID) (ExecutionControlSnapshot, error) {
	if requestID != s.snapshot.RequestID || versionID != s.snapshot.Execution.RequestVersionID {
		return ExecutionControlSnapshot{}, ErrExecutionNotFound
	}
	return s.snapshot, nil
}
func (s *executionControlRepositoryStub) Replay(context.Context, ExecutionCommandReplay) (DeploymentExecution, bool, error) {
	return s.replayResult, s.replayFound, nil
}
func (s *executionControlRepositoryStub) Retry(_ context.Context, value ExecutionRetryChange) (DeploymentExecution, error) {
	s.retry = value
	result := s.snapshot.Execution
	result.TriggerKind = "Retry"
	return result, nil
}
func (s *executionControlRepositoryStub) Terminate(_ context.Context, value ExecutionTerminateChange) (DeploymentExecution, error) {
	s.terminate = value
	return s.snapshot.Execution, nil
}
func (s *executionControlRepositoryStub) Unlock(_ context.Context, value ExecutionUnlockChange) (DeploymentExecution, error) {
	s.unlock = value
	return s.snapshot.Execution, nil
}

type executionAuthorizerStub struct{ allowed bool }

func (s executionAuthorizerStub) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return s.allowed, nil
}

type actualStateReaderStub struct{ states []ActualState }

func (s *actualStateReaderStub) ReadActualState(_ context.Context, node DeploymentExecutionNode) (ActualState, error) {
	for _, state := range s.states {
		if state.ApplicationID == node.ApplicationID {
			return state, nil
		}
	}
	return ActualState{}, ErrExecutionNotFound
}

type operationTerminatorStub struct{ calls int }

func (s *operationTerminatorStub) TerminateApplication(context.Context, argodomain.ApplicationIdentity, string) error {
	s.calls++
	return nil
}

type executionClockStub struct{ now time.Time }

func (s executionClockStub) Now() time.Time { return s.now }
