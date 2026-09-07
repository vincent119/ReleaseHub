package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (queue *DeploymentJobQueue) Claim(ctx context.Context, request deployapp.JobClaimRequest) (deploydomain.JobLease, error) {
	if strings.TrimSpace(request.Owner) == "" || request.LeaseDuration <= 0 || request.Now.IsZero() {
		return deploydomain.JobLease{}, deploydomain.ErrInvalidJob
	}
	var lease deploydomain.JobLease
	err := queue.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := failExhaustedJobLeases(tx, request); err != nil {
			return err
		}
		var claimError error
		lease, claimError = claimDeploymentJob(tx, request)
		return claimError
	})
	return lease, queueClaimError(err)
}

func failExhaustedJobLeases(tx *gorm.DB, request deployapp.JobClaimRequest) error {
	result := tx.Exec(`UPDATE deployment_jobs SET status = 'Failed', lease_owner = NULL,
		lease_expires_at = NULL, last_error_code = 'lease_exhausted',
		last_error_message = 'job lease expired after maximum attempts', updated_at = ?
		WHERE status = 'Running' AND lease_expires_at <= ? AND attempts >= max_attempts`, request.Now.UTC(), request.Now.UTC())
	if result.Error != nil {
		return fmt.Errorf("fail exhausted deployment job leases: %w", result.Error)
	}
	return nil
}

func claimDeploymentJob(tx *gorm.DB, request deployapp.JobClaimRequest) (deploydomain.JobLease, error) {
	var model deploymentJobModel
	err := tx.Raw(claimDeploymentJobSQL(), request.Now.UTC(), request.Now.UTC(), request.Type, request.Type, request.Owner,
		request.Now.Add(request.LeaseDuration).UTC(), request.Now.UTC()).Scan(&model).Error
	if err != nil {
		return deploydomain.JobLease{}, fmt.Errorf("claim deployment job: %w", err)
	}
	if model.ID == uuid.Nil {
		return deploydomain.JobLease{}, deploydomain.ErrNoJobAvailable
	}
	return jobLeaseFromModel(model), nil
}

func queueClaimError(err error) error {
	if err == nil || errorsIsQueueDomain(err) {
		return err
	}
	return fmt.Errorf("claim deployment job transaction: %w", err)
}

func errorsIsQueueDomain(err error) bool {
	return errors.Is(err, deploydomain.ErrNoJobAvailable) || errors.Is(err, deploydomain.ErrInvalidJob)
}

func claimDeploymentJobSQL() string {
	return `WITH candidate AS (
		SELECT id FROM deployment_jobs
		WHERE ((status = 'Pending' AND available_at <= ?) OR
		       (status = 'Running' AND lease_expires_at <= ?))
		  AND (? = '' OR job_type = ?)
		  AND attempts < max_attempts
		ORDER BY available_at, created_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE deployment_jobs AS job
	SET status = 'Running', lease_owner = ?, lease_expires_at = ?,
		fencing_token = job.fencing_token + 1, attempts = job.attempts + 1, updated_at = ?
	FROM candidate WHERE job.id = candidate.id RETURNING job.*`
}

func jobLeaseFromModel(model deploymentJobModel) deploydomain.JobLease {
	return deploydomain.JobLease{
		Job: deploymentJobFromModel(model), Owner: *model.LeaseOwner,
		FencingToken: model.FencingToken, ExpiresAt: model.LeaseExpiresAt.UTC(),
	}
}
