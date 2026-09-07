package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestPlanDefinitionRejectsCycleWhenPublishing(t *testing.T) {
	projectID := uuid.New()
	plan := planAggregateFixture(t, projectID, cyclicPlanDocument())
	repository := &planRepositoryStub{plan: plan}
	service := planServiceFixture(t, repository, true, projectID)
	_, err := service.ChangeLifecycle(context.Background(), PlanPrincipal{UserID: uuid.New()}, ChangePlanLifecycleInput{
		PlanID: plan.ID, VersionID: plan.Versions[0].ID,
		ExpectedVersion: 1, Lifecycle: deploydomain.DefinitionPublished,
	})
	if !errors.Is(err, ErrPlanInvalid) || repository.updated.ID != uuid.Nil {
		t.Fatalf("publish cycle error = %v", err)
	}
}

func TestPlanDefinitionUsesOwnerScopeAuthorization(t *testing.T) {
	projectID := uuid.New()
	repository := &planRepositoryStub{}
	service := planServiceFixture(t, repository, false, projectID)
	_, err := service.Create(context.Background(), PlanPrincipal{UserID: uuid.New()}, CreatePlanInput{
		OwnerKind: deploydomain.DeploymentPlanOwnerProject, OwnerProjectID: &projectID,
		Name: "Production", Document: planDocumentFixture(),
	})
	if !errors.Is(err, ErrPlanForbidden) || repository.created.ID != uuid.Nil {
		t.Fatalf("owner authorization error = %v", err)
	}
}

func planServiceFixture(t *testing.T, repository DeploymentPlanRepository, allowed bool, projectID uuid.UUID) *PlanDefinitionService {
	t.Helper()
	service, err := NewPlanDefinitionService(PlanDefinitionServiceOptions{
		Repository: repository, Authorizer: planAuthorizerStub{allowed: allowed},
		Scopes: planScopeResolverStub{projectID: projectID}, Clock: planClockStub{},
	})
	if err != nil {
		t.Fatalf("create plan service: %v", err)
	}
	return service
}

func planAggregateFixture(t *testing.T, projectID uuid.UUID, document deploydomain.DeploymentPlanDocument) deploydomain.DeploymentPlan {
	t.Helper()
	plan, err := deploydomain.NewDeploymentPlan(deploydomain.DeploymentPlan{
		ID: uuid.New(), OwnerKind: deploydomain.DeploymentPlanOwnerProject,
		OwnerProjectID: &projectID, Name: "Production", CreatedBy: uuid.New(), CreatedAt: planClockStub{}.Now(),
	}, document)
	if err != nil {
		t.Fatalf("create plan aggregate: %v", err)
	}
	return plan
}

func planDocumentFixture() deploydomain.DeploymentPlanDocument {
	return deploydomain.DeploymentPlanDocument{Nodes: []deploydomain.DeploymentPlanNode{{
		Key: "api", ApplicationKey: "api", SuccessCondition: deploydomain.DeploymentPlanCondition{
			SyncStatuses: []string{"Synced"}, HealthStatuses: []string{"Healthy"},
		},
	}}, Edges: []deploydomain.DeploymentPlanEdge{}}
}

func cyclicPlanDocument() deploydomain.DeploymentPlanDocument {
	document := planDocumentFixture()
	document.Nodes = append(document.Nodes, deploydomain.DeploymentPlanNode{
		Key: "worker", ApplicationKey: "worker",
		SuccessCondition: deploydomain.DeploymentPlanCondition{SyncStatuses: []string{"Synced"}, HealthStatuses: []string{"Healthy"}},
	})
	document.Edges = []deploydomain.DeploymentPlanEdge{
		{From: "api", To: "worker", Condition: deploydomain.PlanEdgeUpstreamSucceeded},
		{From: "worker", To: "api", Condition: deploydomain.PlanEdgeUpstreamSucceeded},
	}
	return document
}

type planRepositoryStub struct {
	plan    deploydomain.DeploymentPlan
	created deploydomain.DeploymentPlan
	updated deploydomain.DeploymentPlanVersion
}

func (s *planRepositoryStub) List(context.Context, uuid.UUID) ([]deploydomain.DeploymentPlan, error) {
	return []deploydomain.DeploymentPlan{s.plan}, nil
}

func (s *planRepositoryStub) Load(context.Context, uuid.UUID) (deploydomain.DeploymentPlan, error) {
	return s.plan, nil
}

func (s *planRepositoryStub) Create(_ context.Context, _ PlanMutation, value deploydomain.DeploymentPlan) error {
	s.created = value
	return nil
}

func (*planRepositoryStub) AppendVersion(context.Context, PlanMutation, uint64, deploydomain.DeploymentPlanVersion) error {
	return nil
}

func (s *planRepositoryStub) UpdateLifecycle(_ context.Context, _ PlanMutation, _ uint64, value deploydomain.DeploymentPlanVersion) error {
	s.updated = value
	return nil
}

type planAuthorizerStub struct{ allowed bool }

func (s planAuthorizerStub) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return s.allowed, nil
}

type planScopeResolverStub struct{ projectID uuid.UUID }

func (s planScopeResolverStub) ResolveProject(_ context.Context, projectID uuid.UUID) (authz.Scope, error) {
	if projectID != s.projectID {
		return authz.Scope{}, ErrPlanNotFound
	}
	return authz.NewProjectScope(uuid.New(), projectID)
}

type planClockStub struct{}

func (planClockStub) Now() time.Time { return time.Date(2026, 9, 3, 8, 9, 10, 0, time.UTC) }
