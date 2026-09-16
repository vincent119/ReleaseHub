package application

import (
	"context"
	"time"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type JobClaimRequest struct {
	Owner         string
	Type          deploydomain.JobType
	Now           time.Time
	LeaseDuration time.Duration
}

type JobRetryRequest struct {
	Lease        deploydomain.JobLease
	Now          time.Time
	AvailableAt  time.Time
	ErrorCode    string
	ErrorMessage string
}

// JobDeferRequest returns a claimed job to its durable schedule without recording a failure.
type JobDeferRequest struct {
	Lease       deploydomain.JobLease
	Now         time.Time
	AvailableAt time.Time
}

type DeploymentJobQueue interface {
	Enqueue(context.Context, deploydomain.DeploymentJob) (deploydomain.DeploymentJob, bool, error)
	Claim(context.Context, JobClaimRequest) (deploydomain.JobLease, error)
	Heartbeat(context.Context, deploydomain.JobLease, time.Time) (deploydomain.JobLease, error)
	Succeed(context.Context, deploydomain.JobLease, time.Time) error
	Defer(context.Context, JobDeferRequest) error
	Retry(context.Context, JobRetryRequest) error
}
