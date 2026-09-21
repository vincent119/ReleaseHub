package infrastructure

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

// ListApprovedReviewRecoveries returns only current approved reviews on running instances.
func (r *WorkflowRuntimeRepository) ListApprovedReviewRecoveries(ctx context.Context) ([]deployapp.ApprovedReviewRecovery, error) {
	var values []deployapp.ApprovedReviewRecovery
	err := r.db.WithContext(ctx).Raw(approvedReviewRecoveryQuery).Scan(&values).Error
	if err != nil {
		return nil, fmt.Errorf("list approved review recoveries: %w", err)
	}
	return values, nil
}

const approvedReviewRecoveryQuery = `
	SELECT instance.request_version_id
	FROM deployment_workflow_instances AS instance
	JOIN deployment_review_tasks AS task ON task.workflow_instance_id = instance.id
	WHERE instance.status = 'Running'
		AND task.status = 'Approved'
		AND task.state_key = instance.current_state_key
		AND NOT EXISTS (
			SELECT 1 FROM deployment_review_tasks AS newer
			WHERE newer.workflow_instance_id = instance.id
				AND newer.stage_number > task.stage_number
		)
`

func reviewNextLock(change deployapp.WorkflowReviewChange) uint64 {
	if change.Result != nil {
		return change.Result.Instance.LockVersion
	}
	return change.ExpectedLock + 1
}

func updateReviewRuntime(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	if change.Result == nil {
		return advanceReviewRuntimeLock(ctx, tx, change)
	}
	return updateRuntimeInstance(ctx, tx, reviewTransitionChange(change))
}

func applyReviewTransition(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	if change.Result == nil {
		return nil
	}
	transition := reviewTransitionChange(change)
	if err := insertRuntimeTransition(ctx, tx, transition); err != nil {
		return err
	}
	if err := createRuntimeReview(ctx, tx, runtimeReviewInsert{change.Result.Instance.ID, change.NextReview, change.Mutation.OccurredAt}); err != nil {
		return err
	}
	return appendRuntimeEffects(ctx, tx, runtimeEffects{mutation: change.Mutation, result: *change.Result})
}

func reviewTransitionChange(change deployapp.WorkflowReviewChange) deployapp.WorkflowTransitionChange {
	return deployapp.WorkflowTransitionChange{
		Mutation: change.Mutation, ExpectedLock: change.ExpectedLock,
		Result: *change.Result, Review: change.NextReview,
	}
}
