package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func createWorkflowInput(c *gin.Context) (deployapp.CreateWorkflowInput, error) {
	var body contract.CreateReleaseWorkflowRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		return deployapp.CreateWorkflowInput{}, err
	}
	document, err := workflowDocumentFromContract(body.Document)
	if err != nil {
		return deployapp.CreateWorkflowInput{}, err
	}
	return deployapp.CreateWorkflowInput{
		Name: body.Name, Description: optionalString(body.Description),
		Document: document, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func createWorkflowVersionInput(c *gin.Context, workflowID uuid.UUID) (deployapp.CreateWorkflowVersionInput, error) {
	var body contract.CreateReleaseWorkflowVersionRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.CreateWorkflowVersionInput{}, errors.New("workflow version request is invalid")
	}
	document, err := workflowDocumentFromContract(body.Document)
	if err != nil {
		return deployapp.CreateWorkflowVersionInput{}, err
	}
	return deployapp.CreateWorkflowVersionInput{
		WorkflowID: workflowID, ExpectedVersion: uint64(body.ExpectedVersion),
		Document: document, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func deleteWorkflowInput(c *gin.Context, workflowID uuid.UUID, expectedVersion contract.ExpectedWorkflowVersion) (deployapp.DeleteWorkflowInput, error) {
	if expectedVersion < 1 {
		return deployapp.DeleteWorkflowInput{}, errors.New("workflow deletion request is invalid")
	}
	return deployapp.DeleteWorkflowInput{
		WorkflowID: workflowID, ExpectedVersion: uint64(expectedVersion), RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func changeWorkflowLifecycleInput(c *gin.Context, workflowID, versionID uuid.UUID) (deployapp.ChangeWorkflowLifecycleInput, error) {
	var body contract.DefinitionLifecycleRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.ChangeWorkflowLifecycleInput{}, errors.New("workflow lifecycle request is invalid")
	}
	return deployapp.ChangeWorkflowLifecycleInput{
		WorkflowID: workflowID, VersionID: versionID,
		ExpectedVersion: uint64(body.ExpectedVersion),
		Lifecycle:       deploydomain.DefinitionLifecycle(body.Lifecycle),
		RequestID:       c.GetHeader(requestIDHeader),
	}, nil
}

func workflowDocumentFromContract(value contract.ReleaseWorkflowDocument) (deploydomain.WorkflowDocument, error) {
	states, err := workflowStatesFromContract(value.States)
	if err != nil {
		return deploydomain.WorkflowDocument{}, err
	}
	transitions, err := workflowTransitionsFromContract(value.Transitions)
	if err != nil {
		return deploydomain.WorkflowDocument{}, err
	}
	return deploydomain.WorkflowDocument{InitialState: value.InitialState, States: states, Transitions: transitions}, nil
}

func workflowStatesFromContract(values []contract.ReleaseWorkflowState) ([]deploydomain.WorkflowState, error) {
	result := make([]deploydomain.WorkflowState, 0, len(values))
	for _, value := range values {
		policy, err := reviewPolicyFromContract(value.ReviewPolicy)
		if err != nil {
			return nil, err
		}
		result = append(result, deploydomain.WorkflowState{
			Key: value.Key, Name: value.Name, Type: deploydomain.WorkflowStateType(value.Type), ReviewPolicy: policy,
		})
	}
	return result, nil
}

func reviewPolicyFromContract(value *contract.WorkflowReviewPolicy) (*deploydomain.ReviewPolicy, error) {
	if value == nil {
		return nil, nil
	}
	policy, err := deploydomain.NewReviewPolicy(deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyType(value.Type), RequiredApprovals: value.RequiredApprovals,
		AllowSelfReview: value.AllowSelfReview, UserIDs: value.UserIds, RoleIDs: value.RoleIds,
	})
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func workflowTransitionsFromContract(values []contract.ReleaseWorkflowTransition) ([]deploydomain.WorkflowTransition, error) {
	result := make([]deploydomain.WorkflowTransition, 0, len(values))
	for _, value := range values {
		conditions, err := workflowConditionsFromContract(value.Conditions)
		if err != nil {
			return nil, err
		}
		result = append(result, deploydomain.WorkflowTransition{
			Key: value.Key, From: value.From, To: value.To,
			Trigger: deploydomain.WorkflowTrigger(value.Trigger), Permission: value.Permission, Conditions: conditions,
		})
	}
	return result, nil
}

func workflowConditionsFromContract(values []contract.WorkflowCondition) ([]deploydomain.WorkflowCondition, error) {
	result := make([]deploydomain.WorkflowCondition, 0, len(values))
	for _, value := range values {
		conditionValues, err := workflowConditionValues(value.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, deploydomain.WorkflowCondition{
			Fact: deploydomain.WorkflowFact(value.Fact), Operator: deploydomain.WorkflowOperator(value.Operator), Values: conditionValues,
		})
	}
	return result, nil
}

func workflowConditionValues(value contract.WorkflowCondition_Value) ([]string, error) {
	single, singleErr := value.AsWorkflowConditionValue0()
	if singleErr == nil {
		return []string{single}, nil
	}
	multiple, multipleErr := value.AsWorkflowConditionValue1()
	if multipleErr != nil {
		return nil, errors.New("workflow condition value is invalid")
	}
	return multiple, nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
