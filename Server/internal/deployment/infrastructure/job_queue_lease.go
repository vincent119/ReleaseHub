package infrastructure

import (
	"context"
	"fmt"
	"time"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (queue *DeploymentJobQueue) Heartbeat(ctx context.Context, lease deploydomain.JobLease, now time.Time) (deploydomain.JobLease, error) {
	duration := lease.ExpiresAt.Sub(lease.Job.UpdatedAt)
	if duration <= 0 || now.IsZero() {
		return deploydomain.JobLease{}, deploydomain.ErrInvalidJob
	}
	expiresAt := now.Add(duration).UTC()
	changes := map[string]any{"lease_expires_at": expiresAt, "updated_at": now.UTC()}
	if err := queue.updateLease(ctx, lease, now, changes); err != nil {
		return deploydomain.JobLease{}, err
	}
	lease.ExpiresAt, lease.Job.UpdatedAt = expiresAt, now.UTC()
	return lease, nil
}

func (queue *DeploymentJobQueue) Succeed(ctx context.Context, lease deploydomain.JobLease, now time.Time) error {
	if now.IsZero() {
		return deploydomain.ErrInvalidJob
	}
	changes := terminalJobChanges("Succeeded", now, "", "")
	return queue.updateLease(ctx, lease, now, changes)
}

func (queue *DeploymentJobQueue) Retry(ctx context.Context, request deployapp.JobRetryRequest) error {
	if request.AvailableAt.IsZero() || request.Now.IsZero() {
		return deploydomain.ErrInvalidJob
	}
	status := "Pending"
	if request.Lease.Job.Attempts >= request.Lease.Job.MaxAttempts {
		status = "Failed"
	}
	changes := retryJobChanges(status, request)
	return queue.updateLease(ctx, request.Lease, request.Now, changes)
}

func (queue *DeploymentJobQueue) updateLease(ctx context.Context, lease deploydomain.JobLease, now time.Time, changes map[string]any) error {
	result := queue.db.WithContext(ctx).Model(&deploymentJobModel{}).
		Where("id = ? AND status = 'Running' AND lease_owner = ? AND fencing_token = ? AND lease_expires_at > ?",
			lease.Job.ID, lease.Owner, lease.FencingToken, now.UTC()).Updates(changes)
	if result.Error != nil {
		return fmt.Errorf("update deployment job lease: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return deploydomain.ErrStaleJobLease
	}
	return nil
}

func retryJobChanges(status string, request deployapp.JobRetryRequest) map[string]any {
	return map[string]any{
		"status": status, "available_at": request.AvailableAt.UTC(), "lease_owner": nil,
		"lease_expires_at": nil, "last_error_code": request.ErrorCode,
		"last_error_message": request.ErrorMessage, "updated_at": request.Now.UTC(),
	}
}

func terminalJobChanges(status string, now time.Time, code, message string) map[string]any {
	return map[string]any{
		"status": status, "lease_owner": nil, "lease_expires_at": nil,
		"last_error_code": code, "last_error_message": message, "updated_at": now.UTC(),
	}
}

var _ deployapp.DeploymentJobQueue = (*DeploymentJobQueue)(nil)
