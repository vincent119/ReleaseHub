package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
)

type confirmedActualStates struct {
	executionID uuid.UUID
	states      []deployapp.ActualState
	now         time.Time
}

// Terminate records an explicit stop while retaining Application locks.
func (r *ExecutionRepository) Terminate(ctx context.Context, change deployapp.ExecutionTerminateChange) (deployapp.DeploymentExecution, error) {
	var result deployapp.DeploymentExecution
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		execution, err := lockExecution(ctx, tx, change.ExecutionID)
		if err != nil {
			return err
		}
		if execution.LockVersion != change.ExpectedVersion || (execution.Status != "Running" && execution.Status != "PartialFailed") {
			return deployapp.ErrExecutionConflict
		}
		if err := persistTerminatedExecution(ctx, tx, execution, change); err != nil {
			return err
		}
		result, err = loadExecutionView(ctx, tx, execution.ID)
		return err
	})
	return result, executionCommandWriteError(err)
}

func persistTerminatedExecution(ctx context.Context, tx *gorm.DB, execution deploymentExecutionModel, change deployapp.ExecutionTerminateChange) error {
	now := change.Mutation.OccurredAt.UTC()
	if err := persistTerminateWorkflow(ctx, tx, change); err != nil {
		return err
	}
	updates := map[string]any{"status": "Terminated", "completed_at": now,
		"updated_at": now, "lock_version": gorm.Expr("lock_version + 1")}
	if err := tx.WithContext(ctx).Model(&deploymentExecutionModel{}).Where("id = ?", execution.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("terminate deployment execution: %w", err)
	}
	if err := terminateExecutionNodes(ctx, tx, execution.ID, now); err != nil {
		return err
	}
	if err := terminateRequestVersion(ctx, tx, execution.RequestVersionID, now); err != nil {
		return err
	}
	return recordTerminateCommand(ctx, tx, execution, change)
}

func persistTerminateWorkflow(ctx context.Context, tx *gorm.DB, change deployapp.ExecutionTerminateChange) error {
	return persistExecutionCommandWorkflow(executionWorkflowCommand{
		ctx: ctx, tx: tx, mutation: change.Mutation, result: change.WorkflowResult,
	})
}

func terminateExecutionNodes(ctx context.Context, tx *gorm.DB, executionID uuid.UUID, now time.Time) error {
	return tx.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).
		Where("execution_id = ? AND status NOT IN ?", executionID, []string{"Succeeded", "Failed", "Blocked", "Skipped"}).
		Updates(map[string]any{"status": "Terminated", "completed_at": now, "updated_at": now}).Error
}

func terminateRequestVersion(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) error {
	result := tx.WithContext(ctx).Model(&deploymentRequestVersionModel{}).Where("id = ?", versionID).
		Updates(map[string]any{"status": "Terminated", "lock_version": gorm.Expr("lock_version + 1"), "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		return fmt.Errorf("terminate deployment Request Version: %w", executionMutationError(result))
	}
	return nil
}

func recordTerminateCommand(ctx context.Context, tx *gorm.DB, execution deploymentExecutionModel, change deployapp.ExecutionTerminateChange) error {
	command := executionCommandModel{
		ID: uuid.New(), RequestVersionID: execution.RequestVersionID, ExecutionID: execution.ID,
		CommandType: "Terminate", ActorID: change.Mutation.ActorID,
		IdempotencyKey: change.Mutation.IdempotencyKey, RequestHash: change.Mutation.RequestHash,
		Reason: change.Reason, ActualStates: datatypes.JSON("[]"), OccurredAt: change.Mutation.OccurredAt,
	}
	if err := tx.WithContext(ctx).Create(&command).Error; err != nil {
		return fmt.Errorf("record deployment terminate command: %w", err)
	}
	evidence := commandEvidence{command: command, requestID: change.Mutation.RequestID,
		metadata: map[string]any{"reason": change.Reason}}
	return appendExecutionCommandAudit(ctx, tx, evidence)
}

// Unlock saves fresh actual-state evidence and releases every lock owned by the terminated execution.
func (r *ExecutionRepository) Unlock(ctx context.Context, change deployapp.ExecutionUnlockChange) (deployapp.DeploymentExecution, error) {
	var result deployapp.DeploymentExecution
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		execution, err := lockExecution(ctx, tx, change.ExecutionID)
		if err != nil {
			return err
		}
		if execution.LockVersion != change.ExpectedVersion || execution.Status != "Terminated" {
			return deployapp.ErrExecutionConflict
		}
		if err := persistUnlock(ctx, tx, execution, change); err != nil {
			return err
		}
		result, err = loadExecutionView(ctx, tx, execution.ID)
		return err
	})
	return result, executionCommandWriteError(err)
}

