package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// ExecutionControlService coordinates D10 commands without bypassing Workflow or RBAC.
type ExecutionControlService struct {
	repository   ExecutionControlRepository
	authorizer   WorkflowAuthorizer
	actualStates ExecutionActualStateReader
	terminator   ExecutionOperationTerminator
	clock        WorkflowClock
}

// NewExecutionControlService creates the execution query and command use case.
func NewExecutionControlService(options ExecutionControlServiceOptions) (*ExecutionControlService, error) {
	if options.Repository == nil || options.Authorizer == nil || options.ActualStates == nil ||
		options.Terminator == nil || options.Clock == nil {
		return nil, errors.New("execution control dependencies are required")
	}
	return &ExecutionControlService{
		repository: options.Repository, authorizer: options.Authorizer,
		actualStates: options.ActualStates, terminator: options.Terminator, clock: options.Clock,
	}, nil
}

// Get returns one execution after resolving its persisted Environment scope.
func (s *ExecutionControlService) Get(ctx context.Context, principal RequestPrincipal, executionID uuid.UUID) (DeploymentExecution, error) {
	snapshot, err := s.repository.LoadByID(ctx, executionID)
	if err != nil {
		return DeploymentExecution{}, err
	}
	if err := s.authorize(ctx, principal, "deployment_request.view", snapshot.Scope); err != nil {
		return DeploymentExecution{}, err
	}
	return snapshot.Execution, nil
}

// Retry creates a distinct attempt containing only selected failed nodes as runnable work.
func (s *ExecutionControlService) Retry(ctx context.Context, principal RequestPrincipal, input RetryExecutionInput) (DeploymentExecution, error) {
	prepared, err := s.prepareCommand(ctx, principal, commandLocator{requestID: input.RequestID, versionID: input.RequestVersionID},
		commandReplay{command: "Retry", key: input.IdempotencyKey, hash: retryRequestHash(input)})
	if err != nil {
		return DeploymentExecution{}, err
	}
	if prepared.found {
		return prepared.result, nil
	}
	if err := s.validateRetry(ctx, principal, prepared.snapshot, input); err != nil {
		return DeploymentExecution{}, err
	}
	return s.retryPrepared(ctx, principal, input, prepared)
}

func (s *ExecutionControlService) retryPrepared(ctx context.Context, principal RequestPrincipal, input RetryExecutionInput, prepared preparedCommand) (DeploymentExecution, error) {
	now := s.clock.Now()
	workflowResult, err := s.workflowCommand(prepared.snapshot, "deployment_request.retry", principal.UserID, now)
	if err != nil {
		return DeploymentExecution{}, err
	}
	return s.repository.Retry(ctx, ExecutionRetryChange{
		Mutation: commandMutation(principal.UserID, input.RequestTraceID, prepared.replay, now), WorkflowResult: workflowResult,
		RequestID: input.RequestID, RequestVersionID: input.RequestVersionID,
		ExecutionID: prepared.snapshot.Execution.ID, ExpectedVersion: input.ExpectedVersion,
		ApplicationIDs: slices.Clone(input.ApplicationIDs),
	})
}

// Terminate stops active Argo CD operations and preserves locks for manual reconciliation.
func (s *ExecutionControlService) Terminate(ctx context.Context, principal RequestPrincipal, input TerminateExecutionInput) (DeploymentExecution, error) {
	prepared, err := s.prepareCommand(ctx, principal, commandLocator{requestID: input.RequestID, versionID: input.RequestVersionID},
		commandReplay{command: "Terminate", key: input.IdempotencyKey, hash: terminateRequestHash(input)})
	if err != nil {
		return DeploymentExecution{}, err
	}
	if prepared.found {
		return prepared.result, nil
	}
	if err := s.validateTerminate(ctx, principal, prepared.snapshot, input); err != nil {
		return DeploymentExecution{}, err
	}
	return s.terminatePrepared(ctx, principal, input, prepared)
}

func (s *ExecutionControlService) terminatePrepared(ctx context.Context, principal RequestPrincipal, input TerminateExecutionInput, prepared preparedCommand) (DeploymentExecution, error) {
	now := s.clock.Now()
	workflowResult, err := s.workflowCommand(prepared.snapshot, "deployment_request.terminate", principal.UserID, now)
	if err != nil {
		return DeploymentExecution{}, err
	}
	if err := s.terminateActiveNodes(ctx, prepared.snapshot.Execution.Nodes); err != nil {
		return DeploymentExecution{}, err
	}
	return s.repository.Terminate(ctx, terminateChange(prepared.command(principal, now), input, workflowResult))
}

