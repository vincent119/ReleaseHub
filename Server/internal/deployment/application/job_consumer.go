package application

import (
	"context"
	"errors"
	"time"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// DeploymentJobConsumer polls only deployment execution jobs.
type DeploymentJobConsumer struct {
	queue        DeploymentJobQueue
	executor     *DeploymentExecutor
	owner        string
	pollInterval time.Duration
	leaseTTL     time.Duration
	retryDelay   time.Duration
	now          func() time.Time
}

// DeploymentJobConsumerOptions contains typed runtime settings.
type DeploymentJobConsumerOptions struct {
	Queue        DeploymentJobQueue
	Executor     *DeploymentExecutor
	Owner        string
	PollInterval time.Duration
	LeaseTTL     time.Duration
	RetryDelay   time.Duration
}

// NewDeploymentJobConsumer validates worker queue dependencies.
func NewDeploymentJobConsumer(options DeploymentJobConsumerOptions) (*DeploymentJobConsumer, error) {
	if options.Queue == nil || options.Executor == nil || options.Owner == "" ||
		options.PollInterval <= 0 || options.LeaseTTL <= 0 || options.RetryDelay <= 0 {
		return nil, errors.New("invalid deployment job consumer dependencies")
	}
	return &DeploymentJobConsumer{
		queue: options.Queue, executor: options.Executor, owner: options.Owner,
		pollInterval: options.PollInterval, leaseTTL: options.LeaseTTL,
		retryDelay: options.RetryDelay, now: time.Now,
	}, nil
}

// Run processes available jobs until shutdown.
func (c *DeploymentJobConsumer) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		if err := c.ProcessOnce(ctx); err != nil && !errors.Is(err, deploydomain.ErrNoJobAvailable) {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOnce claims and resolves one infrastructure attempt.
func (c *DeploymentJobConsumer) ProcessOnce(ctx context.Context) error {
	now := c.now().UTC()
	lease, err := c.queue.Claim(ctx, JobClaimRequest{
		Owner: c.owner, Type: deploydomain.JobExecuteDeployment, Now: now, LeaseDuration: c.leaseTTL,
	})
	if err != nil {
		return err
	}
	lease, executionErr := c.executeWithHeartbeat(ctx, lease)
	if executionErr != nil {
		return c.retry(ctx, lease, executionErr)
	}
	return c.queue.Succeed(ctx, lease, c.now().UTC())
}

func (c *DeploymentJobConsumer) executeWithHeartbeat(ctx context.Context, lease deploydomain.JobLease) (deploydomain.JobLease, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- c.executeJob(runCtx, lease.Job) }()
	return c.waitForExecution(runCtx, lease, result)
}

func (c *DeploymentJobConsumer) executeJob(ctx context.Context, job deploydomain.DeploymentJob) error {
	if job.AggregateType == "deployment_execution" {
		return c.executor.ExecuteAttempt(ctx, job.AggregateID)
	}
	return c.executor.Execute(ctx, job.AggregateID)
}

func (c *DeploymentJobConsumer) waitForExecution(ctx context.Context, lease deploydomain.JobLease, result <-chan error) (deploydomain.JobLease, error) {
	ticker := time.NewTicker(c.leaseTTL / 3)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			return lease, err
		case <-ticker.C:
			updated, err := c.queue.Heartbeat(ctx, lease, c.now().UTC())
			if err != nil {
				return lease, err
			}
			lease = updated
		case <-ctx.Done():
			return lease, ctx.Err()
		}
	}
}

func (c *DeploymentJobConsumer) retry(ctx context.Context, lease deploydomain.JobLease, cause error) error {
	now := c.now().UTC()
	err := c.queue.Retry(ctx, JobRetryRequest{
		Lease: lease, Now: now, AvailableAt: now.Add(c.retryDelay),
		ErrorCode: "deployment_execution_failed", ErrorMessage: cause.Error(),
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return nil
}
