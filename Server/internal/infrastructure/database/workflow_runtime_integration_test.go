//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestWorkflowRuntimePersistsReviewTransitionAndDeploymentIntent(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentSchemaScope(t, db)
	reviewerID := uuid.New()
	replacementID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'workflow-reviewer')`, reviewerID)
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'workflow-replacement')`, replacementID)
	workflowVersion := seedRuntimeWorkflow(t, ctx, db, reviewerID)
	planVersionID := seedRuntimePlan(t, db, ids)
	requestVersionID := seedRuntimeRequest(t, db, ids, workflowVersion.ID, planVersionID)

	repository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		t.Fatalf("create workflow runtime repository: %v", err)
	}
	service := runtimeService(t, repository)
	started, err := service.Start(ctx, deployapp.WorkflowStartInput{
		RequestVersionID: requestVersionID, IdempotencyKey: "runtime-start", RequestID: "runtime-test",
	})
	if err != nil || started.ReviewPolicy == nil {
		t.Fatalf("start workflow runtime: %#v %v", started, err)
	}
	snapshot, err := repository.Load(ctx, requestVersionID)
	if err != nil || snapshot.CurrentReview == nil {
		t.Fatalf("load workflow review: %#v %v", snapshot, err)
	}
	reassignRuntimeReview(t, ctx, service, snapshot, replacementID)
	snapshot, err = repository.Load(ctx, requestVersionID)
	if err != nil || snapshot.CurrentReview == nil || snapshot.CurrentReview.Policy.UserIDs[0] != replacementID {
		t.Fatalf("load reassigned workflow review: %#v %v", snapshot, err)
	}
	approveRuntimeReview(t, ctx, service, snapshot, replacementID)
	snapshot, err = repository.Load(ctx, requestVersionID)
	if err != nil {
		t.Fatalf("reload approved workflow review: %v", err)
	}
	assertRuntimeReviewProjection(t, ctx, db, snapshot)
	transitionRuntimeToDeployment(t, ctx, service, snapshot, reviewerID)

	assertCountWhere(t, db, "deployment_review_decisions", "review_task_id = ?", snapshot.CurrentReview.ID, 1)
	assertCountWhere(t, db, "deployment_review_reassignments", "review_task_id = ?", snapshot.CurrentReview.ID, 1)
	if err := db.Table("deployment_review_reassignments").Where("review_task_id = ?", snapshot.CurrentReview.ID).Update("reason", "changed").Error; err == nil {
		t.Fatal("review reassignment must be append-only")
	}
	assertCountWhere(t, db, "deployment_workflow_transitions", "workflow_instance_id = ?", snapshot.Instance.ID, 1)
	assertCountWhere(t, db, "deployment_jobs", "aggregate_id = ?", requestVersionID, 1)
}

func reassignRuntimeReview(t *testing.T, ctx context.Context, service *deployapp.WorkflowRuntimeService, snapshot deployapp.WorkflowRuntimeSnapshot, replacementID uuid.UUID) {
	t.Helper()
	task, err := service.ReassignReview(ctx, deployapp.WorkflowPrincipal{UserID: replacementID}, deployapp.WorkflowReviewReassignmentInput{
		RequestVersionID: snapshot.RequestVersionID, ReviewTaskID: snapshot.CurrentReview.ID,
		ExpectedLock: snapshot.Instance.LockVersion, UserIDs: []uuid.UUID{replacementID},
		Reason: "reviewer unavailable", IdempotencyKey: "runtime-reassign", RequestID: "runtime-test",
	})
	if err != nil || task.Policy.UserIDs[0] != replacementID {
		t.Fatalf("reassign workflow review: %#v %v", task, err)
	}
}

func assertRuntimeReviewProjection(t *testing.T, ctx context.Context, db *gorm.DB, snapshot deployapp.WorkflowRuntimeSnapshot) {
	t.Helper()
	repository, err := deployinfra.NewDeploymentRequestRepository(db)
	if err != nil {
		t.Fatalf("create request projection repository: %v", err)
	}
	detail, err := repository.Load(ctx, snapshot.RequestID)
	if err != nil || len(detail.Reviews) != 1 || detail.Reviews[0].Status != deploydomain.ReviewTaskApproved || detail.Version.LockVersion != snapshot.Instance.LockVersion {
		t.Fatalf("load request review projection: %#v %v", detail.Reviews, err)
	}
}

func seedRuntimeWorkflow(t *testing.T, ctx context.Context, db *gorm.DB, actorID uuid.UUID) deploydomain.ReleaseWorkflowVersion {
	t.Helper()
	definitionRepository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		t.Fatalf("create workflow definition repository: %v", err)
	}
	now := runtimeIntegrationTime()
	workflow := workflowAggregate(t, actorID, now)
	workflow.Name = "Runtime approval"
	workflow, err = deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: workflow.Name, CreatedBy: actorID, CreatedAt: now,
	}, runtimeIntegrationDocument(actorID))
	if err != nil {
		t.Fatalf("create runtime workflow: %v", err)
	}
	mutation := deployapp.WorkflowMutation{ActorID: actorID, RequestID: "runtime-seed", OccurredAt: now}
	if err := definitionRepository.Create(ctx, mutation, workflow); err != nil {
		t.Fatalf("persist runtime workflow: %v", err)
	}
	published := publishWorkflowVersion(t, workflow.Versions[0], now)
	if err := definitionRepository.UpdateLifecycle(ctx, mutation, 1, published); err != nil {
		t.Fatalf("publish runtime workflow: %v", err)
	}
	return published
}

