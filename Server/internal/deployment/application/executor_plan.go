package application

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type nodeBatchFailure struct{ causes []error }

func (e nodeBatchFailure) Error() string {
	return fmt.Sprintf("%d deployment nodes failed", len(e.causes))
}

type nodeFailure struct{ cause error }

func (e nodeFailure) Error() string { return e.cause.Error() }

func (e nodeFailure) Unwrap() error { return e.cause }

func (e *DeploymentExecutor) executePlan(ctx context.Context, snapshot ExecutionSnapshot) error {
	engine, err := deploydomain.NewDeploymentPlanEngine(snapshot.Plan)
	if err != nil {
		return err
	}
	runtimes := queuedNodeRuntimes(snapshot.Targets)
	for hasQueuedNode(runtimes) {
		ready, readyErr := engine.ReadyNodes(planEvaluation(snapshot, runtimes, e.maxParallel))
		if readyErr != nil || len(ready) == 0 {
			return errors.Join(readyErr, errors.New("deployment plan has no executable node"))
		}
		if err := e.executeReady(ctx, snapshot, ready, runtimes); err != nil {
			return err
		}
	}
	if !allNodesSucceeded(runtimes) {
		return nodeBatchFailure{causes: []error{errors.New("deployment plan contains unsuccessful nodes")}}
	}
	return nil
}

func (e *DeploymentExecutor) executeReady(ctx context.Context, snapshot ExecutionSnapshot, ready []deploydomain.DeploymentPlanNode, runtimes map[string]deploydomain.DeploymentNodeRuntime) error {
	var wait sync.WaitGroup
	results := make(chan nodeResult, len(ready))
	for _, node := range ready {
		wait.Add(1)
		go func(value deploydomain.DeploymentPlanNode) {
			defer wait.Done()
			target := executionTarget(snapshot.Targets, value.Key)
			results <- nodeResult{key: value.Key, err: e.executeNode(ctx, snapshot.ID, target)}
		}(node)
	}
	wait.Wait()
	close(results)
	return collectNodeResults(runtimes, results)
}

func (e *DeploymentExecutor) executeNode(ctx context.Context, executionID uuid.UUID, target ExecutionTarget) error {
	operationID := executionID.String() + ":" + target.Node.Key
	execution := nodeExecution{executionID: executionID, target: target, operationID: operationID}
	if target.OperationID != "" {
		return e.resumeNode(ctx, execution)
	}
	if err := e.saveNode(ctx, execution, argodomain.Application{}, "Syncing"); err != nil {
		return err
	}
	application, err := e.argo.SyncApplication(ctx, pinnedSyncRequest(execution))
	if err != nil {
		return e.failNode(ctx, execution, "sync_failed", err)
	}
	return e.watchNode(ctx, execution, application)
}

func pinnedSyncRequest(execution nodeExecution) argodomain.SyncRequest {
	target := execution.target.Preflight
	return argodomain.SyncRequest{
		Identity: target.Identity, Project: target.ArgoProject,
		Revision: target.Snapshot.TargetRevision, Revisions: target.Snapshot.TargetRevisions,
		OperationID: execution.operationID,
	}
}

func queuedNodeRuntimes(targets []ExecutionTarget) map[string]deploydomain.DeploymentNodeRuntime {
	result := make(map[string]deploydomain.DeploymentNodeRuntime, len(targets))
	for _, target := range targets {
		status := deploydomain.DeploymentNodeQueued
		switch target.Status {
		case "Succeeded":
			status = deploydomain.DeploymentNodeSucceeded
		case "Failed":
			status = deploydomain.DeploymentNodeFailed
		case "Skipped":
			status = deploydomain.DeploymentNodeSkipped
		}
		result[target.Node.Key] = deploydomain.DeploymentNodeRuntime{Status: status}
	}
	return result
}

func planEvaluation(snapshot ExecutionSnapshot, runtimes map[string]deploydomain.DeploymentNodeRuntime, maximum int) deploydomain.DeploymentPlanEvaluation {
	keys := make([]string, 0, len(snapshot.Targets))
	for _, target := range snapshot.Targets {
		keys = append(keys, target.Preflight.Snapshot.ApplicationKey)
	}
	return deploydomain.DeploymentPlanEvaluation{ApplicationKeys: keys, Nodes: runtimes, MaxParallel: maximum}
}

func executionTarget(targets []ExecutionTarget, nodeKey string) ExecutionTarget {
	for _, target := range targets {
		if target.Node.Key == nodeKey {
			return target
		}
	}
	return ExecutionTarget{}
}

func collectNodeResults(runtimes map[string]deploydomain.DeploymentNodeRuntime, results <-chan nodeResult) error {
	var failures, infrastructure []error
	for node := range results {
		status, business := classifyNodeResult(node.err)
		failures = append(failures, business...)
		if node.err != nil && len(business) == 0 {
			infrastructure = append(infrastructure, node.err)
		}
		runtimes[node.key] = deploydomain.DeploymentNodeRuntime{Status: status}
	}
	if len(infrastructure) > 0 {
		return errors.Join(infrastructure...)
	}
	if len(failures) > 0 {
		return nodeBatchFailure{causes: failures}
	}
	return nil
}

func classifyNodeResult(err error) (deploydomain.DeploymentNodeStatus, []error) {
	if err == nil {
		return deploydomain.DeploymentNodeSucceeded, nil
	}
	var business nodeFailure
	if errors.As(err, &business) {
		return deploydomain.DeploymentNodeFailed, []error{err}
	}
	return deploydomain.DeploymentNodeFailed, nil
}

func allNodesSucceeded(values map[string]deploydomain.DeploymentNodeRuntime) bool {
	for _, value := range values {
		if value.Status != deploydomain.DeploymentNodeSucceeded {
			return false
		}
	}
	return true
}

func hasQueuedNode(values map[string]deploydomain.DeploymentNodeRuntime) bool {
	for _, value := range values {
		if value.Status == deploydomain.DeploymentNodeQueued {
			return true
		}
	}
	return false
}
