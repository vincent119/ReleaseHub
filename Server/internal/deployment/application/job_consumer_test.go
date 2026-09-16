package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestJobConsumerDefersDeploymentWhenScheduleChanges(t *testing.T) {
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	next := now.Add(2 * time.Hour)
	fixture := newScheduleConsumerFixture(t, now, deploydomain.DeploymentEligibility{
		EligibleNow: false, NextEligibleAt: next,
		Reason: deploydomain.DeploymentScheduleWaitingForMaintenanceWindow, PolicyVersion: 4,
	})

	require.NoError(t, fixture.consumer.ProcessOnce(t.Context()))
	require.Equal(t, next, fixture.queue.deferred.AvailableAt)
	require.Equal(t, 0, fixture.executor.calls)
	require.Equal(t, 0, fixture.queue.retryCalls)
	require.Equal(t, 0, fixture.queue.succeedCalls)
	require.Equal(t, "MaintenanceWindow", fixture.observer.value.Reason)
	require.Equal(t, uint64(4), fixture.observer.value.PolicyVersion)
}

func TestDeploymentScheduleDoesNotInterruptStartedExecution(t *testing.T) {
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	fixture := newScheduleConsumerFixture(t, now, deploydomain.DeploymentEligibility{
		EligibleNow: true, NextEligibleAt: now,
		Reason: deploydomain.DeploymentScheduleReady, PolicyVersion: 7,
	})
	fixture.executor.execute = func(context.Context, uuid.UUID) error {
		fixture.gate.result = deploydomain.DeploymentEligibility{
			EligibleNow: false, NextEligibleAt: now.Add(24 * time.Hour),
			Reason: deploydomain.DeploymentScheduleWaitingForBlackout, PolicyVersion: 8,
		}
		return nil
	}

	require.NoError(t, fixture.consumer.ProcessOnce(t.Context()))
	require.Equal(t, 1, fixture.gate.calls)
	require.Equal(t, 1, fixture.executor.calls)
	require.Equal(t, 1, fixture.queue.succeedCalls)
	require.True(t, fixture.queue.deferred.AvailableAt.IsZero())
}

func TestJobConsumerDefersUnavailableScheduleWithoutFailureRetry(t *testing.T) {
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	fixture := newScheduleConsumerFixture(t, now, deploydomain.DeploymentEligibility{})
	fixture.gate.err = deploydomain.ErrDeploymentScheduleUnavailable

	require.NoError(t, fixture.consumer.ProcessOnce(t.Context()))
	require.Equal(t, now.Add(time.Minute), fixture.queue.deferred.AvailableAt)
	require.Equal(t, "Unavailable", fixture.observer.value.Reason)
	require.Zero(t, fixture.queue.retryCalls)
}

func TestJobConsumerRetriesScheduleInfrastructureFailure(t *testing.T) {
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	fixture := newScheduleConsumerFixture(t, now, deploydomain.DeploymentEligibility{})
	fixture.gate.err = context.DeadlineExceeded

	require.NoError(t, fixture.consumer.ProcessOnce(t.Context()))
	require.Equal(t, 1, fixture.queue.retryCalls)
	require.Zero(t, fixture.executor.calls)
	require.True(t, fixture.queue.deferred.AvailableAt.IsZero())
}

type scheduleConsumerFixture struct {
	consumer *DeploymentJobConsumer
	queue    *scheduleQueueStub
	executor *scheduleExecutorStub
	gate     *scheduleGateStub
	observer *scheduleObserverStub
}

func newScheduleConsumerFixture(t *testing.T, now time.Time, eligibility deploydomain.DeploymentEligibility) scheduleConsumerFixture {
	t.Helper()
	job, err := deploydomain.NewDeploymentJob(deploydomain.DeploymentJobDraft{
		Type: deploydomain.JobExecuteDeployment, AggregateType: "deployment_request_version",
		AggregateID: uuid.New(), Payload: json.RawMessage(`{}`), IdempotencyKey: "schedule-job",
		AvailableAt: now, MaxAttempts: 3,
	}, now)
	require.NoError(t, err)
	job.Attempts = 1
	queue := &scheduleQueueStub{lease: deploydomain.JobLease{
		Job: job, Owner: "worker-a", FencingToken: 1, ExpiresAt: now.Add(time.Minute),
	}}
	executor := &scheduleExecutorStub{}
	gate := &scheduleGateStub{result: eligibility}
	observer := &scheduleObserverStub{}
	consumer, err := NewDeploymentJobConsumer(DeploymentJobConsumerOptions{
		Queue: queue, Executor: executor, Schedules: gate, Observer: observer,
		Owner: "worker-a", PollInterval: time.Second, LeaseTTL: time.Minute, RetryDelay: time.Minute,
	})
	require.NoError(t, err)
	consumer.now = func() time.Time { return now }
	return scheduleConsumerFixture{consumer: consumer, queue: queue, executor: executor, gate: gate, observer: observer}
}

type scheduleQueueStub struct {
	lease        deploydomain.JobLease
	deferred     JobDeferRequest
	retryCalls   int
	succeedCalls int
}

func (*scheduleQueueStub) Enqueue(context.Context, deploydomain.DeploymentJob) (deploydomain.DeploymentJob, bool, error) {
	return deploydomain.DeploymentJob{}, false, nil
}
func (s *scheduleQueueStub) Claim(context.Context, JobClaimRequest) (deploydomain.JobLease, error) {
	return s.lease, nil
}
func (s *scheduleQueueStub) Heartbeat(context.Context, deploydomain.JobLease, time.Time) (deploydomain.JobLease, error) {
	return s.lease, nil
}
func (s *scheduleQueueStub) Succeed(context.Context, deploydomain.JobLease, time.Time) error {
	s.succeedCalls++
	return nil
}
func (s *scheduleQueueStub) Defer(_ context.Context, request JobDeferRequest) error {
	s.deferred = request
	return nil
}
func (s *scheduleQueueStub) Retry(context.Context, JobRetryRequest) error {
	s.retryCalls++
	return nil
}

type scheduleExecutorStub struct {
	calls   int
	execute func(context.Context, uuid.UUID) error
}

func (s *scheduleExecutorStub) Execute(ctx context.Context, id uuid.UUID) error {
	s.calls++
	if s.execute != nil {
		return s.execute(ctx, id)
	}
	return nil
}
func (s *scheduleExecutorStub) ExecuteAttempt(ctx context.Context, id uuid.UUID) error {
	return s.Execute(ctx, id)
}

type scheduleGateStub struct {
	result deploydomain.DeploymentEligibility
	err    error
	calls  int
}

func (s *scheduleGateStub) Evaluate(context.Context, deploydomain.DeploymentJob, time.Time) (deploydomain.DeploymentEligibility, error) {
	s.calls++
	return s.result, s.err
}

type scheduleObserverStub struct {
	value DeploymentScheduleDeferObservation
}

func (s *scheduleObserverStub) ObserveDeploymentScheduleDefer(value DeploymentScheduleDeferObservation) {
	s.value = value
}
