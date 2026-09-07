package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

type retryPersistence struct {
	ctx      context.Context
	tx       *gorm.DB
	previous deploymentExecutionModel
	retry    deploymentExecutionModel
	change   deployapp.ExecutionRetryChange
}

type commandEvidence struct {
	command   executionCommandModel
	requestID string
	metadata  map[string]any
}

// Retry atomically creates a distinct failed-only business attempt and durable job.
func (r *ExecutionRepository) Retry(ctx context.Context, change deployapp.ExecutionRetryChange) (deployapp.DeploymentExecution, error) {
	var result deployapp.DeploymentExecution
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var err error
		result, err = createRetryExecution(ctx, tx, change)
		return err
	})
	return result, executionCommandWriteError(err)
}

func createRetryExecution(ctx context.Context, tx *gorm.DB, change deployapp.ExecutionRetryChange) (deployapp.DeploymentExecution, error) {
	previous, err := lockExecution(ctx, tx, change.ExecutionID)
	if err != nil {
		return deployapp.DeploymentExecution{}, err
	}
	if previous.LockVersion != change.ExpectedVersion {
		return deployapp.DeploymentExecution{}, deployapp.ErrExecutionConflict
	}
	model := retryExecutionModel(previous, change)
	persistence := retryPersistence{ctx: ctx, tx: tx, previous: previous, retry: model, change: change}
	if err := persistRetryExecution(persistence); err != nil {
		return deployapp.DeploymentExecution{}, err
	}
	return loadExecutionView(ctx, tx, model.ID)
}

func persistRetryExecution(value retryPersistence) error {
	if err := value.tx.WithContext(value.ctx).Create(&value.retry).Error; err != nil {
		return fmt.Errorf("create retry execution: %w", err)
	}
	if err := cloneRetryNodes(value); err != nil {
		return err
	}
	return persistRetryEffects(value)
}

