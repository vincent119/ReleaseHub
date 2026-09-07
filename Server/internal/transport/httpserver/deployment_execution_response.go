package httpserver

import (
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func deploymentExecutionResponse(value deployapp.DeploymentExecution) contract.DeploymentExecution {
	return contract.DeploymentExecution{
		Id: value.ID, RequestVersionId: value.RequestVersionID, PlanVersionId: value.PlanVersionID,
		Attempt: value.Attempt, Status: contract.DeploymentExecutionStatus(value.Status),
		TriggerKind: contract.DeploymentExecutionTriggerKind(value.TriggerKind), LockVersion: int64(value.LockVersion),
		Nodes: deploymentExecutionNodes(value.Nodes), CreatedAt: value.CreatedAt,
		StartedAt: value.StartedAt, CompletedAt: value.CompletedAt,
	}
}

func deploymentExecutionNodes(values []deployapp.DeploymentExecutionNode) []contract.DeploymentExecutionNode {
	result := make([]contract.DeploymentExecutionNode, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentExecutionNode{
			Id: value.ID, ApplicationId: value.ApplicationID, NodeKey: value.NodeKey,
			Status: contract.DeploymentExecutionNodeStatus(value.Status), OperationId: value.OperationID,
			SyncStatus: value.SyncStatus, HealthStatus: value.HealthStatus,
			ActualRevision: value.ActualRevision, ActualImages: deploymentRequestImages(value.ActualImages),
			ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage,
		})
	}
	return result
}
