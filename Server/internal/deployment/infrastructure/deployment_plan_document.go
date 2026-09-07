package infrastructure

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type deploymentPlanDocumentModel struct {
	Nodes       []deploymentPlanNodeModel `json:"nodes"`
	Edges       []deploymentPlanEdgeModel `json:"edges"`
	MaxParallel *int                      `json:"maxParallel,omitempty"`
}

type deploymentPlanNodeModel struct {
	Key              string                       `json:"key"`
	ApplicationKey   string                       `json:"applicationKey"`
	Order            int                          `json:"order"`
	SuccessCondition deploymentPlanConditionModel `json:"successCondition"`
}

type deploymentPlanConditionModel struct {
	SyncStatuses         []string `json:"syncStatuses"`
	HealthStatuses       []string `json:"healthStatuses"`
	StabilizationSeconds int      `json:"stabilizationSeconds"`
	TimeoutSeconds       *int     `json:"timeoutSeconds,omitempty"`
}

type deploymentPlanEdgeModel struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition"`
}

func marshalDeploymentPlanDocument(value deploydomain.DeploymentPlanDocument) (datatypes.JSON, error) {
	model := deploymentPlanDocumentModel{MaxParallel: value.MaxParallel}
	for _, node := range value.Nodes {
		model.Nodes = append(model.Nodes, deploymentPlanNodeToModel(node))
	}
	for _, edge := range value.Edges {
		model.Edges = append(model.Edges, deploymentPlanEdgeToModel(edge))
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("marshal deployment plan document: %w", err)
	}
	return encoded, nil
}

func unmarshalDeploymentPlanDocument(value datatypes.JSON) (deploydomain.DeploymentPlanDocument, error) {
	var model deploymentPlanDocumentModel
	if err := json.Unmarshal(value, &model); err != nil {
		return deploydomain.DeploymentPlanDocument{}, fmt.Errorf("unmarshal deployment plan document: %w", err)
	}
	document := deploydomain.DeploymentPlanDocument{MaxParallel: model.MaxParallel}
	for _, node := range model.Nodes {
		document.Nodes = append(document.Nodes, deploymentPlanNodeFromModel(node))
	}
	for _, edge := range model.Edges {
		document.Edges = append(document.Edges, deploymentPlanEdgeFromModel(edge))
	}
	return deploydomain.NewDeploymentPlanDocument(document)
}

func deploymentPlanNodeToModel(value deploydomain.DeploymentPlanNode) deploymentPlanNodeModel {
	condition := value.SuccessCondition
	return deploymentPlanNodeModel{
		Key: value.Key, ApplicationKey: value.ApplicationKey, Order: value.Order,
		SuccessCondition: deploymentPlanConditionModel{
			SyncStatuses: condition.SyncStatuses, HealthStatuses: condition.HealthStatuses,
			StabilizationSeconds: condition.StabilizationSeconds, TimeoutSeconds: condition.TimeoutSeconds,
		},
	}
}

func deploymentPlanNodeFromModel(value deploymentPlanNodeModel) deploydomain.DeploymentPlanNode {
	condition := value.SuccessCondition
	return deploydomain.DeploymentPlanNode{
		Key: value.Key, ApplicationKey: value.ApplicationKey, Order: value.Order,
		SuccessCondition: deploydomain.DeploymentPlanCondition{
			SyncStatuses: condition.SyncStatuses, HealthStatuses: condition.HealthStatuses,
			StabilizationSeconds: condition.StabilizationSeconds, TimeoutSeconds: condition.TimeoutSeconds,
		},
	}
}

func deploymentPlanEdgeToModel(value deploydomain.DeploymentPlanEdge) deploymentPlanEdgeModel {
	return deploymentPlanEdgeModel{From: value.From, To: value.To, Condition: string(value.Condition)}
}

func deploymentPlanEdgeFromModel(value deploymentPlanEdgeModel) deploydomain.DeploymentPlanEdge {
	return deploydomain.DeploymentPlanEdge{
		From: value.From, To: value.To, Condition: deploydomain.DeploymentPlanEdgeCondition(value.Condition),
	}
}
