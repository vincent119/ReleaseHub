package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func createPlanInput(c *gin.Context) (deployapp.CreatePlanInput, error) {
	var body contract.CreateDeploymentPlanRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		return deployapp.CreatePlanInput{}, err
	}
	document, err := planDocumentFromContract(body.Document)
	if err != nil {
		return deployapp.CreatePlanInput{}, err
	}
	return deployapp.CreatePlanInput{
		OwnerKind:      deploydomain.DeploymentPlanOwnerKind(body.OwnerKind),
		OwnerProjectID: body.OwnerProjectId, Name: body.Name,
		Description: optionalString(body.Description), Document: document,
		RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func createPlanVersionInput(c *gin.Context, planID uuid.UUID) (deployapp.CreatePlanVersionInput, error) {
	var body contract.CreateDeploymentPlanVersionRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.CreatePlanVersionInput{}, errors.New("deployment plan version request is invalid")
	}
	document, err := planDocumentFromContract(body.Document)
	if err != nil {
		return deployapp.CreatePlanVersionInput{}, err
	}
	return deployapp.CreatePlanVersionInput{
		PlanID: planID, ExpectedVersion: uint64(body.ExpectedVersion),
		Document: document, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func changePlanLifecycleInput(c *gin.Context, planID, versionID uuid.UUID) (deployapp.ChangePlanLifecycleInput, error) {
	var body contract.DefinitionLifecycleRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.ChangePlanLifecycleInput{}, errors.New("deployment plan lifecycle request is invalid")
	}
	return deployapp.ChangePlanLifecycleInput{
		PlanID: planID, VersionID: versionID, ExpectedVersion: uint64(body.ExpectedVersion),
		Lifecycle: deploydomain.DefinitionLifecycle(body.Lifecycle), RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func bindDefinitionsInput(c *gin.Context) (deployapp.BindDefinitionsInput, error) {
	var body contract.BindDeploymentDefinitionsRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 0 {
		return deployapp.BindDefinitionsInput{}, errors.New("deployment binding request is invalid")
	}
	return deployapp.BindDefinitionsInput{
		OrganizationID: body.OrganizationId, ProjectID: body.ProjectId, EnvironmentID: body.EnvironmentId,
		WorkflowVersionID: body.WorkflowVersionId, PlanVersionID: body.PlanVersionId,
		ExpectedVersion: uint64(body.ExpectedVersion), RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func planDocumentFromContract(value contract.DeploymentPlanDocument) (deploydomain.DeploymentPlanDocument, error) {
	nodes := make([]deploydomain.DeploymentPlanNode, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, planNodeFromContract(node))
	}
	edges := make([]deploydomain.DeploymentPlanEdge, 0, len(value.Edges))
	for _, edge := range value.Edges {
		edges = append(edges, deploydomain.DeploymentPlanEdge{
			From: edge.From, To: edge.To, Condition: deploydomain.DeploymentPlanEdgeCondition(edge.Condition),
		})
	}
	return deploydomain.NewDeploymentPlanDocument(deploydomain.DeploymentPlanDocument{
		Nodes: nodes, Edges: edges, MaxParallel: value.MaxParallel,
	})
}

func planNodeFromContract(value contract.DeploymentPlanNode) deploydomain.DeploymentPlanNode {
	return deploydomain.DeploymentPlanNode{
		Key: value.Key, ApplicationKey: value.ApplicationKey, Order: value.Order,
		SuccessCondition: deploydomain.DeploymentPlanCondition{
			SyncStatuses:         value.SuccessCondition.SyncStatuses,
			HealthStatuses:       value.SuccessCondition.HealthStatuses,
			StabilizationSeconds: value.SuccessCondition.StabilizationSeconds,
			TimeoutSeconds:       value.SuccessCondition.TimeoutSeconds,
		},
	}
}
