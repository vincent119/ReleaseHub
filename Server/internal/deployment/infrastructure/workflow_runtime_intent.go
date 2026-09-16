package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func appendRuntimeEffects(ctx context.Context, tx *gorm.DB, effects runtimeEffects) error {
	if effects.result.DeploymentIntent {
		if err := insertDeploymentIntent(ctx, tx, effects); err != nil {
			return err
		}
	}
	return appendWorkflowRuntimeEvent(ctx, tx, effects)
}

func insertDeploymentIntent(ctx context.Context, tx *gorm.DB, effects runtimeEffects) error {
	eligibility, err := calculateDeploymentScheduleEligibility(
		ctx, tx, effects.result.Instance.RequestVersionID, effects.mutation.OccurredAt,
	)
	if err != nil {
		return runtimeWriteError("calculate workflow deployment schedule", err)
	}
	return createDeploymentIntentJob(ctx, tx, effects, eligibility.NextEligibleAt)
}

func createDeploymentIntentJob(ctx context.Context, tx *gorm.DB, effects runtimeEffects, availableAt time.Time) error {
	payload, _ := json.Marshal(map[string]any{
		"requestVersionId":   effects.result.Instance.RequestVersionID,
		"workflowInstanceId": effects.result.Instance.ID,
		"workflowStateKey":   effects.result.Instance.CurrentStateKey,
	})
	model := deploymentJobModel{
		ID: uuid.New(), JobType: "ExecuteDeployment", AggregateType: "deployment_request_version",
		AggregateID: effects.result.Instance.RequestVersionID, Payload: payload, Status: "Pending",
		IdempotencyKey: "workflow-deployment:" + effects.mutation.IdempotencyKey,
		AvailableAt:    availableAt, MaxAttempts: 10,
		CreatedAt: effects.mutation.OccurredAt, UpdatedAt: effects.mutation.OccurredAt,
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return runtimeWriteError("persist workflow deployment intent", err)
	}
	return nil
}

func appendWorkflowRuntimeEvent(ctx context.Context, tx *gorm.DB, effects runtimeEffects) error {
	payload, _ := json.Marshal(map[string]any{
		"requestVersionId": effects.result.Instance.RequestVersionID,
		"stateKey":         effects.result.Instance.CurrentStateKey,
	})
	event, err := platform.NewEvent(
		"deployment.workflow.advanced", "deployment_request_version",
		effects.result.Instance.RequestVersionID.String(), payload, effects.mutation.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("create workflow runtime event: %w", err)
	}
	if err := appendWorkflowRuntimeAudit(ctx, tx, effects); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func appendWorkflowRuntimeAudit(ctx context.Context, tx *gorm.DB, effects runtimeEffects) error {
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: effects.mutation.OccurredAt, ActorID: optionalActorID(effects.mutation.ActorID),
		Action: "deployment_workflow.transition", ResourceType: "deployment_request_version",
		ResourceID: effects.result.Instance.RequestVersionID.String(), RequestID: effects.mutation.RequestID,
		Metadata: map[string]any{"stateKey": effects.result.Instance.CurrentStateKey},
	})
}
