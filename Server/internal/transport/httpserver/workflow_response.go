package httpserver

import (
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func workflowReviewOptionsResponse(value deployapp.WorkflowReviewOptions) contract.ReleaseWorkflowReviewOptions {
	users := make([]contract.ReleaseWorkflowReviewUserOption, 0, len(value.Users))
	for _, item := range value.Users {
		users = append(users, contract.ReleaseWorkflowReviewUserOption{
			Id: item.ID, Username: item.Username, Assignable: item.Assignable,
		})
	}
	roles := make([]contract.ReleaseWorkflowReviewRoleOption, 0, len(value.Roles))
	for _, item := range value.Roles {
		roles = append(roles, contract.ReleaseWorkflowReviewRoleOption{
			Id: item.ID, Name: item.Name, OwnerKind: contract.ReleaseWorkflowReviewRoleOptionOwnerKind(item.OwnerKind),
			OwnerId: item.OwnerID, Assignable: item.Assignable,
		})
	}
	return contract.ReleaseWorkflowReviewOptions{Users: users, Roles: roles}
}

func workflowResponses(values []deploydomain.ReleaseWorkflow) []contract.ReleaseWorkflow {
	result := make([]contract.ReleaseWorkflow, 0, len(values))
	for _, value := range values {
		result = append(result, workflowResponse(value))
	}
	return result
}

func workflowResponse(value deploydomain.ReleaseWorkflow) contract.ReleaseWorkflow {
	return contract.ReleaseWorkflow{
		Id: value.ID, Name: value.Name, Description: value.Description,
		Active: value.Active, Versions: workflowVersionResponses(value.Versions),
	}
}

func workflowVersionResponses(values []deploydomain.ReleaseWorkflowVersion) []contract.ReleaseWorkflowVersion {
	result := make([]contract.ReleaseWorkflowVersion, 0, len(values))
	for _, value := range values {
		result = append(result, workflowVersionResponse(value))
	}
	return result
}

func workflowVersionResponse(value deploydomain.ReleaseWorkflowVersion) contract.ReleaseWorkflowVersion {
	return contract.ReleaseWorkflowVersion{
		Id: value.ID, WorkflowId: value.WorkflowID, VersionNumber: int64(value.VersionNumber),
		Lifecycle: contract.DefinitionLifecycle(value.Lifecycle), Document: workflowDocumentResponse(value.Document),
		LockVersion: int64(value.LockVersion), CreatedAt: value.CreatedAt,
		PublishedAt: value.PublishedAt, DisabledAt: value.DisabledAt,
	}
}

func workflowDocumentResponse(value deploydomain.WorkflowDocument) contract.ReleaseWorkflowDocument {
	return contract.ReleaseWorkflowDocument{
		InitialState: value.InitialState, States: workflowStateResponses(value.States),
		Transitions: workflowTransitionResponses(value.Transitions),
	}
}

func workflowStateResponses(values []deploydomain.WorkflowState) []contract.ReleaseWorkflowState {
	result := make([]contract.ReleaseWorkflowState, 0, len(values))
	for _, value := range values {
		result = append(result, contract.ReleaseWorkflowState{
			Key: value.Key, Name: value.Name, Type: contract.WorkflowStateType(value.Type),
			ReviewPolicy: reviewPolicyResponse(value.ReviewPolicy),
		})
	}
	return result
}

func reviewPolicyResponse(value *deploydomain.ReviewPolicy) *contract.WorkflowReviewPolicy {
	if value == nil {
		return nil
	}
	return &contract.WorkflowReviewPolicy{
		Type: contract.WorkflowReviewPolicyType(value.Type), RequiredApprovals: value.RequiredApprovals,
		AllowSelfReview: value.AllowSelfReview, UserIds: value.UserIDs, RoleIds: value.RoleIDs,
	}
}

func workflowTransitionResponses(values []deploydomain.WorkflowTransition) []contract.ReleaseWorkflowTransition {
	result := make([]contract.ReleaseWorkflowTransition, 0, len(values))
	for _, value := range values {
		result = append(result, contract.ReleaseWorkflowTransition{
			Key: value.Key, From: value.From, To: value.To,
			Trigger:    contract.ReleaseWorkflowTransitionTrigger(value.Trigger),
			Permission: value.Permission, Conditions: workflowConditionResponses(value.Conditions),
		})
	}
	return result
}

func workflowConditionResponses(values []deploydomain.WorkflowCondition) []contract.WorkflowCondition {
	result := make([]contract.WorkflowCondition, 0, len(values))
	for _, value := range values {
		result = append(result, contract.WorkflowCondition{
			Fact: contract.WorkflowConditionFact(value.Fact), Operator: contract.WorkflowConditionOperator(value.Operator),
			Value: workflowConditionValue(value),
		})
	}
	return result
}

func workflowConditionValue(value deploydomain.WorkflowCondition) contract.WorkflowCondition_Value {
	var result contract.WorkflowCondition_Value
	if value.Operator == deploydomain.WorkflowOperatorIn || value.Operator == deploydomain.WorkflowOperatorNotIn {
		_ = result.FromWorkflowConditionValue1(value.Values)
		return result
	}
	_ = result.FromWorkflowConditionValue0(value.Values[0])
	return result
}
