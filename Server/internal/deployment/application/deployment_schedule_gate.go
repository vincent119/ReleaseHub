package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// DeploymentScheduleEvaluator resolves the latest policy for a durable deployment job.
type DeploymentScheduleEvaluator interface {
	Evaluate(context.Context, deploydomain.DeploymentJob, time.Time) (deploydomain.DeploymentEligibility, error)
}

// DeploymentScheduleObserver records low-cardinality defer telemetry.
type DeploymentScheduleObserver interface {
	ObserveDeploymentScheduleDefer(DeploymentScheduleDeferObservation)
}

// DeploymentScheduleDeferObservation contains bounded operational evidence for one policy defer.
type DeploymentScheduleDeferObservation struct {
	JobType        deploydomain.JobType
	Reason         string
	Wait           time.Duration
	NextEligibleAt time.Time
	PolicyVersion  uint64
}

// DeploymentJobExecutor starts initial and business-retry deployment attempts.
type DeploymentJobExecutor interface {
	Execute(context.Context, uuid.UUID) error
	ExecuteAttempt(context.Context, uuid.UUID) error
}
