package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// WorkflowRuntimeRepository persists workflow instances, reviews, and deployment intents.
type WorkflowRuntimeRepository struct{ db *gorm.DB }

var _ deployapp.WorkflowRuntimeRepository = (*WorkflowRuntimeRepository)(nil)

// NewWorkflowRuntimeRepository creates the PostgreSQL runtime repository.
func NewWorkflowRuntimeRepository(db *gorm.DB) (*WorkflowRuntimeRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &WorkflowRuntimeRepository{db: db}, nil
}

// Load returns the pinned workflow graph and its current instance state.
func (r *WorkflowRuntimeRepository) Load(ctx context.Context, requestVersionID uuid.UUID) (deployapp.WorkflowRuntimeSnapshot, error) {
	var model workflowRuntimeSourceModel
	if err := r.db.WithContext(ctx).Raw(workflowRuntimeQuery, requestVersionID).Scan(&model).Error; err != nil {
		return deployapp.WorkflowRuntimeSnapshot{}, fmt.Errorf("load workflow runtime: %w", err)
	}
	if model.RequestVersionID == uuid.Nil {
		return deployapp.WorkflowRuntimeSnapshot{}, deployapp.ErrWorkflowRuntimeNotFound
	}
	snapshot, err := workflowRuntimeSnapshot(model)
	if err != nil || snapshot.Instance == nil {
		return snapshot, err
	}
	return r.loadReviewState(ctx, snapshot)
}

// Start atomically inserts an instance, initial review, and optional deployment intent.
func (r *WorkflowRuntimeRepository) Start(ctx context.Context, change deployapp.WorkflowStartChange) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		model := workflowInstanceToModel(change.Result.Instance, change.Mutation.OccurredAt)
		if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
			return runtimeWriteError("start workflow instance", err)
		}
		if err := createRuntimeReview(ctx, tx, runtimeReviewInsert{change.Result.Instance.ID, change.Review, change.Mutation.OccurredAt}); err != nil {
			return err
		}
		return appendRuntimeEffects(ctx, tx, runtimeEffects{mutation: change.Mutation, result: change.Result})
	})
}

// ApplyTransition atomically advances an instance and emits persisted deployment intent.
func (r *WorkflowRuntimeRepository) ApplyTransition(ctx context.Context, change deployapp.WorkflowTransitionChange) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		return applyRuntimeTransition(ctx, tx, change)
	})
}

func applyRuntimeTransition(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowTransitionChange) error {
	lock := runtimeLockChange{versionID: change.Result.Instance.RequestVersionID, expected: change.ExpectedLock, next: change.Result.Instance.LockVersion}
	if err := updateRequestVersionLock(ctx, tx, lock); err != nil {
		return err
	}
	if err := updateRuntimeInstance(ctx, tx, change); err != nil {
		return err
	}
	if err := insertRuntimeTransition(ctx, tx, change); err != nil {
		return err
	}
	return finishRuntimeTransition(ctx, tx, change)
}

func finishRuntimeTransition(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowTransitionChange) error {
	if err := closeCurrentReview(ctx, tx, change); err != nil {
		return err
	}
	if err := createRuntimeReview(ctx, tx, runtimeReviewInsert{change.Result.Instance.ID, change.Review, change.Mutation.OccurredAt}); err != nil {
		return err
	}
	return appendRuntimeEffects(ctx, tx, runtimeEffects{mutation: change.Mutation, result: change.Result})
}

// ApplyReview atomically appends a decision and updates its task projection.
func (r *WorkflowRuntimeRepository) ApplyReview(ctx context.Context, change deployapp.WorkflowReviewChange) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		lock := runtimeLockChange{versionID: change.Task.RequestVersionID, expected: change.ExpectedLock, next: reviewNextLock(change)}
		if err := updateRequestVersionLock(ctx, tx, lock); err != nil {
			return err
		}
		if err := updateReviewRuntime(ctx, tx, change); err != nil {
			return err
		}
		if err := updateRuntimeReview(ctx, tx, change); err != nil {
			return err
		}
		if err := insertRuntimeDecision(ctx, tx, change); err != nil {
			return err
		}
		if err := appendReviewEffects(ctx, tx, change); err != nil {
			return err
		}
		return applyReviewTransition(ctx, tx, change)
	})
}

type runtimeEffects struct {
	mutation deployapp.WorkflowRuntimeMutation
	result   deploydomain.WorkflowResult
}

type runtimeReviewInsert struct {
	instanceID uuid.UUID
	review     *deploydomain.ReviewTask
	now        time.Time
}

func (r *WorkflowRuntimeRepository) loadReviewState(ctx context.Context, snapshot deployapp.WorkflowRuntimeSnapshot) (deployapp.WorkflowRuntimeSnapshot, error) {
	var model reviewTaskModel
	err := r.db.WithContext(ctx).Where("workflow_instance_id = ?", snapshot.Instance.ID).
		Order("stage_number DESC").First(&model).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.WorkflowRuntimeSnapshot{}, fmt.Errorf("load active workflow review: %w", err)
	}
	if err == nil {
		task, taskErr := r.loadReviewTask(ctx, model, snapshot.RequestCreatorID)
		if taskErr != nil {
			return deployapp.WorkflowRuntimeSnapshot{}, taskErr
		}
		snapshot.CurrentReview = &task
	}
	snapshot.NextReviewStage, err = r.nextReviewStage(ctx, snapshot.Instance.ID)
	return snapshot, err
}

