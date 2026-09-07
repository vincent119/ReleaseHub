//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestDeploymentJobQueueConcurrentClaimAndDuplicateProducer(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	queue, err := deployinfra.NewDeploymentJobQueue(db)
	if err != nil {
		t.Fatalf("create deployment job queue: %v", err)
	}
	job := deploymentJobFixture(t, "queue-concurrent", 3)
	persisted, created, err := queue.Enqueue(ctx, job)
	if err != nil || !created {
		t.Fatalf("enqueue deployment job: %#v %t %v", persisted, created, err)
	}
	duplicate, created, err := queue.Enqueue(ctx, job)
	if err != nil || created || duplicate.ID != persisted.ID {
		t.Fatalf("deduplicate deployment job: %#v %t %v", duplicate, created, err)
	}
	conflict := job
	conflict.ID, conflict.Payload = uuid.New(), json.RawMessage(`{"changed":true}`)
	if _, _, err := queue.Enqueue(ctx, conflict); !errors.Is(err, deploydomain.ErrJobConflict) {
		t.Fatalf("conflicting duplicate error = %v", err)
	}
	assertSingleConcurrentClaim(t, ctx, queue)
}

func TestDeploymentJobQueueLeaseRecoveryAndFencing(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	queue, _ := deployinfra.NewDeploymentJobQueue(db)
	now := time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC)
	job := deploymentJobFixtureAt(t, "queue-recovery", 3, now)
	if _, _, err := queue.Enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue deployment job: %v", err)
	}
	first := claimJob(t, ctx, queue, "worker-a", now)
	heartbeat, err := queue.Heartbeat(ctx, first, now.Add(30*time.Second))
	if err != nil || !heartbeat.ExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("heartbeat deployment job: %#v %v", heartbeat, err)
	}
	assertNoJob(t, ctx, queue, now.Add(61*time.Second))
	if err := queue.Succeed(ctx, heartbeat, now.Add(91*time.Second)); !errors.Is(err, deploydomain.ErrStaleJobLease) {
		t.Fatalf("expired worker completion error = %v", err)
	}
	second := claimJob(t, ctx, queue, "worker-b", now.Add(91*time.Second))
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("fencing token did not advance: %d <= %d", second.FencingToken, first.FencingToken)
	}
	if err := queue.Succeed(ctx, first, now.Add(92*time.Second)); !errors.Is(err, deploydomain.ErrStaleJobLease) {
		t.Fatalf("stale worker completion error = %v", err)
	}
	if err := queue.Succeed(ctx, second, now.Add(93*time.Second)); err != nil {
		t.Fatalf("complete recovered job: %v", err)
	}
}

func TestDeploymentJobQueueRetryReusesJobAndStopsAtMaximum(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	queue, _ := deployinfra.NewDeploymentJobQueue(db)
	now := time.Date(2026, 9, 7, 4, 5, 6, 0, time.UTC)
	job := deploymentJobFixtureAt(t, "queue-retry", 2, now)
	if _, _, err := queue.Enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue deployment job: %v", err)
	}
	first := claimJob(t, ctx, queue, "worker-a", now)
	retryJob(t, ctx, queue, first, now.Add(time.Second))
	second := claimJob(t, ctx, queue, "worker-b", now.Add(2*time.Second))
	if second.Job.ID != first.Job.ID || second.Job.Attempts != 2 {
		t.Fatalf("retry created another job or attempt mismatch: %#v", second.Job)
	}
	retryJob(t, ctx, queue, second, now.Add(3*time.Second))
	assertNoJob(t, ctx, queue, now.Add(4*time.Second))
	assertJobState(t, db, job.ID, "Failed", 2)
}

func assertSingleConcurrentClaim(t *testing.T, ctx context.Context, queue *deployinfra.DeploymentJobQueue) {
	t.Helper()
	now := time.Now().UTC()
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, owner := range []string{"worker-a", "worker-b"} {
		group.Add(1)
		go func(owner string) {
			defer group.Done()
			<-start
			_, err := queue.Claim(ctx, deployapp.JobClaimRequest{Owner: owner, Now: now, LeaseDuration: time.Minute})
			results <- err
		}(owner)
	}
	close(start)
	group.Wait()
	close(results)
	assertClaimResults(t, results)
}

func assertClaimResults(t *testing.T, results <-chan error) {
	t.Helper()
	succeeded, unavailable := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, deploydomain.ErrNoJobAvailable) {
			unavailable++
		} else {
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if succeeded != 1 || unavailable != 1 {
		t.Fatalf("claim counts = success %d unavailable %d", succeeded, unavailable)
	}
}

func deploymentJobFixture(t *testing.T, key string, maxAttempts int) deploydomain.DeploymentJob {
	t.Helper()
	return deploymentJobFixtureAt(t, key, maxAttempts, time.Now().UTC().Add(-time.Second))
}

func deploymentJobFixtureAt(t *testing.T, key string, maxAttempts int, now time.Time) deploydomain.DeploymentJob {
	t.Helper()
	job, err := deploydomain.NewDeploymentJob(deploydomain.DeploymentJobDraft{
		Type: deploydomain.JobExecuteDeployment, AggregateType: "deployment_execution",
		AggregateID: uuid.New(), Payload: json.RawMessage(`{"requestVersionId":"request-version"}`),
		IdempotencyKey: key, AvailableAt: now, MaxAttempts: maxAttempts,
	}, now)
	if err != nil {
		t.Fatalf("create deployment job: %v", err)
	}
	return job
}

func claimJob(t *testing.T, ctx context.Context, queue *deployinfra.DeploymentJobQueue, owner string, now time.Time) deploydomain.JobLease {
	t.Helper()
	lease, err := queue.Claim(ctx, deployapp.JobClaimRequest{Owner: owner, Now: now, LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("claim deployment job: %v", err)
	}
	return lease
}

func assertNoJob(t *testing.T, ctx context.Context, queue *deployinfra.DeploymentJobQueue, now time.Time) {
	t.Helper()
	_, err := queue.Claim(ctx, deployapp.JobClaimRequest{Owner: "worker-c", Now: now, LeaseDuration: time.Minute})
	if !errors.Is(err, deploydomain.ErrNoJobAvailable) {
		t.Fatalf("expected no available job, got %v", err)
	}
}

func retryJob(t *testing.T, ctx context.Context, queue *deployinfra.DeploymentJobQueue, lease deploydomain.JobLease, now time.Time) {
	t.Helper()
	err := queue.Retry(ctx, deployapp.JobRetryRequest{
		Lease: lease, Now: now, AvailableAt: now, ErrorCode: "temporary_failure",
		ErrorMessage: "temporary infrastructure failure",
	})
	if err != nil {
		t.Fatalf("retry deployment job: %v", err)
	}
}

func assertJobState(t *testing.T, db *gorm.DB, jobID uuid.UUID, status string, attempts int) {
	t.Helper()
	var actual struct {
		Status   string
		Attempts int
	}
	if err := db.Table("deployment_jobs").Select("status, attempts").Where("id = ?", jobID).Take(&actual).Error; err != nil {
		t.Fatalf("load deployment job state: %v", err)
	}
	if actual.Status != status || actual.Attempts != attempts {
		t.Fatalf("deployment job state = %s/%d, want %s/%d", actual.Status, actual.Attempts, status, attempts)
	}
}
