package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

// Replay returns the immutable result of a previously accepted command.
func (r *ExecutionRepository) Replay(ctx context.Context, input deployapp.ExecutionCommandReplay) (deployapp.DeploymentExecution, bool, error) {
	var model executionCommandModel
	err := r.db.WithContext(ctx).Where(
		"request_version_id = ? AND command_type = ? AND idempotency_key = ?",
		input.RequestVersionID, input.Command, input.IdempotencyKey,
	).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.DeploymentExecution{}, false, nil
	}
	if err != nil {
		return deployapp.DeploymentExecution{}, false, fmt.Errorf("load deployment execution command: %w", err)
	}
	if model.RequestHash != input.RequestHash {
		return deployapp.DeploymentExecution{}, false, deployapp.ErrExecutionConflict
	}
	value, err := r.loadReplayResult(ctx, model)
	return value, true, err
}

func (r *ExecutionRepository) loadReplayResult(ctx context.Context, model executionCommandModel) (deployapp.DeploymentExecution, error) {
	executionID := model.ExecutionID
	if model.ResultExecutionID != nil {
		executionID = *model.ResultExecutionID
	}
	value, err := r.LoadByID(ctx, executionID)
	return value.Execution, err
}

func loadExecutionView(ctx context.Context, tx *gorm.DB, executionID uuid.UUID) (deployapp.DeploymentExecution, error) {
	repository := &ExecutionRepository{db: tx}
	snapshot, err := repository.LoadByID(ctx, executionID)
	return snapshot.Execution, err
}

func executionCommandWriteError(err error) error {
	if err == nil || errors.Is(err, deployapp.ErrExecutionConflict) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return deployapp.ErrExecutionConflict
	}
	return err
}
