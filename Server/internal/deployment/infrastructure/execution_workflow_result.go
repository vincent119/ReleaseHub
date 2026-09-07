package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func advanceExecutionWorkflowResult(value executionResultContext) error {
	instance, document, err := loadExecutionWorkflow(value.ctx, value.tx, value.execution.RequestVersionID)
	if err != nil {
		return err
	}
	engine, err := deploydomain.NewWorkflowEngine(document)
	if err != nil {
		return blockExecutionWorkflow(value, instance, err)
	}
	result, err := engine.ApplyDeploymentResult(workflowInstanceFromModel(instance), value.status, value.now)
	if err != nil || result.ReviewPolicy != nil || result.DeploymentIntent {
		return blockExecutionWorkflow(value, instance, errors.New("deployment result workflow path is not executable"))
	}
	return persistExecutionWorkflowTransition(value, instance, result)
}

func loadExecutionWorkflow(ctx context.Context, tx *gorm.DB, versionID uuid.UUID) (workflowInstanceModel, deploydomain.WorkflowDocument, error) {
	var instance workflowInstanceModel
	if err := tx.WithContext(ctx).Raw("SELECT * FROM deployment_workflow_instances WHERE request_version_id = ? FOR UPDATE", versionID).Scan(&instance).Error; err != nil || instance.ID == uuid.Nil {
		return instance, deploydomain.WorkflowDocument{}, fmt.Errorf("load execution workflow instance: %w", executionLookupError(err))
	}
	var version releaseWorkflowVersionModel
	if err := tx.WithContext(ctx).First(&version, "id = ?", instance.WorkflowVersionID).Error; err != nil {
		return instance, deploydomain.WorkflowDocument{}, fmt.Errorf("load execution workflow document: %w", err)
	}
	document, err := unmarshalWorkflowDocument(version.Document)
	return instance, document, err
}

func workflowInstanceFromModel(model workflowInstanceModel) deploydomain.WorkflowInstance {
	return deploydomain.WorkflowInstance{
		ID: model.ID, RequestVersionID: model.RequestVersionID, WorkflowVersionID: model.WorkflowVersionID,
		CurrentStateKey: model.CurrentStateKey, Status: deploydomain.WorkflowInstanceStatus(model.Status),
		LockVersion: model.LockVersion, StartedAt: model.StartedAt, CompletedAt: model.CompletedAt,
	}
}

func persistExecutionWorkflowTransition(value executionResultContext, instance workflowInstanceModel, result deploydomain.WorkflowResult) error {
	change := deployapp.WorkflowTransitionChange{
		Mutation:     deployapp.WorkflowRuntimeMutation{IdempotencyKey: "deployment-result:" + value.execution.ID.String(), OccurredAt: value.now},
		ExpectedLock: instance.LockVersion, Result: result,
	}
	if err := updateRuntimeInstance(value.ctx, value.tx, change); err != nil {
		return err
	}
	if err := insertRuntimeTransition(value.ctx, value.tx, change); err != nil {
		return err
	}
	return appendRuntimeEffects(value.ctx, value.tx, runtimeEffects{mutation: change.Mutation, result: result})
}

func blockExecutionWorkflow(value executionResultContext, instance workflowInstanceModel, cause error) error {
	updates := map[string]any{"status": string(deploydomain.WorkflowInstanceBlocked),
		"lock_version": gorm.Expr("lock_version + 1"), "updated_at": value.now}
	if err := value.tx.WithContext(value.ctx).Model(&workflowInstanceModel{}).Where("id = ?", instance.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("block execution workflow result: %w", err)
	}
	return appendBlockedWorkflowEvidence(value, cause)
}

func appendBlockedWorkflowEvidence(value executionResultContext, cause error) error {
	metadata := map[string]any{"executionId": value.execution.ID, "status": value.status, "reason": cause.Error()}
	if err := database.AppendAudit(value.ctx, value.tx, database.AuditRecord{
		OccurredAt: value.now, Action: "deployment_workflow.result_blocked", ResourceType: "deployment_request_version",
		ResourceID: value.execution.RequestVersionID.String(), Metadata: metadata,
	}); err != nil {
		return err
	}
	payload, _ := json.Marshal(metadata)
	event, err := platform.NewEvent("deployment.workflow.result_blocked", "deployment_request_version",
		value.execution.RequestVersionID.String(), payload, value.now)
	if err != nil {
		return err
	}
	return database.AppendOutbox(value.ctx, value.tx, event)
}
