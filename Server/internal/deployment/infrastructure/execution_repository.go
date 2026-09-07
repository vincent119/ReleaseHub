package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
)

// ExecutionRepository persists immutable execution inputs and node observations.
type ExecutionRepository struct{ db *gorm.DB }

var _ deployapp.ExecutionRepository = (*ExecutionRepository)(nil)

// NewExecutionRepository creates the PostgreSQL execution repository.
func NewExecutionRepository(db *gorm.DB) (*ExecutionRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &ExecutionRepository{db: db}, nil
}

// Prepare creates or reloads the first workflow execution attempt atomically.
func (r *ExecutionRepository) Prepare(ctx context.Context, versionID uuid.UUID, now time.Time) (deployapp.ExecutionSnapshot, error) {
	var snapshot deployapp.ExecutionSnapshot
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := lockExecutionVersion(ctx, tx, versionID); err != nil {
			return err
		}
		var err error
		snapshot, err = prepareExecution(ctx, tx, versionID, now.UTC())
		return err
	})
	return snapshot, err
}

// Load reads an already-created business retry attempt.
func (r *ExecutionRepository) Load(ctx context.Context, executionID uuid.UUID) (deployapp.ExecutionSnapshot, error) {
	var model deploymentExecutionModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", executionID).Error; err != nil {
		return deployapp.ExecutionSnapshot{}, fmt.Errorf("load deployment execution: %w", err)
	}
	return loadExecutionSnapshot(ctx, r.db, model)
}

// MarkRunning advances a preflight-complete execution.
func (r *ExecutionRepository) MarkRunning(ctx context.Context, executionID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&deploymentExecutionModel{}).
		Where("id = ? AND status IN ?", executionID, []string{"Queued", "Preflight"}).
		Updates(map[string]any{"status": "Running", "started_at": now.UTC(), "updated_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("mark deployment execution running: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("deployment execution cannot start")
	}
	return nil
}

// UpdateNode persists one fenced executor observation.
func (r *ExecutionRepository) UpdateNode(ctx context.Context, value deployapp.ExecutionNodeUpdate) error {
	changes, err := executionNodeChanges(value)
	if err != nil {
		return err
	}
	return r.persistNodeUpdate(ctx, value, changes)
}

func executionNodeChanges(value deployapp.ExecutionNodeUpdate) (map[string]any, error) {
	actualImages := value.ActualImages
	if actualImages == nil {
		actualImages = []deploydomain.DeploymentRequestImageSnapshot{}
	}
	images, err := json.Marshal(actualImages)
	if err != nil {
		return nil, fmt.Errorf("marshal deployment execution node images: %w", err)
	}
	changes := map[string]any{
		"status": value.Status, "operation_id": value.OperationID,
		"sync_status": value.SyncStatus, "health_status": value.HealthStatus,
		"actual_revision": value.ActualRevision, "error_code": value.ErrorCode,
		"actual_images": images, "error_message": value.ErrorMessage, "updated_at": value.UpdatedAt.UTC(),
	}
	applyExecutionNodeTimes(changes, value)
	return changes, nil
}

func (r *ExecutionRepository) persistNodeUpdate(ctx context.Context, value deployapp.ExecutionNodeUpdate, changes map[string]any) error {
	result := r.db.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).
		Where("execution_id = ? AND application_id = ? AND EXISTS (SELECT 1 FROM deployment_executions execution WHERE execution.id = deployment_execution_nodes.execution_id AND execution.status <> 'Terminated')", value.ExecutionID, value.ApplicationID).
		Updates(changes)
	if result.Error != nil {
		return fmt.Errorf("update deployment execution node: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return r.executionNodeMutationError(ctx, value.ExecutionID)
	}
	return nil
}

func (r *ExecutionRepository) executionNodeMutationError(ctx context.Context, executionID uuid.UUID) error {
	var status string
	if err := r.db.WithContext(ctx).Model(&deploymentExecutionModel{}).Where("id = ?", executionID).Pluck("status", &status).Error; err != nil {
		return fmt.Errorf("load deployment execution status: %w", err)
	}
	if status == "Terminated" {
		return deployapp.ErrExecutionStopped
	}
	return errors.New("deployment execution node was not found")
}

func applyExecutionNodeTimes(changes map[string]any, value deployapp.ExecutionNodeUpdate) {
	if value.Status == "Syncing" {
		changes["started_at"] = value.UpdatedAt.UTC()
	}
	if value.Status == "Succeeded" || value.Status == "Failed" || value.Status == "Blocked" {
		changes["completed_at"] = value.UpdatedAt.UTC()
	}
}

// Block records preflight drift and supersedes the reviewed Request Version.
func (r *ExecutionRepository) Block(ctx context.Context, block deployapp.ExecutionBlock) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := blockExecution(ctx, tx, block.ExecutionID, block.OccurredAt); err != nil {
			return err
		}
		if err := blockExecutionNode(ctx, tx, block); err != nil {
			return err
		}
		if err := supersedeExecutionRequest(ctx, tx, block.ExecutionID, block.OccurredAt); err != nil {
			return err
		}
		return appendExecutionBlockedEvidence(ctx, tx, block)
	})
}

func blockExecution(ctx context.Context, tx *gorm.DB, executionID uuid.UUID, now time.Time) error {
	return tx.WithContext(ctx).Model(&deploymentExecutionModel{}).Where("id = ?", executionID).
		Updates(map[string]any{"status": "Blocked", "completed_at": now.UTC(), "updated_at": now.UTC()}).Error
}

func blockExecutionNode(ctx context.Context, tx *gorm.DB, block deployapp.ExecutionBlock) error {
	return tx.WithContext(ctx).Model(&deploymentExecutionNodeModel{}).
		Where("execution_id = ? AND request_application_id = ?", block.ExecutionID, block.RequestApplicationID).
		Updates(map[string]any{"status": "Blocked", "error_code": block.Code,
			"error_message": "reviewed deployment content changed", "completed_at": block.OccurredAt.UTC(), "updated_at": block.OccurredAt.UTC()}).Error
}

func supersedeExecutionRequest(ctx context.Context, tx *gorm.DB, executionID uuid.UUID, now time.Time) error {
	return tx.WithContext(ctx).Exec(`UPDATE deployment_request_versions version
SET status = 'Superseded', lock_version = version.lock_version + 1, updated_at = ?
FROM deployment_executions execution
WHERE execution.id = ? AND version.id = execution.request_version_id`, now.UTC(), executionID).Error
}