func seedRuntimePlan(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) uuid.UUID {
	t.Helper()
	planID, versionID := uuid.New(), uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_plans (id, owner_kind, owner_project_id, name, created_by)
		VALUES (?, 'project', ?, 'runtime plan', ?)
	`, planID, ids.projectID, ids.userID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_plan_versions (
			id, plan_id, version_number, lifecycle, document, created_by, published_at
		) VALUES (?, ?, 1, 'Published', '{"nodes":[{"key":"api"}],"edges":[]}', ?, now())
	`, versionID, planID, ids.userID)
	return versionID
}

func seedRuntimeRequest(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, workflowVersionID, planVersionID uuid.UUID) uuid.UUID {
	t.Helper()
	requestID, versionID := uuid.New(), uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_requests (id, organization_id, project_id, environment_id)
		VALUES (?, ?, ?, ?)
	`, requestID, ids.organizationID, ids.projectID, ids.environmentID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_versions (
			id, request_id, version_number, status, fingerprint,
			workflow_version_id, plan_version_id, title, source_snapshot, created_by
		) VALUES (?, ?, 1, 'Candidate', ?, ?, ?, 'Runtime request', '{}', ?)
	`, versionID, requestID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		workflowVersionID, planVersionID, ids.userID)
	return versionID
}

func runtimeService(t *testing.T, repository deployapp.WorkflowRuntimeRepository) *deployapp.WorkflowRuntimeService {
	t.Helper()
	service, err := deployapp.NewWorkflowRuntimeService(deployapp.WorkflowRuntimeServiceOptions{
		Repository: repository, Authorizer: runtimeIntegrationAuthorizer{},
		Assignments: runtimeIntegrationAssignments{}, Clock: runtimeIntegrationClock{},
	})
	if err != nil {
		t.Fatalf("create workflow runtime service: %v", err)
	}
	return service
}

func approveRuntimeReview(t *testing.T, ctx context.Context, service *deployapp.WorkflowRuntimeService, snapshot deployapp.WorkflowRuntimeSnapshot, reviewerID uuid.UUID) {
	t.Helper()
	task, err := service.DecideReview(ctx, deployapp.WorkflowPrincipal{UserID: reviewerID}, deployapp.WorkflowReviewInput{
		RequestVersionID: snapshot.RequestVersionID, ReviewTaskID: snapshot.CurrentReview.ID,
		ExpectedLock: snapshot.Instance.LockVersion, Decision: deploydomain.ReviewDecisionApprove,
		IdempotencyKey: "runtime-review", RequestID: "runtime-test",
	})
	if err != nil || task.Status != deploydomain.ReviewTaskApproved {
		t.Fatalf("approve workflow review: %#v %v", task, err)
	}
}

func transitionRuntimeToDeployment(t *testing.T, ctx context.Context, service *deployapp.WorkflowRuntimeService, snapshot deployapp.WorkflowRuntimeSnapshot, actorID uuid.UUID) {
	t.Helper()
	result, err := service.Transition(ctx, deployapp.WorkflowPrincipal{UserID: actorID}, deployapp.WorkflowTransitionInput{
		RequestVersionID: snapshot.RequestVersionID, ExpectedLock: snapshot.Instance.LockVersion,
		TransitionKey: "deploy", Trigger: deploydomain.WorkflowTriggerReviewSatisfied,
		Facts:          map[deploydomain.WorkflowFact]string{deploydomain.WorkflowFactReviewStatus: "Approved"},
		IdempotencyKey: "runtime-deploy", RequestID: "runtime-test",
	})
	if err != nil || !result.DeploymentIntent {
		t.Fatalf("transition workflow to deployment: %#v %v", result, err)
	}
}

func runtimeIntegrationDocument(reviewerID uuid.UUID) deploydomain.WorkflowDocument {
	policy := deploydomain.ReviewPolicy{
		Type: deploydomain.ReviewPolicyAny, RequiredApprovals: 1, UserIDs: []uuid.UUID{reviewerID},
	}
	condition := deploydomain.WorkflowCondition{
		Fact:     deploydomain.WorkflowFactReviewStatus,
		Operator: deploydomain.WorkflowOperatorEquals, Values: []string{"Approved"},
	}
	return deploydomain.WorkflowDocument{
		InitialState: "review",
		States: []deploydomain.WorkflowState{
			{Key: "review", Name: "Review", Type: deploydomain.WorkflowStateReview, ReviewPolicy: &policy},
			{Key: "deploying", Name: "Deploying", Type: deploydomain.WorkflowStateDeployment},
		},
		Transitions: []deploydomain.WorkflowTransition{{
			Key: "deploy", From: "review", To: "deploying",
			Trigger:    deploydomain.WorkflowTriggerReviewSatisfied,
			Permission: "deployment_request.deploy", Conditions: []deploydomain.WorkflowCondition{condition},
		}},
	}
}

type runtimeIntegrationAuthorizer struct{}

func (runtimeIntegrationAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return true, nil
}

type runtimeIntegrationAssignments struct{}

func (runtimeIntegrationAssignments) Resolve(_ context.Context, policy deploydomain.ReviewPolicy, _ authz.Scope) (deploydomain.ReviewAssignment, error) {
	return deploydomain.ReviewAssignment{UserIDs: policy.UserIDs, RoleMembers: map[uuid.UUID][]uuid.UUID{}}, nil
}

type runtimeIntegrationClock struct{}

func (runtimeIntegrationClock) Now() time.Time { return runtimeIntegrationTime() }

func runtimeIntegrationTime() time.Time {
	return time.Date(2026, 9, 3, 4, 5, 6, 0, time.UTC)
}
