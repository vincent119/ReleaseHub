//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestDeploymentPlanRepositoryPreservesVersionLifecycle(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	repository, err := deployinfra.NewDeploymentPlanRepository(db)
	if err != nil {
		t.Fatalf("create deployment plan repository: %v", err)
	}
	now := time.Date(2026, 9, 3, 9, 10, 11, 0, time.UTC)
	plan := deploymentPlanAggregate(t, ids.projectID, ids.userID, now)
	mutation := deployapp.PlanMutation{ActorID: ids.userID, RequestID: "plan-integration", OccurredAt: now}
	if err := repository.Create(ctx, mutation, plan); err != nil {
		t.Fatalf("persist deployment plan: %v", err)
	}
	second := deploymentPlanVersion(t, plan.ID, 2, ids.userID, now)
	if err := repository.AppendVersion(ctx, mutation, 1, second); !errors.Is(err, deployapp.ErrPlanConflict) {
		t.Fatalf("append with active draft error = %v", err)
	}
	published := publishDeploymentPlanVersion(t, plan.Versions[0], now)
	if err := repository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish deployment plan version: %v", err)
	}
	if err := repository.AppendVersion(ctx, mutation, 1, second); err != nil {
		t.Fatalf("append deployment plan version: %v", err)
	}
	loaded, err := repository.Load(ctx, plan.ID)
	if err != nil || len(loaded.Versions) != 2 || loaded.Versions[0].Lifecycle != deploydomain.DefinitionPublished {
		t.Fatalf("loaded deployment plan = %#v, error = %v", loaded, err)
	}
	listed, err := repository.List(ctx, ids.projectID)
	if err != nil || len(listed) != 1 || listed[0].ID != plan.ID {
		t.Fatalf("listed deployment plans = %#v, error = %v", listed, err)
	}
	scope, err := repository.ResolveProject(ctx, ids.projectID)
	if err != nil || scope.OrganizationID != ids.organizationID || scope.ProjectID != ids.projectID {
		t.Fatalf("resolved Project scope = %#v, error = %v", scope, err)
	}
	assertCountWhere(t, db, "audit_logs", "request_id = ?", mutation.RequestID, 3)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", plan.ID.String(), 3)
}

func deploymentPlanAggregate(t *testing.T, projectID, actorID uuid.UUID, now time.Time) deploydomain.DeploymentPlan {
	t.Helper()
	value, err := deploydomain.NewDeploymentPlan(deploydomain.DeploymentPlan{
		ID: uuid.New(), OwnerKind: deploydomain.DeploymentPlanOwnerProject,
		OwnerProjectID: &projectID, Name: "Production", CreatedBy: actorID, CreatedAt: now,
	}, integrationDeploymentPlanDocument())
	if err != nil {
		t.Fatalf("create deployment plan aggregate: %v", err)
	}
	return value
}

func deploymentPlanVersion(t *testing.T, planID uuid.UUID, number uint64, actorID uuid.UUID, now time.Time) deploydomain.DeploymentPlanVersion {
	t.Helper()
	value, err := deploydomain.NewDeploymentPlanVersion(deploydomain.DeploymentPlanVersionDraft{
		PlanID: planID, VersionNumber: number, ActorID: actorID,
		Document: integrationDeploymentPlanDocument(), CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create deployment plan version: %v", err)
	}
	return value
}

func publishDeploymentPlanVersion(t *testing.T, version deploydomain.DeploymentPlanVersion, now time.Time) deploydomain.DeploymentPlanVersion {
	t.Helper()
	value, err := version.ChangeLifecycle(deploydomain.DefinitionPublished, 1, now)
	if err != nil {
		t.Fatalf("publish deployment plan version: %v", err)
	}
	return value
}

func integrationDeploymentPlanDocument() deploydomain.DeploymentPlanDocument {
	return deploydomain.DeploymentPlanDocument{
		Nodes: []deploydomain.DeploymentPlanNode{{
			Key: "api", ApplicationKey: "api", SuccessCondition: deploydomain.DeploymentPlanCondition{
				SyncStatuses: []string{"Synced"}, HealthStatuses: []string{"Healthy"},
			},
		}},
		Edges: []deploydomain.DeploymentPlanEdge{},
	}
}
