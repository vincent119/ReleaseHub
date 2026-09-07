//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestApplicationLocksAreAllOrNothingAndScopedToIntersection(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	requestVersionID, _ := seedDeploymentRequest(t, db, ids)
	applications := []uuid.UUID{ids.applicationID, seedLockApplication(t, db, ids, "worker"), seedLockApplication(t, db, ids, "scheduler")}
	executions := seedLockExecutions(t, db, requestVersionID, ids.planVersionID, 3)
	locks, _ := deployinfra.NewApplicationLocks(db)
	now := time.Date(2026, 9, 7, 2, 3, 4, 0, time.UTC)
	first := acquireLockSet(t, ctx, locks, lockRequest(executions[0], applications[:2], now))
	third := acquireLockSet(t, ctx, locks, lockRequest(executions[2], applications[2:], now))

	request := lockRequest(executions[1], []uuid.UUID{applications[1], applications[2]}, now)
	if _, err := locks.Acquire(ctx, request); !errors.Is(err, deploydomain.ErrApplicationLocked) {
		t.Fatalf("overlapping lock set error = %v", err)
	}
	assertExecutionLockCount(t, db, executions[1], 0)
	if err := locks.Release(ctx, first); err != nil {
		t.Fatalf("release first lock set: %v", err)
	}
	if err := locks.Release(ctx, third); err != nil {
		t.Fatalf("release disjoint lock set: %v", err)
	}
	acquireLockSet(t, ctx, locks, request)
}

func TestApplicationLocksRecoverAfterWorkerRestartWithFencing(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	requestVersionID, _ := seedDeploymentRequest(t, db, ids)
	executions := seedLockExecutions(t, db, requestVersionID, ids.planVersionID, 2)
	locks, _ := deployinfra.NewApplicationLocks(db)
	now := time.Date(2026, 9, 7, 3, 4, 5, 0, time.UTC)
	first := acquireLockSet(t, ctx, locks, lockRequest(executions[0], []uuid.UUID{ids.applicationID}, now))
	activeRequest := lockRequest(executions[1], []uuid.UUID{ids.applicationID}, now.Add(30*time.Second))
	if _, err := locks.Acquire(ctx, activeRequest); !errors.Is(err, deploydomain.ErrApplicationLocked) {
		t.Fatalf("active lock takeover error = %v", err)
	}
	if _, err := locks.Heartbeat(ctx, first, now.Add(61*time.Second)); !errors.Is(err, deploydomain.ErrStaleApplicationLock) {
		t.Fatalf("expired lock heartbeat error = %v", err)
	}
	recoveryRequest := lockRequest(executions[1], []uuid.UUID{ids.applicationID}, now.Add(2*time.Minute))
	recovered := acquireLockSet(t, ctx, locks, recoveryRequest)
	if recovered.Leases[0].FencingToken <= first.Leases[0].FencingToken {
		t.Fatal("recovered lock fencing token did not advance")
	}
	if _, err := locks.Heartbeat(ctx, first, now.Add(3*time.Minute)); !errors.Is(err, deploydomain.ErrStaleApplicationLock) {
		t.Fatalf("stale lock heartbeat error = %v", err)
	}
	if err := locks.Release(ctx, recovered); err != nil {
		t.Fatalf("release recovered lock: %v", err)
	}
}

func lockRequest(executionID uuid.UUID, applicationIDs []uuid.UUID, now time.Time) deploydomain.ApplicationLockRequest {
	return deploydomain.ApplicationLockRequest{
		ExecutionID: executionID, ApplicationIDs: applicationIDs, OwnerToken: uuid.New(),
		Now: now, LeaseDuration: time.Minute,
	}
}

func acquireLockSet(t *testing.T, ctx context.Context, locks *deployinfra.ApplicationLocks, request deploydomain.ApplicationLockRequest) deploydomain.ApplicationLockSet {
	t.Helper()
	set, err := locks.Acquire(ctx, request)
	if err != nil {
		t.Fatalf("acquire application lock set: %v", err)
	}
	return set
}

func seedLockApplication(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, name string) uuid.UUID {
	t.Helper()
	applicationID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO applications (
		id, organization_id, project_id, environment_id, name,
		argocd_namespace, argocd_application_name, argocd_project,
		destination_server, destination_namespace, source_repo_url, source_target_revision, source_path
	) VALUES (?, ?, ?, ?, ?, 'argocd', ?, 'apps',
		'https://kubernetes.default.svc', ?, 'https://git.example/gitops.git', 'main', ?)`,
		applicationID, ids.organizationID, ids.projectID, ids.environmentID, name,
		name+"-production", name, "production/"+name)
	return applicationID
}

func seedLockExecutions(t *testing.T, db *gorm.DB, requestVersionID, planVersionID uuid.UUID, count int) []uuid.UUID {
	t.Helper()
	result := make([]uuid.UUID, 0, count)
	for attempt := 1; attempt <= count; attempt++ {
		executionID := uuid.New()
		execDeploymentSQL(t, db, `INSERT INTO deployment_executions (
			id, request_version_id, plan_version_id, attempt, status, trigger_kind, plan_snapshot
		) VALUES (?, ?, ?, ?, 'Queued', 'Workflow', '{}')`, executionID, requestVersionID, planVersionID, attempt)
		result = append(result, executionID)
	}
	return result
}

func assertExecutionLockCount(t *testing.T, db *gorm.DB, executionID uuid.UUID, expected int64) {
	t.Helper()
	var count int64
	if err := db.Table("deployment_application_locks").Where("execution_id = ?", executionID).Count(&count).Error; err != nil {
		t.Fatalf("count execution locks: %v", err)
	}
	if count != expected {
		t.Fatalf("execution lock count = %d, want %d", count, expected)
	}
}
