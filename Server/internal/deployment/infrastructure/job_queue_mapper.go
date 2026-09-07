package infrastructure

import (
	"bytes"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func deploymentJobToModel(job deploydomain.DeploymentJob) deploymentJobModel {
	return deploymentJobModel{
		ID: job.ID, JobType: string(job.Type), AggregateType: job.AggregateType,
		AggregateID: job.AggregateID, Payload: bytes.Clone(job.Payload), Status: job.Status,
		IdempotencyKey: job.IdempotencyKey, AvailableAt: job.AvailableAt,
		Attempts: job.Attempts, MaxAttempts: job.MaxAttempts,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
}

func deploymentJobFromModel(model deploymentJobModel) deploydomain.DeploymentJob {
	return deploydomain.DeploymentJob{
		ID: model.ID, Type: deploydomain.JobType(model.JobType), AggregateType: model.AggregateType,
		AggregateID: model.AggregateID, Payload: bytes.Clone(model.Payload), Status: model.Status,
		IdempotencyKey: model.IdempotencyKey, AvailableAt: model.AvailableAt,
		Attempts: model.Attempts, MaxAttempts: model.MaxAttempts,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}
