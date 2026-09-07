package infrastructure

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

type runtimeLockChange struct {
	versionID uuid.UUID
	expected  uint64
	next      uint64
}

func updateRequestVersionLock(ctx context.Context, tx *gorm.DB, change runtimeLockChange) error {
	result := tx.WithContext(ctx).Table("deployment_request_versions").
		Where("id = ? AND lock_version = ? AND status NOT IN ?", change.versionID, change.expected, []string{"Superseded", "Terminated"}).
		Update("lock_version", change.next)
	if result.Error != nil {
		return runtimeWriteError("advance deployment request version", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}

func advanceReviewRuntimeLock(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	result := tx.WithContext(ctx).Model(&workflowInstanceModel{}).
		Where("request_version_id = ? AND lock_version = ?", change.Task.RequestVersionID, change.ExpectedLock).
		Updates(map[string]any{"lock_version": change.ExpectedLock + 1, "updated_at": change.Mutation.OccurredAt.UTC()})
	if result.Error != nil {
		return runtimeWriteError("advance workflow review version", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}

func advanceReviewReassignmentRuntimeLock(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	result := tx.WithContext(ctx).Model(&workflowInstanceModel{}).
		Where("request_version_id = ? AND lock_version = ?", change.Task.RequestVersionID, change.ExpectedLock).
		Updates(map[string]any{"lock_version": change.ExpectedLock + 1, "updated_at": change.Mutation.OccurredAt.UTC()})
	if result.Error != nil {
		return runtimeWriteError("advance workflow review reassignment version", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}
