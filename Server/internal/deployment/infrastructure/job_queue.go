package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type DeploymentJobQueue struct {
	db *gorm.DB
}

func NewDeploymentJobQueue(db *gorm.DB) (*DeploymentJobQueue, error) {
	if db == nil {
		return nil, errors.New("deployment job queue database is required")
	}
	return &DeploymentJobQueue{db: db}, nil
}

func (queue *DeploymentJobQueue) Enqueue(ctx context.Context, job deploydomain.DeploymentJob) (deploydomain.DeploymentJob, bool, error) {
	model := deploymentJobToModel(job)
	result := queue.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&model)
	if result.Error != nil {
		return deploydomain.DeploymentJob{}, false, fmt.Errorf("enqueue deployment job: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return deploymentJobFromModel(model), true, nil
	}
	return queue.loadDuplicate(ctx, job)
}

func (queue *DeploymentJobQueue) loadDuplicate(ctx context.Context, job deploydomain.DeploymentJob) (deploydomain.DeploymentJob, bool, error) {
	var model deploymentJobModel
	err := queue.db.WithContext(ctx).Where("idempotency_key = ?", job.IdempotencyKey).Take(&model).Error
	if err != nil {
		return deploydomain.DeploymentJob{}, false, fmt.Errorf("load duplicate deployment job: %w", err)
	}
	if !sameDeploymentJob(model, job) {
		return deploydomain.DeploymentJob{}, false, deploydomain.ErrJobConflict
	}
	return deploymentJobFromModel(model), false, nil
}

func sameDeploymentJob(model deploymentJobModel, job deploydomain.DeploymentJob) bool {
	return model.JobType == string(job.Type) && model.AggregateType == job.AggregateType &&
		model.AggregateID == job.AggregateID && sameJSON(model.Payload, job.Payload)
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue map[string]any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
