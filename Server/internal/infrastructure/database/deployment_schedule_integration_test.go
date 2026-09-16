//go:build integration

package database_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestDeploymentScheduleRepositoryRoundTripAndIdempotency(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	repository, err := deployinfra.NewDeploymentScheduleRepository(db)
	if err != nil {
		t.Fatalf("create deployment schedule repository: %v", err)
	}
	scope, err := repository.ResolveEnvironment(ctx, ids.environmentID)
	if err != nil || scope.OrganizationID != ids.organizationID || scope.ProjectID != ids.projectID {
		t.Fatalf("resolve schedule Environment = %#v, error = %v", scope, err)
	}
	if value, getErr := repository.Get(ctx, ids.environmentID); getErr != nil || value != nil {
		t.Fatalf("default schedule = %#v, error = %v", value, getErr)
	}

	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	policy := integrationSchedulePolicy(t, ids.environmentID, 1, 60)
	mutation := deployapp.DeploymentScheduleMutation{
		ActorID: ids.userID, OrganizationID: ids.organizationID, RequestID: "schedule-create",
		IdempotencyKey: "schedule-1", ExpectedVersion: 0, OccurredAt: now,
	}
	created, err := repository.Put(ctx, mutation, policy)
	if err != nil || created.Version != 1 || created.Blackouts[0].StartsAt.Location() != time.UTC {
		t.Fatalf("created schedule = %#v, error = %v", created, err)
	}
	replayed, err := repository.Put(ctx, mutation, policy)
	if err != nil || replayed.Version != created.Version {
		t.Fatalf("replayed schedule = %#v, error = %v", replayed, err)
	}
	assertCountWhere(t, db, "deployment_schedule_commands", "idempotency_key = ?", mutation.IdempotencyKey, 1)
	assertCountWhere(t, db, "audit_logs", "request_id = ?", mutation.RequestID, 1)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", ids.environmentID.String(), 1)

	conflicting := integrationSchedulePolicy(t, ids.environmentID, 1, 120)
	if _, err := repository.Put(ctx, mutation, conflicting); !errors.Is(err, deployapp.ErrDeploymentScheduleConflict) {
		t.Fatalf("idempotency reuse error = %v", err)
	}

	updated := integrationSchedulePolicy(t, ids.environmentID, 2, 120)
	updateMutation := mutation
	updateMutation.RequestID = "schedule-update"
	updateMutation.IdempotencyKey = "schedule-2"
	updateMutation.ExpectedVersion = 1
	stored, err := repository.Put(ctx, updateMutation, updated)
	if err != nil || stored.Version != 2 || stored.WeeklyWindows[0].StartMinute != 120 {
		t.Fatalf("updated schedule = %#v, error = %v", stored, err)
	}

	staleMutation := updateMutation
	staleMutation.RequestID = "schedule-stale"
	staleMutation.IdempotencyKey = "schedule-3"
	if _, err := repository.Put(ctx, staleMutation, integrationSchedulePolicy(t, ids.environmentID, 2, 180)); !errors.Is(err, deployapp.ErrDeploymentScheduleConflict) {
		t.Fatalf("stale schedule error = %v", err)
	}
	assertCountWhere(t, db, "deployment_schedule_commands", "idempotency_key = ?", staleMutation.IdempotencyKey, 0)
	assertCountWhere(t, db, "audit_logs", "request_id = ?", staleMutation.RequestID, 0)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", ids.environmentID.String(), 2)
}

func TestDeploymentScheduleRepositoryRejectsConcurrentStaleUpdates(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	repository, _ := deployinfra.NewDeploymentScheduleRepository(db)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	base := deployapp.DeploymentScheduleMutation{
		ActorID: ids.userID, OrganizationID: ids.organizationID, RequestID: "schedule-base",
		IdempotencyKey: "schedule-base", ExpectedVersion: 0, OccurredAt: now,
	}
	if _, err := repository.Put(ctx, base, integrationSchedulePolicy(t, ids.environmentID, 1, 60)); err != nil {
		t.Fatalf("create base schedule: %v", err)
	}

	errorsByUpdate := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	for index, minute := range []int{120, 180} {
		go func(index, minute int) {
			ready.Done()
			<-start
			mutation := deployapp.DeploymentScheduleMutation{
				ActorID: ids.userID, OrganizationID: ids.organizationID,
				RequestID: "schedule-concurrent", IdempotencyKey: "schedule-concurrent-" + string(rune('a'+index)),
				ExpectedVersion: 1, OccurredAt: now.Add(time.Minute),
			}
			_, err := repository.Put(ctx, mutation, integrationSchedulePolicy(t, ids.environmentID, 2, minute))
			errorsByUpdate <- err
		}(index, minute)
	}
	ready.Wait()
	close(start)

	var succeeded, conflicted int
	for range 2 {
		err := <-errorsByUpdate
		if err == nil {
			succeeded++
		} else if errors.Is(err, deployapp.ErrDeploymentScheduleConflict) {
			conflicted++
		} else {
			t.Fatalf("concurrent update error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent updates succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func integrationSchedulePolicy(t *testing.T, environmentID uuid.UUID, version uint64, startMinute int) deploydomain.DeploymentSchedulePolicy {
	t.Helper()
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: environmentID, Enabled: true, TimeZone: "Asia/Taipei", Version: version,
		WeeklyWindows: []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: startMinute, EndMinute: startMinute + 60}},
		Blackouts: []deploydomain.DeploymentScheduleBlackout{{
			StartsAt: time.Date(2026, 9, 21, 2, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
			EndsAt:   time.Date(2026, 9, 21, 3, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
		}},
	})
	if err != nil {
		t.Fatalf("create integration schedule: %v", err)
	}
	return policy
}
