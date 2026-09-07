package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

type executionResultContext struct {
	ctx       context.Context
	tx        *gorm.DB
	execution deploymentExecutionModel
	status    deploydomain.ExecutionStatus
	now       time.Time
}

// Complete atomically aggregates durable node results into the execution and Request Version.
func (r *ExecutionRepository) Complete(ctx context.Context, executionID uuid.UUID, now time.Time) (deploydomain.ExecutionStatus, error) {
	var status deploydomain.ExecutionStatus
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var err error
		status, err = completeExecution(ctx, tx, executionID, now.UTC())
		return err
	})
	return status, err
}

func completeExecution(ctx context.Context, tx *gorm.DB, executionID uuid.UUID, now time.Time) (deploydomain.ExecutionStatus, error) {
	execution, err := lockExecution(ctx, tx, executionID)
	if err != nil {
		return "", err
	}
	if status := terminalExecutionStatus(execution.Status); status != "" {
		return status, nil
	}
	statuses, err := finalizeExecutionNodes(ctx, tx, executionID, now)
	if err != nil {
		return "", err
	}
	status, err := deploydomain.AggregateExecutionStatus(statuses)
	if err != nil {
		return "", err
	}
	result := executionResultContext{ctx: ctx, tx: tx, execution: execution, status: status, now: now}
	return status, persistExecutionResult(result)
}

func lockExecution(ctx context.Context, tx *gorm.DB, executionID uuid.UUID) (deploymentExecutionModel, error) {
	var model deploymentExecutionModel
	err := tx.WithContext(ctx).Raw("SELECT * FROM deployment_executions WHERE id = ? FOR UPDATE", executionID).Scan(&model).Error
	if err != nil || model.ID == uuid.Nil {
		return model, fmt.Errorf("lock deployment execution: %w", executionLookupError(err))
	}
	return model, nil
}

func executionLookupError(err error) error {
	if err != nil {
		return err
	}
	return gorm.ErrRecordNotFound
}

func terminalExecutionStatus(value string) deploydomain.ExecutionStatus {
	switch deploydomain.ExecutionStatus(value) {
	case deploydomain.ExecutionSucceeded, deploydomain.ExecutionFailed, deploydomain.ExecutionPartialFailed, deploydomain.ExecutionTerminated:
		return deploydomain.ExecutionStatus(value)
	default:
		return ""
	}
}

func finalizeExecutionNodes(ctx context.Context, tx *gorm.DB, executionID uuid.UUID, now time.Time) ([]string, error) {
	if err := tx.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).
		Where("execution_id = ? AND status IN ?", executionID, []string{"Waiting", "Queued", "Preflight"}).
		Updates(map[string]any{"status": "Skipped", "completed_at": now, "updated_at": now}).Error; err != nil {
		return nil, fmt.Errorf("skip unresolved deployment nodes: %w", err)
	}
	var statuses []string
	err := tx.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).Where("execution_id = ?", executionID).Pluck("status", &statuses).Error
	return statuses, err
}

func persistExecutionResult(result executionResultContext) error {
	changes := map[string]any{"status": string(result.status), "completed_at": result.now, "updated_at": result.now, "lock_version": gorm.Expr("lock_version + 1")}
	if err := result.tx.WithContext(result.ctx).Model(&deploymentExecutionModel{}).Where("id = ?", result.execution.ID).Updates(changes).Error; err != nil {
		return fmt.Errorf("complete deployment execution: %w", err)
	}
	if err := updateExecutionRequestResult(result); err != nil {
		return err
	}
	if err := advanceExecutionWorkflowResult(result); err != nil {
		return err
	}
	return appendExecutionResultEvidence(result)
}

func updateExecutionRequestResult(value executionResultContext) error {
	mutation := value.tx.WithContext(value.ctx).Model(&deploymentRequestVersionModel{}).Where("id = ?", value.execution.RequestVersionID).
		Updates(map[string]any{"status": string(value.status), "updated_at": value.now, "lock_version": gorm.Expr("lock_version + 1")})
	if mutation.Error != nil || mutation.RowsAffected != 1 {
		return fmt.Errorf("complete deployment Request Version: %w", executionMutationError(mutation))
	}
	return nil
}

func executionMutationError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	return gorm.ErrRecordNotFound
}

func appendExecutionResultEvidence(result executionResultContext) error {
	payload, _ := json.Marshal(map[string]any{"executionId": result.execution.ID, "status": result.status})
	event, err := platform.NewEvent("deployment.execution.completed", "deployment_execution", result.execution.ID.String(), payload, result.now)
	if err != nil {
		return fmt.Errorf("create deployment execution event: %w", err)
	}
	if err := database.AppendAudit(result.ctx, result.tx, database.AuditRecord{OccurredAt: result.now, Action: "deployment_execution.complete", ResourceType: "deployment_execution", ResourceID: result.execution.ID.String(), Metadata: map[string]any{"status": result.status}}); err != nil {
		return err
	}
	return database.AppendOutbox(result.ctx, result.tx, event)
}
