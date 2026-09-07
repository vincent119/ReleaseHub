package application

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (s *ExecutionControlService) validateRetry(ctx context.Context, principal RequestPrincipal, snapshot ExecutionControlSnapshot, input RetryExecutionInput) error {
	if input.ExpectedVersion == 0 || !validRuntimeIdempotencyKey(input.IdempotencyKey) || !validSelectedFailures(snapshot.Execution, input.ApplicationIDs) {
		return ErrExecutionInvalid
	}
	if snapshot.Execution.LockVersion != input.ExpectedVersion || !s.workflowAllows(snapshot, "deployment_request.retry") {
		return ErrExecutionConflict
	}
	return s.authorize(ctx, principal, "deployment_request.retry", snapshot.Scope)
}

func validSelectedFailures(execution DeploymentExecution, selected []uuid.UUID) bool {
	if execution.Status != string(deploydomain.ExecutionFailed) && execution.Status != string(deploydomain.ExecutionPartialFailed) {
		return false
	}
	if len(selected) == 0 || len(selected) != len(uniqueUUIDs(selected)) {
		return false
	}
	for _, applicationID := range selected {
		if !executionHasFailedApplication(execution, applicationID) {
			return false
		}
	}
	return true
}

func executionHasFailedApplication(execution DeploymentExecution, applicationID uuid.UUID) bool {
	return slices.ContainsFunc(execution.Nodes, func(node DeploymentExecutionNode) bool {
		return node.ApplicationID == applicationID && node.Status == "Failed"
	})
}

func uniqueUUIDs(values []uuid.UUID) map[uuid.UUID]struct{} {
	result := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			result[value] = struct{}{}
		}
	}
	return result
}

func (s *ExecutionControlService) validateTerminate(ctx context.Context, principal RequestPrincipal, snapshot ExecutionControlSnapshot, input TerminateExecutionInput) error {
	if input.ExpectedVersion == 0 || !validReason(input.Reason) || !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrExecutionInvalid
	}
	if snapshot.Execution.LockVersion != input.ExpectedVersion || !terminateStatus(snapshot.Execution.Status) ||
		!s.workflowAllows(snapshot, "deployment_request.terminate") {
		return ErrExecutionConflict
	}
	return s.authorize(ctx, principal, "deployment_request.terminate", snapshot.Scope)
}

func terminateStatus(status string) bool {
	return status == "Running" || status == string(deploydomain.ExecutionPartialFailed)
}

func (s *ExecutionControlService) validateUnlock(ctx context.Context, principal RequestPrincipal, snapshot ExecutionControlSnapshot, input UnlockExecutionInput) error {
	if input.ExpectedVersion == 0 || !validReason(input.Reason) || !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrExecutionInvalid
	}
	if snapshot.Execution.LockVersion != input.ExpectedVersion || snapshot.Execution.Status != "Terminated" ||
		len(input.ActualStates) != len(snapshot.Execution.Nodes) {
		return ErrExecutionConflict
	}
	return s.authorize(ctx, principal, "deployment_request.unlock", snapshot.Scope)
}

func (s *ExecutionControlService) workflowAllows(snapshot ExecutionControlSnapshot, permission string) bool {
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Workflow)
	if err != nil {
		return false
	}
	facts := map[deploydomain.WorkflowFact]string{
		deploydomain.WorkflowFactDeploymentStatus: snapshot.Execution.Status,
		deploydomain.WorkflowFactRequestClass:     snapshot.Classification,
	}
	return engine.AllowsPermission(snapshot.WorkflowInstance.CurrentStateKey, permission, facts)
}

func (s *ExecutionControlService) workflowCommand(snapshot ExecutionControlSnapshot, permission string, actorID uuid.UUID, now time.Time) (deploydomain.WorkflowResult, error) {
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Workflow)
	if err != nil {
		return deploydomain.WorkflowResult{}, ErrExecutionConflict
	}
	facts := map[deploydomain.WorkflowFact]string{
		deploydomain.WorkflowFactDeploymentStatus: snapshot.Execution.Status,
		deploydomain.WorkflowFactRequestClass:     snapshot.Classification,
	}
	result, err := engine.ApplyPermissionAction(snapshot.WorkflowInstance, deploydomain.WorkflowTransitionCommand{
		Permission: permission, ActorID: actorID, PermissionGranted: true, Facts: facts, OccurredAt: now,
	})
	if err != nil {
		return deploydomain.WorkflowResult{}, ErrExecutionConflict
	}
	return result, nil
}

func validReason(value string) bool {
	length := len(strings.TrimSpace(value))
	return length > 0 && length <= 10000
}

func (s *ExecutionControlService) terminateActiveNodes(ctx context.Context, nodes []DeploymentExecutionNode) error {
	for _, node := range nodes {
		if node.Status != "Syncing" && node.Status != "Stabilizing" {
			continue
		}
		if err := s.terminator.TerminateApplication(ctx, node.Identity, node.ArgoProject); err != nil {
			return fmt.Errorf("terminate Argo CD Application %s: %w", node.NodeKey, err)
		}
	}
	return nil
}
