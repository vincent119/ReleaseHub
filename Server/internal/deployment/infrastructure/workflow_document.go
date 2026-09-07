package infrastructure

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type workflowDocumentModel struct {
	InitialState string                    `json:"initialState"`
	States       []workflowStateModel      `json:"states"`
	Transitions  []workflowTransitionModel `json:"transitions"`
}

type workflowStateModel struct {
	Key          string             `json:"key"`
	Name         string             `json:"name"`
	Type         string             `json:"type"`
	ReviewPolicy *reviewPolicyModel `json:"reviewPolicy,omitempty"`
}

type reviewPolicyModel struct {
	Type              string      `json:"type"`
	RequiredApprovals int         `json:"requiredApprovals"`
	AllowSelfReview   bool        `json:"allowSelfReview"`
	UserIDs           []uuid.UUID `json:"userIds"`
	RoleIDs           []uuid.UUID `json:"roleIds"`
}

type workflowTransitionModel struct {
	Key        string                   `json:"key"`
	From       string                   `json:"from"`
	To         string                   `json:"to"`
	Trigger    string                   `json:"trigger"`
	Permission string                   `json:"permission"`
	Conditions []workflowConditionModel `json:"conditions"`
}

type workflowConditionModel struct {
	Fact     string   `json:"fact"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

func marshalWorkflowDocument(value deploydomain.WorkflowDocument) (datatypes.JSON, error) {
	model := workflowDocumentModel{InitialState: value.InitialState}
	for _, state := range value.States {
		model.States = append(model.States, workflowStateToModel(state))
	}
	for _, transition := range value.Transitions {
		model.Transitions = append(model.Transitions, workflowTransitionToModel(transition))
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow document: %w", err)
	}
	return encoded, nil
}

func unmarshalWorkflowDocument(value datatypes.JSON) (deploydomain.WorkflowDocument, error) {
	var model workflowDocumentModel
	if err := json.Unmarshal(value, &model); err != nil {
		return deploydomain.WorkflowDocument{}, fmt.Errorf("unmarshal workflow document: %w", err)
	}
	document := deploydomain.WorkflowDocument{InitialState: model.InitialState}
	for _, state := range model.States {
		document.States = append(document.States, workflowStateFromModel(state))
	}
	for _, transition := range model.Transitions {
		document.Transitions = append(document.Transitions, workflowTransitionFromModel(transition))
	}
	return deploydomain.NewWorkflowDocument(document)
}

func workflowStateToModel(value deploydomain.WorkflowState) workflowStateModel {
	model := workflowStateModel{Key: value.Key, Name: value.Name, Type: string(value.Type)}
	if value.ReviewPolicy != nil {
		model.ReviewPolicy = &reviewPolicyModel{
			Type: string(value.ReviewPolicy.Type), RequiredApprovals: value.ReviewPolicy.RequiredApprovals,
			AllowSelfReview: value.ReviewPolicy.AllowSelfReview,
			UserIDs:         value.ReviewPolicy.UserIDs, RoleIDs: value.ReviewPolicy.RoleIDs,
		}
	}
	return model
}

func workflowStateFromModel(value workflowStateModel) deploydomain.WorkflowState {
	state := deploydomain.WorkflowState{Key: value.Key, Name: value.Name, Type: deploydomain.WorkflowStateType(value.Type)}
	if value.ReviewPolicy != nil {
		state.ReviewPolicy = &deploydomain.ReviewPolicy{
			Type:              deploydomain.ReviewPolicyType(value.ReviewPolicy.Type),
			RequiredApprovals: value.ReviewPolicy.RequiredApprovals,
			AllowSelfReview:   value.ReviewPolicy.AllowSelfReview,
			UserIDs:           value.ReviewPolicy.UserIDs, RoleIDs: value.ReviewPolicy.RoleIDs,
		}
	}
	return state
}

func workflowTransitionToModel(value deploydomain.WorkflowTransition) workflowTransitionModel {
	model := workflowTransitionModel{
		Key: value.Key, From: value.From, To: value.To,
		Trigger: string(value.Trigger), Permission: value.Permission,
	}
	for _, condition := range value.Conditions {
		model.Conditions = append(model.Conditions, workflowConditionModel{
			Fact: string(condition.Fact), Operator: string(condition.Operator), Values: condition.Values,
		})
	}
	return model
}

func workflowTransitionFromModel(value workflowTransitionModel) deploydomain.WorkflowTransition {
	transition := deploydomain.WorkflowTransition{
		Key: value.Key, From: value.From, To: value.To,
		Trigger: deploydomain.WorkflowTrigger(value.Trigger), Permission: value.Permission,
	}
	for _, condition := range value.Conditions {
		transition.Conditions = append(transition.Conditions, deploydomain.WorkflowCondition{
			Fact:     deploydomain.WorkflowFact(condition.Fact),
			Operator: deploydomain.WorkflowOperator(condition.Operator), Values: condition.Values,
		})
	}
	return transition
}
