//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestDeploymentBindingRepositorySwitchesPublishedVersions(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, err := deployinfra.NewDeploymentBindingRepository(db)
	if err != nil {
		t.Fatalf("create binding repository: %v", err)
	}
	scopeInput := deployapp.BindingScopeInput{
		OrganizationID: ids.organizationID, ProjectID: ids.projectID, EnvironmentID: ids.environmentID,
	}
	scope, err := repository.ResolveEnvironment(ctx, scopeInput)
	if err != nil {
		t.Fatalf("resolve Environment: %v", err)
	}
	current, err := repository.Get(ctx, scope)
	if err != nil || current == nil || current.Version != 1 {
		t.Fatalf("current binding = %#v, error = %v", current, err)
	}
	workflowVersionID, planVersionID := seedAlternatePublishedDefinitions(t, db, ids)
	input := deployapp.BindDefinitionsInput{
		OrganizationID: ids.organizationID, ProjectID: ids.projectID, EnvironmentID: ids.environmentID,
		WorkflowVersionID: workflowVersionID, PlanVersionID: planVersionID, ExpectedVersion: 1,
	}
	mutation := deployapp.BindingMutation{ActorID: ids.userID, RequestID: "binding-switch", OccurredAt: time.Now().UTC()}
	updated, err := repository.Bind(ctx, mutation, scope, input)
	if err != nil || updated.Version != 2 || updated.WorkflowVersionID != workflowVersionID {
		t.Fatalf("updated binding = %#v, error = %v", updated, err)
	}
	if _, err := repository.Bind(ctx, mutation, scope, input); !errors.Is(err, deployapp.ErrBindingConflict) {
		t.Fatalf("stale binding error = %v", err)
	}
	assertCountWhere(t, db, "audit_logs", "request_id = ?", mutation.RequestID, 1)
	assertCountWhere(t, db, "outbox_events", "aggregate_id = ?", updated.ID.String(), 1)
}

func TestDeploymentBindingRepositoryCreatesUnboundEnvironment(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	environmentID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO environments (id, organization_id, project_id, name, environment_type) VALUES (?, ?, ?, 'staging', 'Staging')`, environmentID, ids.organizationID, ids.projectID)
	repository, _ := deployinfra.NewDeploymentBindingRepository(db)
	scope, err := repository.ResolveEnvironment(ctx, deployapp.BindingScopeInput{
		OrganizationID: ids.organizationID, ProjectID: ids.projectID, EnvironmentID: environmentID,
	})
	if err != nil {
		t.Fatalf("resolve unbound Environment: %v", err)
	}
	input := deployapp.BindDefinitionsInput{
		OrganizationID: ids.organizationID, ProjectID: ids.projectID, EnvironmentID: environmentID,
		WorkflowVersionID: ids.workflowVersionID, PlanVersionID: ids.planVersionID, ExpectedVersion: 0,
	}
	mutation := deployapp.BindingMutation{ActorID: ids.userID, RequestID: "binding-create", OccurredAt: time.Now().UTC()}
	created, err := repository.Bind(ctx, mutation, scope, input)
	if err != nil || created.Version != 1 || created.EnvironmentID != environmentID {
		t.Fatalf("created binding = %#v, error = %v", created, err)
	}
}

func seedAlternatePublishedDefinitions(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) (uuid.UUID, uuid.UUID) {
	t.Helper()
	workflowID, workflowVersionID := uuid.New(), uuid.New()
	planID, planVersionID := uuid.New(), uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO release_workflows (id, name, created_by) VALUES (?, 'alternate approval', ?)`, workflowID, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO release_workflow_versions (id, workflow_id, version_number, lifecycle, document, created_by, published_at) VALUES (?, ?, 1, 'Published', '{"initialState":"start","states":[{"key":"start","name":"Start","type":"Start"}],"transitions":[]}', ?, now())`, workflowVersionID, workflowID, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO deployment_plans (id, owner_kind, owner_project_id, name, created_by) VALUES (?, 'project', ?, 'alternate plan', ?)`, planID, ids.projectID, ids.userID)
	execDeploymentSQL(t, db, `INSERT INTO deployment_plan_versions (id, plan_id, version_number, lifecycle, document, created_by, published_at) VALUES (?, ?, 1, 'Published', '{"nodes":[{"key":"api","applicationKey":"api","order":0,"successCondition":{"syncStatuses":["Synced"],"healthStatuses":["Healthy"],"stabilizationSeconds":0}}],"edges":[]}', ?, now())`, planVersionID, planID, ids.userID)
	return workflowVersionID, planVersionID
}
