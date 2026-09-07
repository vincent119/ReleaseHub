package infrastructure

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type reviewAssignmentModel struct {
	UserIDs     []uuid.UUID               `json:"userIds"`
	RoleMembers map[uuid.UUID][]uuid.UUID `json:"roleMembers"`
}

func workflowRuntimeSnapshot(model workflowRuntimeSourceModel) (deployapp.WorkflowRuntimeSnapshot, error) {
	document, err := unmarshalWorkflowDocument(model.Document)
	if err != nil {
		return deployapp.WorkflowRuntimeSnapshot{}, err
	}
	scope, err := authz.NewEnvironmentScope(model.OrganizationID, model.ProjectID, model.EnvironmentID)
	if err != nil {
		return deployapp.WorkflowRuntimeSnapshot{}, fmt.Errorf("create workflow runtime scope: %w", err)
	}
	return deployapp.WorkflowRuntimeSnapshot{
		RequestID: model.RequestID, RequestVersionID: model.RequestVersionID,
		RequestCreatorID: optionalUUID(model.RequestCreatorID), Scope: scope,
		Version: runtimeWorkflowVersion(model, document), Instance: runtimeWorkflowInstance(model),
		NextReviewStage: 1,
	}, nil
}

func runtimeWorkflowVersion(model workflowRuntimeSourceModel, document deploydomain.WorkflowDocument) deploydomain.ReleaseWorkflowVersion {
	return deploydomain.ReleaseWorkflowVersion{
		ID: model.WorkflowVersionID, WorkflowID: model.WorkflowID, VersionNumber: model.VersionNumber,
		Lifecycle: deploydomain.DefinitionLifecycle(model.Lifecycle), Document: document,
		LockVersion: model.VersionLock, CreatedBy: model.VersionCreatedBy,
		PublishedAt: model.PublishedAt, DisabledAt: model.DisabledAt,
		CreatedAt: model.VersionCreatedAt, UpdatedAt: model.VersionUpdatedAt,
	}
}

func runtimeWorkflowInstance(model workflowRuntimeSourceModel) *deploydomain.WorkflowInstance {
	if model.InstanceID == nil {
		return nil
	}
	return &deploydomain.WorkflowInstance{
		ID: *model.InstanceID, RequestVersionID: model.RequestVersionID,
		WorkflowVersionID: model.WorkflowVersionID, CurrentStateKey: optionalStringValue(model.CurrentStateKey),
		Status:      deploydomain.WorkflowInstanceStatus(optionalStringValue(model.InstanceStatus)),
		LockVersion: optionalUint64(model.InstanceLock), StartedAt: optionalTime(model.StartedAt),
		CompletedAt: model.CompletedAt,
	}
}

func workflowInstanceToModel(value deploydomain.WorkflowInstance, updatedAt time.Time) workflowInstanceModel {
	return workflowInstanceModel{
		ID: value.ID, RequestVersionID: value.RequestVersionID, WorkflowVersionID: value.WorkflowVersionID,
		CurrentStateKey: value.CurrentStateKey, Status: string(value.Status), LockVersion: value.LockVersion,
		StartedAt: value.StartedAt, CompletedAt: value.CompletedAt, UpdatedAt: updatedAt.UTC(),
	}
}

func reviewTaskToModel(instanceID uuid.UUID, value deploydomain.ReviewTask, now time.Time) (reviewTaskModel, error) {
	assignment, err := marshalReviewAssignment(value.Assignment)
	if err != nil {
		return reviewTaskModel{}, err
	}
	return reviewTaskModel{
		ID: value.ID, WorkflowInstanceID: instanceID, RequestVersionID: value.RequestVersionID,
		StateKey: value.StateKey, StageNumber: value.StageNumber, PolicyType: string(value.Policy.Type),
		RequiredApprovals: value.Policy.RequiredApprovals, AllowSelfReview: value.Policy.AllowSelfReview,
		AssigneeSnapshot: assignment, Status: string(value.Status), CreatedAt: now.UTC(),
		ClosedAt: reviewClosedAt(value.Status, now),
	}, nil
}

func reviewTaskFromModel(model reviewTaskModel, creatorID uuid.UUID, decisions []deploydomain.ReviewDecision) (deploydomain.ReviewTask, error) {
	assignment, err := unmarshalReviewAssignment(model.AssigneeSnapshot)
	if err != nil {
		return deploydomain.ReviewTask{}, err
	}
	policy, err := deploydomain.NewReviewPolicy(deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyType(model.PolicyType), RequiredApprovals: model.RequiredApprovals,
		AllowSelfReview: model.AllowSelfReview, UserIDs: assignment.UserIDs,
		RoleIDs: roleIDs(assignment.RoleMembers),
	})
	if err != nil {
		return deploydomain.ReviewTask{}, err
	}
	return deploydomain.ReviewTask{
		ID: model.ID, RequestVersionID: model.RequestVersionID, StateKey: model.StateKey,
		StageNumber: model.StageNumber, RequestCreatorID: creatorID, Policy: policy,
		Assignment: assignment, Decisions: decisions, Status: deploydomain.ReviewTaskStatus(model.Status),
	}, nil
}

func marshalReviewAssignment(value deploydomain.ReviewAssignment) (datatypes.JSON, error) {
	encoded, err := json.Marshal(reviewAssignmentModel{UserIDs: value.UserIDs, RoleMembers: value.RoleMembers})
	if err != nil {
		return nil, fmt.Errorf("marshal workflow review assignment: %w", err)
	}
	return encoded, nil
}

func unmarshalReviewAssignment(value datatypes.JSON) (deploydomain.ReviewAssignment, error) {
	var model reviewAssignmentModel
	if err := json.Unmarshal(value, &model); err != nil {
		return deploydomain.ReviewAssignment{}, fmt.Errorf("unmarshal workflow review assignment: %w", err)
	}
	return deploydomain.ReviewAssignment{UserIDs: model.UserIDs, RoleMembers: model.RoleMembers}, nil
}

func roleIDs(values map[uuid.UUID][]uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(values))
	for roleID := range values {
		result = append(result, roleID)
	}
	return result
}

func reviewClosedAt(status deploydomain.ReviewTaskStatus, now time.Time) *time.Time {
	if status == deploydomain.ReviewTaskPending || status == deploydomain.ReviewTaskReassignmentRequired {
		return nil
	}
	value := now.UTC()
	return &value
}

func optionalUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalUint64(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func optionalTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