// Unlock releases a terminated attempt only after fresh actual-state comparison.
func (s *ExecutionControlService) Unlock(ctx context.Context, principal RequestPrincipal, input UnlockExecutionInput) (DeploymentExecution, error) {
	prepared, err := s.prepareCommand(ctx, principal, commandLocator{requestID: input.RequestID, versionID: input.RequestVersionID},
		commandReplay{command: "Unlock", key: input.IdempotencyKey, hash: unlockRequestHash(input)})
	if err != nil {
		return DeploymentExecution{}, err
	}
	if prepared.found {
		return prepared.result, nil
	}
	if err := s.validateUnlock(ctx, principal, prepared.snapshot, input); err != nil {
		return DeploymentExecution{}, err
	}
	fresh, err := s.confirmActualStates(ctx, prepared.snapshot.Execution.Nodes, input.ActualStates)
	if err != nil {
		return DeploymentExecution{}, err
	}
	command := prepared.command(principal, s.clock.Now())
	return s.repository.Unlock(ctx, unlockChange(command, input, fresh))
}

func (s *ExecutionControlService) confirmActualStates(ctx context.Context, nodes []DeploymentExecutionNode, expected []ActualState) ([]ActualState, error) {
	fresh, err := s.readActualStates(ctx, nodes)
	if err != nil {
		return nil, fmt.Errorf("read actual deployment state: %w", err)
	}
	if !actualStatesEqual(fresh, expected) {
		return nil, ErrExecutionConflict
	}
	return fresh, nil
}

type commandLocator struct{ requestID, versionID uuid.UUID }

type preparedCommand struct {
	snapshot ExecutionControlSnapshot
	replay   commandReplay
	result   DeploymentExecution
	found    bool
}

type executionCommand struct {
	principal RequestPrincipal
	snapshot  ExecutionControlSnapshot
	replay    commandReplay
	now       time.Time
}

func (p preparedCommand) command(principal RequestPrincipal, now time.Time) executionCommand {
	return executionCommand{principal: principal, snapshot: p.snapshot, replay: p.replay, now: now}
}

func (s *ExecutionControlService) prepareCommand(ctx context.Context, principal RequestPrincipal, locator commandLocator, replay commandReplay) (preparedCommand, error) {
	snapshot, err := s.loadCommandSnapshot(ctx, locator.requestID, locator.versionID)
	if err != nil {
		return preparedCommand{}, err
	}
	result, found, err := s.replay(ctx, principal, snapshot, replay)
	return preparedCommand{snapshot: snapshot, replay: replay, result: result, found: found}, err
}

func terminateChange(command executionCommand, input TerminateExecutionInput, result deploydomain.WorkflowResult) ExecutionTerminateChange {
	return ExecutionTerminateChange{
		Mutation: commandMutation(command.principal.UserID, input.RequestTraceID, command.replay, command.now), WorkflowResult: result,
		ExecutionID:     command.snapshot.Execution.ID,
		ExpectedVersion: input.ExpectedVersion, Reason: strings.TrimSpace(input.Reason),
	}
}

func unlockChange(command executionCommand, input UnlockExecutionInput, states []ActualState) ExecutionUnlockChange {
	return ExecutionUnlockChange{
		Mutation: commandMutation(command.principal.UserID, input.RequestTraceID, command.replay, command.now), ExecutionID: command.snapshot.Execution.ID,
		ExpectedVersion: input.ExpectedVersion, Reason: strings.TrimSpace(input.Reason), ActualStates: states,
	}
}

func (s *ExecutionControlService) loadCommandSnapshot(ctx context.Context, requestID, versionID uuid.UUID) (ExecutionControlSnapshot, error) {
	if requestID == uuid.Nil || versionID == uuid.Nil {
		return ExecutionControlSnapshot{}, ErrExecutionInvalid
	}
	return s.repository.LoadLatest(ctx, requestID, versionID)
}

func (s *ExecutionControlService) authorize(ctx context.Context, principal RequestPrincipal, key string, scope authz.Scope) error {
	permission, _ := authz.NewPermission(key)
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize deployment execution: %w", err)
	}
	if !allowed {
		return ErrExecutionForbidden
	}
	return nil
}

func commandMutation(actorID uuid.UUID, requestID string, replay commandReplay, now time.Time) ExecutionMutation {
	return ExecutionMutation{ActorID: actorID, RequestID: requestID,
		IdempotencyKey: strings.TrimSpace(replay.key), RequestHash: replay.hash, OccurredAt: now.UTC()}
}
