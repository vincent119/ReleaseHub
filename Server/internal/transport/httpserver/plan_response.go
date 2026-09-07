package httpserver

import (
	"github.com/gin-gonic/gin"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func bindingResponse(value *deploydomain.DeploymentBinding, c *gin.Context) contract.DeploymentBindingResponse {
	if value == nil {
		return contract.DeploymentBindingResponse{Data: nil, Meta: responseMeta(c)}
	}
	data := contract.DeploymentBinding{
		Id: value.ID, OrganizationId: value.OrganizationID, ProjectId: value.ProjectID,
		EnvironmentId: value.EnvironmentID, WorkflowVersionId: value.WorkflowVersionID,
		PlanVersionId: value.PlanVersionID, Version: int64(value.Version),
	}
	return contract.DeploymentBindingResponse{Data: &data, Meta: responseMeta(c)}
}

func planResponses(values []deploydomain.DeploymentPlan) []contract.DeploymentPlan {
	result := make([]contract.DeploymentPlan, 0, len(values))
	for _, value := range values {
		result = append(result, planResponse(value))
	}
	return result
}

func planResponse(value deploydomain.DeploymentPlan) contract.DeploymentPlan {
	return contract.DeploymentPlan{
		Id: value.ID, OwnerKind: contract.DeploymentPlanOwnerKind(value.OwnerKind),
		OwnerProjectId: value.OwnerProjectID, Name: value.Name, Description: value.Description,
		Active: value.Active, Versions: planVersionResponses(value.Versions),
	}
}

func planVersionResponses(values []deploydomain.DeploymentPlanVersion) []contract.DeploymentPlanVersion {
	result := make([]contract.DeploymentPlanVersion, 0, len(values))
	for _, value := range values {
		result = append(result, planVersionResponse(value))
	}
	return result
}

func planVersionResponse(value deploydomain.DeploymentPlanVersion) contract.DeploymentPlanVersion {
	return contract.DeploymentPlanVersion{
		Id: value.ID, PlanId: value.PlanID, VersionNumber: int64(value.VersionNumber),
		Lifecycle: contract.DefinitionLifecycle(value.Lifecycle), Document: planDocumentResponse(value.Document),
		LockVersion: int64(value.LockVersion), CreatedAt: value.CreatedAt,
		PublishedAt: value.PublishedAt, DisabledAt: value.DisabledAt,
	}
}

func planDocumentResponse(value deploydomain.DeploymentPlanDocument) contract.DeploymentPlanDocument {
	nodes := make([]contract.DeploymentPlanNode, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, planNodeResponse(node))
	}
	edges := make([]contract.DeploymentPlanEdge, 0, len(value.Edges))
	for _, edge := range value.Edges {
		edges = append(edges, contract.DeploymentPlanEdge{
			From: edge.From, To: edge.To, Condition: contract.DeploymentPlanEdgeCondition(edge.Condition),
		})
	}
	return contract.DeploymentPlanDocument{Nodes: nodes, Edges: edges, MaxParallel: value.MaxParallel}
}

func planNodeResponse(value deploydomain.DeploymentPlanNode) contract.DeploymentPlanNode {
	condition := value.SuccessCondition
	return contract.DeploymentPlanNode{
		Key: value.Key, ApplicationKey: value.ApplicationKey, Order: value.Order,
		SuccessCondition: contract.DeploymentPlanCondition{
			SyncStatuses: condition.SyncStatuses, HealthStatuses: condition.HealthStatuses,
			StabilizationSeconds: condition.StabilizationSeconds, TimeoutSeconds: condition.TimeoutSeconds,
		},
	}
}
