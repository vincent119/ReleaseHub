package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (e *DeploymentExecutor) resumeNode(ctx context.Context, execution nodeExecution) error {
	target := execution.target.Preflight
	application, err := e.argo.GetApplication(ctx, target.Identity, target.ArgoProject)
	if err != nil {
		return e.failNode(ctx, execution, "operation_reconcile_failed", err)
	}
	if application.OperationID != execution.operationID {
		cause := errors.New("deployment operation identity cannot be reconciled")
		return e.failNode(ctx, execution, "operation_identity_lost", cause)
	}
	return e.watchNode(ctx, execution, application)
}

func (e *DeploymentExecutor) watchNode(ctx context.Context, execution nodeExecution, application argodomain.Application) error {
	timing := conditionTiming{startedAt: e.now().UTC()}
	for {
		timing.now = e.now().UTC()
		result, err := e.evaluateObservation(ctx, execution, application, &timing)
		if err != nil || result {
			return err
		}
		application, err = e.nextObservation(ctx, execution, application.ResourceVersion)
		if err != nil {
			return e.failNode(ctx, execution, "watch_failed", err)
		}
	}
}

func (e *DeploymentExecutor) evaluateObservation(ctx context.Context, execution nodeExecution, application argodomain.Application, timing *conditionTiming) (bool, error) {
	if application.OperationID == execution.operationID && application.OperationPhase == "Failed" {
		cause := errors.New("Argo CD operation failed")
		return true, e.failNode(ctx, execution, "operation_failed", cause)
	}
	timing.stableSince = conditionStableSince(execution.target.Node, application, timing.stableSince, timing.now)
	condition := nodeConditionResult(execution.target.Node, application, *timing)
	if condition == deploydomain.DeploymentConditionSucceeded && operationMatches(execution, application) {
		return true, e.saveNode(ctx, execution, application, "Succeeded")
	}
	if condition == deploydomain.DeploymentConditionTimedOut {
		cause := errors.New("deployment success condition timed out")
		return true, e.failNode(ctx, execution, "condition_timeout", cause)
	}
	return false, nil
}

func operationMatches(execution nodeExecution, application argodomain.Application) bool {
	if application.OperationID != execution.operationID {
		return false
	}
	snapshot := execution.target.Preflight.Snapshot
	if len(snapshot.TargetRevisions) > 0 {
		return slices.Equal(snapshot.TargetRevisions, application.ResolvedRevisions)
	}
	return snapshot.TargetRevision == application.ResolvedRevision
}

func (e *DeploymentExecutor) nextObservation(ctx context.Context, execution nodeExecution, resourceVersion string) (argodomain.Application, error) {
	target := execution.target.Preflight
	watch, err := e.argo.WatchApplication(ctx, target.Identity, target.ArgoProject, resourceVersion)
	if err == nil {
		value, received, receiveErr := receiveWatchObservation(ctx, watch, e.watchTimeout)
		watch.Close()
		if receiveErr == nil && received {
			return value, nil
		}
		err = receiveErr
	}
	return e.reconcileObservation(ctx, execution, err)
}

type watchObservation struct {
	application argodomain.Application
	err         error
}

func receiveWatchObservation(ctx context.Context, watch argodomain.ApplicationWatch, timeout time.Duration) (argodomain.Application, bool, error) {
	result := make(chan watchObservation, 1)
	go func() {
		application, err := watch.Recv()
		result <- watchObservation{application: application, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case value := <-result:
		return value.application, true, value.err
	case <-ctx.Done():
		return argodomain.Application{}, false, ctx.Err()
	case <-timer.C:
		return argodomain.Application{}, false, errors.New("Argo CD Application watch timed out")
	}
}

func (e *DeploymentExecutor) reconcileObservation(ctx context.Context, execution nodeExecution, watchErr error) (argodomain.Application, error) {
	target := execution.target.Preflight
	current, err := e.argo.GetApplication(ctx, target.Identity, target.ArgoProject)
	if err != nil {
		return argodomain.Application{}, errors.Join(watchErr, err)
	}
	if current.OperationID != execution.operationID {
		return argodomain.Application{}, errors.New("deployment operation identity cannot be reconciled")
	}
	return current, nil
}

func (e *DeploymentExecutor) saveNode(ctx context.Context, execution nodeExecution, application argodomain.Application, status string) error {
	images := []deploydomain.DeploymentRequestImageSnapshot(nil)
	if status == "Succeeded" {
		images = execution.target.Preflight.Snapshot.Images
	}
	return e.repository.UpdateNode(ctx, ExecutionNodeUpdate{
		ExecutionID: execution.executionID, ApplicationID: execution.target.Preflight.Snapshot.ApplicationID,
		Status: status, OperationID: execution.operationID, SyncStatus: application.SyncStatus,
		HealthStatus: application.HealthStatus, ActualRevision: application.ResolvedRevision,
		ActualImages: images,
		UpdatedAt:    e.now().UTC(),
	})
}

func (e *DeploymentExecutor) failNode(ctx context.Context, execution nodeExecution, code string, cause error) error {
	if err := e.repository.UpdateNode(ctx, ExecutionNodeUpdate{
		ExecutionID: execution.executionID, ApplicationID: execution.target.Preflight.Snapshot.ApplicationID,
		Status: "Failed", OperationID: execution.operationID, ErrorCode: code,
		ErrorMessage: cause.Error(), UpdatedAt: e.now().UTC(),
	}); err != nil {
		return fmt.Errorf("persist failed deployment node %s: %w", execution.target.Node.Key, err)
	}
	return nodeFailure{cause: fmt.Errorf("execute deployment node %s: %w", execution.target.Node.Key, cause)}
}

func nodeConditionResult(node deploydomain.DeploymentPlanNode, application argodomain.Application, timing conditionTiming) deploydomain.DeploymentConditionResult {
	engine, _ := deploydomain.NewDeploymentPlanEngine(
		deploydomain.DeploymentPlanDocument{Nodes: []deploydomain.DeploymentPlanNode{node}},
	)
	result, _ := engine.EvaluateCondition(node.Key, deploydomain.DeploymentNodeObservation{
		SyncStatus: application.SyncStatus, HealthStatus: application.HealthStatus,
		StableFor: timing.now.Sub(timing.stableSince), Elapsed: timing.now.Sub(timing.startedAt),
	})
	return result
}

func conditionStableSince(node deploydomain.DeploymentPlanNode, application argodomain.Application, previous, now time.Time) time.Time {
	condition := node.SuccessCondition
	if !slices.Contains(condition.SyncStatuses, application.SyncStatus) ||
		!slices.Contains(condition.HealthStatuses, application.HealthStatus) {
		return time.Time{}
	}
	if previous.IsZero() {
		return now
	}
	return previous
}