func persistUnlock(ctx context.Context, tx *gorm.DB, execution deploymentExecutionModel, change deployapp.ExecutionUnlockChange) error {
	confirmed := confirmedActualStates{executionID: execution.ID, states: change.ActualStates, now: change.Mutation.OccurredAt}
	if err := saveConfirmedActualStates(ctx, tx, confirmed); err != nil {
		return err
	}
	result := tx.WithContext(ctx).Where("execution_id = ?", execution.ID).Delete(&applicationLockModel{})
	if result.Error != nil || result.RowsAffected != int64(len(change.ActualStates)) {
		return fmt.Errorf("release confirmed Application locks: %w", executionMutationError(result))
	}
	if err := tx.WithContext(ctx).Model(&deploymentExecutionModel{}).Where("id = ?", execution.ID).
		Updates(map[string]any{"lock_version": gorm.Expr("lock_version + 1"), "updated_at": change.Mutation.OccurredAt.UTC()}).Error; err != nil {
		return fmt.Errorf("fence unlocked deployment execution: %w", err)
	}
	return recordUnlockCommand(ctx, tx, execution, change)
}

func saveConfirmedActualStates(ctx context.Context, tx *gorm.DB, confirmed confirmedActualStates) error {
	for _, state := range confirmed.states {
		images := state.Images
		if images == nil {
			images = []deploydomain.DeploymentRequestImageSnapshot{}
		}
		encoded, err := json.Marshal(images)
		if err != nil {
			return fmt.Errorf("marshal confirmed actual images: %w", err)
		}
		result := tx.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).
			Where("execution_id = ? AND application_id = ?", confirmed.executionID, state.ApplicationID).
			Updates(map[string]any{"actual_revision": state.Revision, "actual_images": encoded, "updated_at": confirmed.now.UTC()})
		if result.Error != nil || result.RowsAffected != 1 {
			return fmt.Errorf("save confirmed actual state: %w", executionMutationError(result))
		}
	}
	return nil
}

func recordUnlockCommand(ctx context.Context, tx *gorm.DB, execution deploymentExecutionModel, change deployapp.ExecutionUnlockChange) error {
	states, err := json.Marshal(change.ActualStates)
	if err != nil {
		return fmt.Errorf("marshal unlock actual states: %w", err)
	}
	command := executionCommandModel{
		ID: uuid.New(), RequestVersionID: execution.RequestVersionID, ExecutionID: execution.ID,
		CommandType: "Unlock", ActorID: change.Mutation.ActorID,
		IdempotencyKey: change.Mutation.IdempotencyKey, RequestHash: change.Mutation.RequestHash,
		Reason: change.Reason, ActualStates: states, OccurredAt: change.Mutation.OccurredAt,
	}
	if err := tx.WithContext(ctx).Create(&command).Error; err != nil {
		return fmt.Errorf("record deployment unlock command: %w", err)
	}
	evidence := commandEvidence{command: command, requestID: change.Mutation.RequestID,
		metadata: map[string]any{"reason": change.Reason, "actualStates": change.ActualStates}}
	return appendExecutionCommandAudit(ctx, tx, evidence)
}