func retryExecutionModel(previous deploymentExecutionModel, change deployapp.ExecutionRetryChange) deploymentExecutionModel {
	now := change.Mutation.OccurredAt.UTC()
	actor := change.Mutation.ActorID
	return deploymentExecutionModel{
		ID: uuid.New(), RequestVersionID: previous.RequestVersionID, PlanVersionID: previous.PlanVersionID,
		Attempt: previous.Attempt + 1, Status: "Preflight", TriggerKind: "Retry", TriggeredBy: &actor,
		PlanSnapshot: previous.PlanSnapshot, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func cloneRetryNodes(value retryPersistence) error {
	var previous []deploymentExecutionNodeModel
	if err := value.tx.WithContext(value.ctx).Where("execution_id = ?", value.previous.ID).Find(&previous).Error; err != nil {
		return fmt.Errorf("load failed deployment nodes: %w", err)
	}
	selected := uuidSet(value.change.ApplicationIDs)
	for _, node := range previous {
		model, err := retryNodeModel(node, value.retry.ID, selected, value.change)
		if err != nil {
			return err
		}
		if err := value.tx.WithContext(value.ctx).Create(&model).Error; err != nil {
			return fmt.Errorf("create retry deployment node: %w", err)
		}
	}
	return nil
}

func retryNodeModel(previous deploymentExecutionNodeModel, retryID uuid.UUID, selected map[uuid.UUID]struct{}, change deployapp.ExecutionRetryChange) (deploymentExecutionNodeModel, error) {
	status := "Skipped"
	if previous.Status == "Succeeded" {
		status = "Succeeded"
	} else if _, exists := selected[previous.ApplicationID]; exists && previous.Status == "Failed" {
		status = "Waiting"
	} else if previous.Status != "Failed" && previous.Status != "Skipped" {
		return deploymentExecutionNodeModel{}, deployapp.ErrExecutionConflict
	}
	previous.ID, previous.ExecutionID, previous.Status = uuid.New(), retryID, status
	previous.UpdatedAt = change.Mutation.OccurredAt.UTC()
	if status == "Waiting" {
		clearRetryNodeRuntime(&previous)
	}
	return previous, nil
}

func clearRetryNodeRuntime(node *deploymentExecutionNodeModel) {
	node.OperationID, node.SyncStatus, node.HealthStatus, node.ActualRevision = "", "", "", ""
	node.ActualImages, node.ErrorCode, node.ErrorMessage = datatypes.JSON("[]"), "", ""
	node.StartedAt, node.CompletedAt = nil, nil
}

func uuidSet(values []uuid.UUID) map[uuid.UUID]struct{} {
	result := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func persistRetryEffects(value retryPersistence) error {
	now := value.change.Mutation.OccurredAt.UTC()
	if err := persistRetryWorkflow(value); err != nil {
		return err
	}
	if err := value.tx.WithContext(value.ctx).Model(&deploymentExecutionModel{}).Where("id = ?", value.previous.ID).
		Update("lock_version", gorm.Expr("lock_version + 1")).Error; err != nil {
		return fmt.Errorf("fence retried deployment execution: %w", err)
	}
	if err := markRetryRequestDeploying(value.ctx, value.tx, value.retry.RequestVersionID, now); err != nil {
		return err
	}
	if err := handoffRetryLocks(value, now); err != nil {
		return err
	}
	if err := insertRetryJob(value.ctx, value.tx, value.retry, value.change); err != nil {
		return err
	}
	return appendRetryEvidence(value)
}

func persistRetryWorkflow(value retryPersistence) error {
	return persistExecutionCommandWorkflow(executionWorkflowCommand{
		ctx: value.ctx, tx: value.tx,
		mutation: value.change.Mutation, result: value.change.WorkflowResult,
	})
}

func handoffRetryLocks(value retryPersistence, now time.Time) error {
	result := value.tx.WithContext(value.ctx).Model(&applicationLockModel{}).
		Where("execution_id = ?", value.previous.ID).Updates(map[string]any{
		"execution_id": value.retry.ID, "owner_token": uuid.New(),
		"fencing_token": gorm.Expr("fencing_token + 1"), "acquired_at": now, "heartbeat_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("handoff retry Application locks: %w", result.Error)
	}
	return nil
}

func markRetryRequestDeploying(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) error {
	result := tx.WithContext(ctx).Model(&deploymentRequestVersionModel{}).
		Where("id = ? AND status IN ?", versionID, []string{"Failed", "PartialFailed"}).
		Updates(map[string]any{"status": "Deploying", "lock_version": gorm.Expr("lock_version + 1"), "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		return fmt.Errorf("mark retry Request Version deploying: %w", executionMutationError(result))
	}
	return nil
}

func insertRetryJob(ctx context.Context, tx *gorm.DB, retry deploymentExecutionModel, change deployapp.ExecutionRetryChange) error {
	payload, _ := json.Marshal(map[string]any{"requestVersionId": retry.RequestVersionID, "attempt": retry.Attempt})
	model := deploymentJobModel{
		ID: uuid.New(), JobType: "ExecuteDeployment", AggregateType: "deployment_execution",
		AggregateID: retry.ID, Payload: payload, Status: "Pending",
		IdempotencyKey: "business-retry:" + retry.RequestVersionID.String() + ":" + change.Mutation.IdempotencyKey,
		AvailableAt:    change.Mutation.OccurredAt, MaxAttempts: 10,
		CreatedAt: change.Mutation.OccurredAt, UpdatedAt: change.Mutation.OccurredAt,
	}
	return tx.WithContext(ctx).Create(&model).Error
}

func appendRetryEvidence(value retryPersistence) error {
	resultID := value.retry.ID
	command := executionCommandModel{
		ID: uuid.New(), RequestVersionID: value.retry.RequestVersionID, ExecutionID: value.previous.ID,
		ResultExecutionID: &resultID, CommandType: "Retry", ActorID: value.change.Mutation.ActorID,
		IdempotencyKey: value.change.Mutation.IdempotencyKey, RequestHash: value.change.Mutation.RequestHash,
		ActualStates: datatypes.JSON("[]"), OccurredAt: value.change.Mutation.OccurredAt,
	}
	if err := value.tx.WithContext(value.ctx).Create(&command).Error; err != nil {
		return fmt.Errorf("record deployment retry command: %w", err)
	}
	evidence := commandEvidence{command: command, requestID: value.change.Mutation.RequestID,
		metadata: map[string]any{"resultExecutionId": value.retry.ID, "applicationIds": value.change.ApplicationIDs}}
	return appendExecutionCommandAudit(value.ctx, value.tx, evidence)
}

func appendExecutionCommandAudit(ctx context.Context, tx *gorm.DB, evidence commandEvidence) error {
	evidence.metadata["command"] = evidence.command.CommandType
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: evidence.command.OccurredAt, ActorID: &evidence.command.ActorID, Action: "deployment_execution." + strings.ToLower(evidence.command.CommandType),
		ResourceType: "deployment_execution", ResourceID: evidence.command.ExecutionID.String(), RequestID: evidence.requestID, Metadata: evidence.metadata,
	}); err != nil {
		return err
	}
	payload, _ := json.Marshal(evidence.metadata)
	event, err := platform.NewEvent("deployment.execution."+strings.ToLower(evidence.command.CommandType), "deployment_execution", evidence.command.ExecutionID.String(), payload, evidence.command.OccurredAt)
	if err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}
