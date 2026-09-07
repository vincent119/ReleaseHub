package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// DeploymentExecutor runs immutable Plan snapshots after an all-target preflight.
type DeploymentExecutor struct {
	repository  ExecutionRepository
	preflight   *PreflightService
	argo        DeploymentArgoClient
	locks       ApplicationLocks
	maxParallel int
	lockTTL     time.Duration
	now         func() time.Time
}

// NewDeploymentExecutor validates the executor composition root.
func NewDeploymentExecutor(options DeploymentExecutorOptions) (*DeploymentExecutor, error) {
	if options.Repository == nil || options.Preflight == nil || options.Argo == nil ||
		options.Locks == nil || options.MaxParallel < 1 || options.LockTTL <= 0 {
		return nil, errors.New("invalid deployment executor dependencies")
	}
	return &DeploymentExecutor{
		repository: options.Repository, preflight: options.Preflight, argo: options.Argo,
		locks: options.Locks, maxParallel: options.MaxParallel,
		lockTTL: options.LockTTL, now: time.Now,
	}, nil
}

// Execute runs one workflow deployment intent without creating a business retry.
func (e *DeploymentExecutor) Execute(ctx context.Context, versionID uuid.UUID) error {
	return e.execute(ctx, versionID, false)
}

// ExecuteAttempt resumes an already-created business retry attempt.
func (e *DeploymentExecutor) ExecuteAttempt(ctx context.Context, executionID uuid.UUID) error {
	return e.execute(ctx, executionID, true)
}

func (e *DeploymentExecutor) execute(ctx context.Context, id uuid.UUID, existing bool) error {
	snapshot, locks, err := e.prepareAndLock(ctx, id, existing)
	if err != nil {
		return err
	}
	if snapshot.Status == "Terminated" {
		return nil
	}
	stopped, err := e.ensurePreflight(ctx, snapshot, locks)
	if err != nil || stopped {
		return err
	}
	return e.runAndComplete(ctx, snapshot, locks)
}

func (e *DeploymentExecutor) runAndComplete(ctx context.Context, snapshot ExecutionSnapshot, locks deploydomain.ApplicationLockSet) error {
	err := e.executeWithLockHeartbeat(ctx, snapshot, locks)
	if errors.Is(err, ErrExecutionStopped) {
		return nil
	}
	var failed nodeBatchFailure
	if err != nil && !errors.As(err, &failed) {
		return err
	}
	status, err := e.repository.Complete(ctx, snapshot.ID, e.now().UTC())
	if err != nil {
		return err
	}
	if status.ReleasesApplicationLocks() {
		_ = e.locks.Release(ctx, locks)
	}
	return nil
}

func (e *DeploymentExecutor) prepareAndLock(ctx context.Context, id uuid.UUID, existing bool) (ExecutionSnapshot, deploydomain.ApplicationLockSet, error) {
	snapshot, err := e.loadExecution(ctx, id, existing)
	if err != nil {
		return ExecutionSnapshot{}, deploydomain.ApplicationLockSet{}, err
	}
	if snapshot.Status == "Terminated" {
		return snapshot, deploydomain.ApplicationLockSet{}, nil
	}
	locks, err := e.acquireLocks(ctx, snapshot)
	return snapshot, locks, err
}

func (e *DeploymentExecutor) loadExecution(ctx context.Context, id uuid.UUID, existing bool) (ExecutionSnapshot, error) {
	if existing {
		return e.repository.Load(ctx, id)
	}
	return e.repository.Prepare(ctx, id, e.now().UTC())
}

func (e *DeploymentExecutor) ensurePreflight(ctx context.Context, snapshot ExecutionSnapshot, locks deploydomain.ApplicationLockSet) (bool, error) {
	if snapshot.Status != "Preflight" {
		return false, nil
	}
	blocked, err := e.preflightAll(ctx, snapshot)
	if err != nil || blocked {
		_ = e.locks.Release(ctx, locks)
		return blocked, err
	}
	if err := e.repository.MarkRunning(ctx, snapshot.ID, e.now().UTC()); err != nil {
		_ = e.locks.Release(ctx, locks)
		return false, err
	}
	return false, nil
}

func (e *DeploymentExecutor) executeWithLockHeartbeat(ctx context.Context, snapshot ExecutionSnapshot, locks deploydomain.ApplicationLockSet) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- e.executePlan(runCtx, snapshot) }()
	ticker := time.NewTicker(e.lockTTL / 3)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			return err
		case <-ticker.C:
			if err := e.heartbeatLocks(runCtx, locks); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (e *DeploymentExecutor) heartbeatLocks(ctx context.Context, locks deploydomain.ApplicationLockSet) error {
	_, err := e.locks.Heartbeat(ctx, locks, e.now().UTC())
	if err != nil {
		return fmt.Errorf("heartbeat deployment Application locks: %w", err)
	}
	return nil
}

func (e *DeploymentExecutor) acquireLocks(ctx context.Context, snapshot ExecutionSnapshot) (deploydomain.ApplicationLockSet, error) {
	ids := make([]uuid.UUID, 0, len(snapshot.Targets))
	for _, target := range snapshot.Targets {
		ids = append(ids, target.Preflight.Snapshot.ApplicationID)
	}
	return e.locks.Acquire(ctx, deploydomain.ApplicationLockRequest{
		ExecutionID: snapshot.ID, ApplicationIDs: ids, OwnerToken: uuid.New(),
		Now: e.now().UTC(), LeaseDuration: e.lockTTL,
	})
}

func (e *DeploymentExecutor) preflightAll(ctx context.Context, snapshot ExecutionSnapshot) (bool, error) {
	for _, target := range snapshot.Targets {
		result, err := e.preflight.Check(ctx, target.Preflight)
		if err != nil {
			return false, err
		}
		if !result.Allowed {
			err = e.repository.Block(ctx, ExecutionBlock{
				ExecutionID: snapshot.ID, RequestApplicationID: target.Preflight.Snapshot.ID,
				Code: result.Code, OccurredAt: e.now().UTC(),
			})
			return true, err
		}
	}
	return false, nil
}
