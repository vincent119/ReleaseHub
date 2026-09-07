package infrastructure

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type executionWorkflowCommand struct {
	ctx      context.Context
	tx       *gorm.DB
	mutation deployapp.ExecutionMutation
	result   deploydomain.WorkflowResult
}

func persistExecutionCommandWorkflow(value executionWorkflowCommand) error {
	if value.result.Instance.ID == uuid.Nil || value.result.Instance.LockVersion == 0 {
		return deployapp.ErrExecutionConflict
	}
	change := executionWorkflowTransition(value)
	if err := updateRuntimeInstance(value.ctx, value.tx, change); err != nil {
		return err
	}
	if err := insertRuntimeTransition(value.ctx, value.tx, change); err != nil {
		return err
	}
	return appendWorkflowRuntimeEvent(value.ctx, value.tx, runtimeEffects{
		mutation: change.Mutation,
		result:   change.Result,
	})
}

func executionWorkflowTransition(value executionWorkflowCommand) deployapp.WorkflowTransitionChange {
	return deployapp.WorkflowTransitionChange{
		Mutation: deployapp.WorkflowRuntimeMutation{
			ActorID: value.mutation.ActorID, RequestID: value.mutation.RequestID,
			IdempotencyKey: value.mutation.IdempotencyKey, OccurredAt: value.mutation.OccurredAt,
		},
		ExpectedLock: value.result.Instance.LockVersion - 1,
		Result:       value.result,
	}
}
