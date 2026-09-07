package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// ApplyReviewReassignment atomically replaces assignees and records audit evidence.
func (r *WorkflowRuntimeRepository) ApplyReviewReassignment(ctx context.Context, change deployapp.WorkflowReviewReassignmentChange) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		lock := runtimeLockChange{versionID: change.Task.RequestVersionID, expected: change.ExpectedLock, next: change.ExpectedLock + 1}
		if err := updateRequestVersionLock(ctx, tx, lock); err != nil {
			return err
		}
		if err := advanceReviewReassignmentRuntimeLock(ctx, tx, change); err != nil {
			return err
		}
		return persistReviewReassignment(ctx, tx, change)
	})
}

func persistReviewReassignment(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	if err := updateReviewAssignment(ctx, tx, change); err != nil {
		return err
	}
	if err := insertReviewReassignment(ctx, tx, change); err != nil {
		return err
	}
	return appendReviewReassignmentEffects(ctx, tx, change)
}

func updateReviewAssignment(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	assignment, err := marshalReviewAssignment(change.Task.Assignment)
	if err != nil {
		return err
	}
	result := tx.WithContext(ctx).Model(&reviewTaskModel{}).
		Where("id = ? AND status IN ?", change.Task.ID, []string{"Pending", "ReassignmentRequired"}).
		Updates(map[string]any{"assignee_snapshot": assignment, "status": deploydomain.ReviewTaskPending})
	if result.Error != nil {
		return runtimeWriteError("update workflow review assignment", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}

func insertReviewReassignment(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	model, err := reviewReassignmentToModel(change)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return runtimeWriteError("record workflow review reassignment", err)
	}
	return nil
}

func reviewReassignmentToModel(change deployapp.WorkflowReviewReassignmentChange) (reviewReassignmentModel, error) {
	previous, err := marshalReviewAssignment(change.PreviousAssignment)
	if err != nil {
		return reviewReassignmentModel{}, err
	}
	current, err := marshalReviewAssignment(change.Reassignment.Assignment)
	if err != nil {
		return reviewReassignmentModel{}, err
	}
	return reviewReassignmentModel{
		ID: uuid.New(), ReviewTaskID: change.Task.ID, ActorID: change.Reassignment.ActorID,
		PreviousAssigneeSnapshot: previous, AssigneeSnapshot: current,
		Reason:             change.Reassignment.Reason,
		PermissionSnapshot: datatypes.JSON(`{"permission":"deployment_request.reassign","authorized":true}`),
		IdempotencyKey:     change.Mutation.IdempotencyKey, ReassignedAt: change.Reassignment.ReassignedAt,
	}, nil
}

func appendReviewReassignmentEffects(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	event, err := reviewReassignedEvent(change)
	if err != nil {
		return err
	}
	if err := appendReviewReassignmentAudit(ctx, tx, change); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func reviewReassignedEvent(change deployapp.WorkflowReviewReassignmentChange) (platform.Event, error) {
	payload, _ := json.Marshal(map[string]any{
		"requestVersionId": change.Task.RequestVersionID,
		"reviewTaskId":     change.Task.ID,
	})
	event, err := platform.NewEvent(
		"deployment.review.reassigned", "deployment_review_task",
		change.Task.ID.String(), payload, change.Mutation.OccurredAt,
	)
	if err != nil {
		return platform.Event{}, fmt.Errorf("create workflow review reassignment event: %w", err)
	}
	return event, nil
}

func appendReviewReassignmentAudit(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewReassignmentChange) error {
	actorID := change.Mutation.ActorID
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: change.Mutation.OccurredAt, ActorID: &actorID,
		Action: "deployment_review.reassign", ResourceType: "deployment_review_task",
		ResourceID: change.Task.ID.String(), RequestID: change.Mutation.RequestID,
		Metadata: map[string]any{"reason": change.Reassignment.Reason},
	})
}