func (r *WorkflowRuntimeRepository) loadReviewTask(ctx context.Context, model reviewTaskModel, creatorID uuid.UUID) (deploydomain.ReviewTask, error) {
	var decisions []reviewDecisionModel
	if err := r.db.WithContext(ctx).Where("review_task_id = ?", model.ID).Order("decided_at, id").Find(&decisions).Error; err != nil {
		return deploydomain.ReviewTask{}, fmt.Errorf("load workflow review decisions: %w", err)
	}
	values := make([]deploydomain.ReviewDecision, 0, len(decisions))
	for _, decision := range decisions {
		values = append(values, deploydomain.ReviewDecision{
			ReviewerID: decision.ReviewerID, Decision: deploydomain.ReviewDecisionType(decision.Decision),
			Reason: decision.Reason, DecidedAt: decision.DecidedAt,
		})
	}
	return reviewTaskFromModel(model, creatorID, values)
}

func (r *WorkflowRuntimeRepository) nextReviewStage(ctx context.Context, instanceID uuid.UUID) (int, error) {
	var maximum int
	err := r.db.WithContext(ctx).Model(&reviewTaskModel{}).
		Where("workflow_instance_id = ?", instanceID).Select("COALESCE(MAX(stage_number), 0)").Scan(&maximum).Error
	if err != nil {
		return 0, fmt.Errorf("load next workflow review stage: %w", err)
	}
	return maximum + 1, nil
}

func createRuntimeReview(ctx context.Context, tx *gorm.DB, value runtimeReviewInsert) error {
	if value.review == nil {
		return nil
	}
	model, err := reviewTaskToModel(value.instanceID, *value.review, value.now)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return runtimeWriteError("create workflow review task", err)
	}
	return nil
}

func updateRuntimeInstance(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowTransitionChange) error {
	instance := change.Result.Instance
	result := tx.WithContext(ctx).Model(&workflowInstanceModel{}).
		Where("id = ? AND lock_version = ?", instance.ID, change.ExpectedLock).
		Updates(map[string]any{
			"current_state_key": instance.CurrentStateKey, "status": instance.Status,
			"lock_version": instance.LockVersion, "completed_at": instance.CompletedAt,
			"updated_at": change.Mutation.OccurredAt,
		})
	if result.Error != nil {
		return runtimeWriteError("advance workflow instance", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}

func insertRuntimeTransition(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowTransitionChange) error {
	actorID := optionalActorID(change.Mutation.ActorID)
	value := change.Result.Transition
	model := workflowTransitionRecordModel{
		ID: uuid.New(), WorkflowInstanceID: change.Result.Instance.ID,
		RequestVersionID: change.Result.Instance.RequestVersionID,
		FromStateKey:     value.FromStateKey, ToStateKey: value.ToStateKey,
		TransitionKey: value.TransitionKey, ActorID: actorID, Reason: value.Reason,
		IdempotencyKey: change.Mutation.IdempotencyKey, OccurredAt: value.OccurredAt,
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return runtimeWriteError("record workflow transition", err)
	}
	return nil
}

func closeCurrentReview(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowTransitionChange) error {
	result := tx.WithContext(ctx).Model(&reviewTaskModel{}).
		Where("workflow_instance_id = ? AND state_key = ? AND status IN ?",
			change.Result.Instance.ID, change.Result.Transition.FromStateKey,
			[]string{"Pending", "ReassignmentRequired"}).
		Updates(map[string]any{"status": "Closed", "closed_at": change.Mutation.OccurredAt})
	if result.Error != nil {
		return runtimeWriteError("close workflow review task", result.Error)
	}
	return nil
}

func updateRuntimeReview(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	closedAt := reviewClosedAt(change.Task.Status, change.Mutation.OccurredAt)
	result := tx.WithContext(ctx).Model(&reviewTaskModel{}).
		Where("id = ? AND status IN ?", change.Task.ID, []string{"Pending", "ReassignmentRequired"}).
		Updates(map[string]any{"status": change.Task.Status, "closed_at": closedAt})
	if result.Error != nil {
		return runtimeWriteError("update workflow review task", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return nil
}

func insertRuntimeDecision(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	permission := datatypes.JSON(`{"permission":"deployment_request.review","authorized":true}`)
	model := reviewDecisionModel{
		ID: uuid.New(), ReviewTaskID: change.Task.ID, ReviewerID: change.Decision.ReviewerID,
		Decision: string(change.Decision.Decision), Reason: change.Decision.Reason,
		PermissionSnapshot: permission, IdempotencyKey: change.Mutation.IdempotencyKey,
		DecidedAt: change.Decision.DecidedAt,
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return runtimeWriteError("record workflow review decision", err)
	}
	return nil
}

func appendReviewEffects(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	payload, _ := json.Marshal(map[string]any{
		"requestVersionId": change.Task.RequestVersionID,
		"reviewTaskId":     change.Task.ID, "status": change.Task.Status,
	})
	event, err := platform.NewEvent(
		"deployment.review.decided", "deployment_request_version",
		change.Task.RequestVersionID.String(), payload, change.Mutation.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("create workflow review event: %w", err)
	}
	if err := appendReviewAudit(ctx, tx, change); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func appendReviewAudit(ctx context.Context, tx *gorm.DB, change deployapp.WorkflowReviewChange) error {
	actorID := change.Mutation.ActorID
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: change.Mutation.OccurredAt, ActorID: &actorID,
		Action: "deployment_review.decide", ResourceType: "deployment_review_task",
		ResourceID: change.Task.ID.String(), RequestID: change.Mutation.RequestID,
		Metadata: map[string]any{"decision": change.Decision.Decision, "status": change.Task.Status},
	})
}

func optionalActorID(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func runtimeWriteError(operation string, err error) error {
	if errors.Is(workflowWriteError(operation, err), deployapp.ErrWorkflowConflict) {
		return deployapp.ErrWorkflowRuntimeConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}
