package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// DeploymentScheduleGate evaluates durable jobs against the latest Environment policy.
type DeploymentScheduleGate struct{ db *gorm.DB }

var _ deployapp.DeploymentScheduleEvaluator = (*DeploymentScheduleGate)(nil)

// NewDeploymentScheduleGate creates the worker schedule adapter.
func NewDeploymentScheduleGate(db *gorm.DB) (*DeploymentScheduleGate, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentScheduleGate{db: db}, nil
}

// Evaluate resolves one job to its immutable Request Version and current policy.
func (g *DeploymentScheduleGate) Evaluate(ctx context.Context, job deploydomain.DeploymentJob, now time.Time) (deploydomain.DeploymentEligibility, error) {
	versionID, err := deploymentScheduleJobVersionID(ctx, g.db, job)
	if err != nil {
		return deploydomain.DeploymentEligibility{}, err
	}
	return calculateDeploymentScheduleEligibility(ctx, g.db, versionID, now)
}

type deploymentScheduleTarget struct {
	EnvironmentID uuid.UUID
	ScheduledFor  *time.Time
}

func deploymentScheduleJobVersionID(ctx context.Context, db *gorm.DB, job deploydomain.DeploymentJob) (uuid.UUID, error) {
	switch job.AggregateType {
	case "deployment_request_version":
		return job.AggregateID, nil
	case "deployment_execution":
		var execution struct{ RequestVersionID uuid.UUID }
		if err := db.WithContext(ctx).Table("deployment_executions").Select("request_version_id").Take(&execution, "id = ?", job.AggregateID).Error; err != nil {
			return uuid.Nil, fmt.Errorf("resolve deployment schedule execution: %w", err)
		}
		return execution.RequestVersionID, nil
	default:
		return uuid.Nil, deploydomain.ErrInvalidJob
	}
}

func calculateDeploymentScheduleEligibility(ctx context.Context, db *gorm.DB, versionID uuid.UUID, now time.Time) (deploydomain.DeploymentEligibility, error) {
	target, err := loadDeploymentScheduleTarget(ctx, db, versionID)
	if err != nil {
		return deploydomain.DeploymentEligibility{}, err
	}
	policy, err := loadDeploymentSchedulePolicy(ctx, db, target.EnvironmentID)
	if err != nil {
		return deploydomain.DeploymentEligibility{}, err
	}
	eligibility, err := deploydomain.CalculateDeploymentEligibility(now, target.ScheduledFor, policy)
	if err != nil {
		return deploydomain.DeploymentEligibility{}, fmt.Errorf("calculate deployment schedule eligibility: %w", err)
	}
	return eligibility, nil
}

func loadDeploymentScheduleTarget(ctx context.Context, db *gorm.DB, versionID uuid.UUID) (deploymentScheduleTarget, error) {
	var target deploymentScheduleTarget
	err := db.WithContext(ctx).Table("deployment_request_versions AS version").
		Select("request.environment_id, version.scheduled_for").
		Joins("JOIN deployment_requests AS request ON request.id = version.request_id").
		Take(&target, "version.id = ?", versionID).Error
	if err != nil {
		return deploymentScheduleTarget{}, fmt.Errorf("load deployment schedule target: %w", err)
	}
	return target, nil
}

func loadDeploymentSchedulePolicy(ctx context.Context, db *gorm.DB, environmentID uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	var model deploymentSchedulePolicyModel
	err := db.WithContext(ctx).Take(&model, "environment_id = ?", environmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load deployment schedule policy: %w", err)
	}
	policy, err := deploymentSchedulePolicyFromModel(model)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}
