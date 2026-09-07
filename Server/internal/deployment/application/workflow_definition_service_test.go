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

func TestWorkflowDefinitionLifecycle(t *testing.T) {
	repository := &workflowDefinitionRepositoryStub{}
	service := mustWorkflowDefinitionService(t, repository, true)
	principal := WorkflowPrincipal{UserID: uuid.New()}
	created, err := service.Create(context.Background(), principal, CreateWorkflowInput{
		Name: "Production approval", Document: definitionTestDocument(), RequestID: "create-workflow",
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	first, err := service.ChangeLifecycle(context.Background(), principal, ChangeWorkflowLifecycleInput{
		WorkflowID: created.ID, VersionID: created.Versions[0].ID, ExpectedVersion: 1,
		Lifecycle: deploydomain.DefinitionPublished, RequestID: "publish-first-version",
	})
	if err != nil || first.Lifecycle != deploydomain.DefinitionPublished {
		t.Fatalf("publish first version: %#v %v", first, err)
	}
	second, err := service.CreateVersion(context.Background(), principal, CreateWorkflowVersionInput{
		WorkflowID: created.ID, ExpectedVersion: 1,
		Document: definitionTestDocument(), RequestID: "create-version",
	})
	if err != nil || second.VersionNumber != 2 || second.Lifecycle != deploydomain.DefinitionDraft {
		t.Fatalf("create version: %#v %v", second, err)
	}
	published, err := service.ChangeLifecycle(context.Background(), principal, ChangeWorkflowLifecycleInput{
		WorkflowID: created.ID, VersionID: second.ID, ExpectedVersion: 1,
		Lifecycle: deploydomain.DefinitionPublished, RequestID: "publish-version",
	})
	if err != nil || published.Lifecycle != deploydomain.DefinitionPublished || published.LockVersion != 2 {
		t.Fatalf("publish version: %#v %v", published, err)
	}
}

func TestWorkflowManagementRequiresPlatformPermission(t *testing.T) {
	service := mustWorkflowDefinitionService(t, &workflowDefinitionRepositoryStub{}, false)
	_, err := service.Create(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, CreateWorkflowInput{
		Name: "Denied", Document: definitionTestDocument(),
	})
	if !errors.Is(err, ErrWorkflowForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestWorkflowVersionUsesPinnedDocumentCopy(t *testing.T) {
	repository := &workflowDefinitionRepositoryStub{}
	service := mustWorkflowDefinitionService(t, repository, true)
	document := definitionTestDocument()
	created, err := service.Create(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, CreateWorkflowInput{
		Name: "Immutable", Document: document,
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	document.States[0].Name = "Changed outside aggregate"
	if created.Versions[0].Document.States[0].Name != "Start" {
		t.Fatal("workflow version document changed through caller-owned slice")
	}
}

func mustWorkflowDefinitionService(t *testing.T, repository WorkflowDefinitionRepository, allowed bool) *WorkflowDefinitionService {
	t.Helper()
	service, err := NewWorkflowDefinitionService(repository, workflowAuthorizerStub{allowed: allowed}, workflowClockStub{})
	if err != nil {
		t.Fatalf("create workflow service: %v", err)
	}
	return service
}

func definitionTestDocument() deploydomain.WorkflowDocument {
	return deploydomain.WorkflowDocument{
		InitialState: "start",
		States: []deploydomain.WorkflowState{
			{Key: "start", Name: "Start", Type: deploydomain.WorkflowStateStart},
			{Key: "done", Name: "Done", Type: deploydomain.WorkflowStateTerminal},
		},
		Transitions: []deploydomain.WorkflowTransition{{
			Key: "finish", From: "start", To: "done",
			Trigger: deploydomain.WorkflowTriggerManual, Permission: "deployment_request.update",
		}},
	}
}

type workflowDefinitionRepositoryStub struct {
	workflows []deploydomain.ReleaseWorkflow
}

func (s *workflowDefinitionRepositoryStub) List(context.Context) ([]deploydomain.ReleaseWorkflow, error) {
	return s.workflows, nil
}

func (s *workflowDefinitionRepositoryStub) Load(_ context.Context, workflowID uuid.UUID) (deploydomain.ReleaseWorkflow, error) {
	for _, workflow := range s.workflows {
		if workflow.ID == workflowID {
			return workflow, nil
		}
	}
	return deploydomain.ReleaseWorkflow{}, ErrWorkflowNotFound
}

func (s *workflowDefinitionRepositoryStub) Create(_ context.Context, _ WorkflowMutation, workflow deploydomain.ReleaseWorkflow) error {
	s.workflows = append(s.workflows, workflow)
	return nil
}

func (s *workflowDefinitionRepositoryStub) AppendVersion(_ context.Context, _ WorkflowMutation, expected uint64, version deploydomain.ReleaseWorkflowVersion) error {
	for index := range s.workflows {
		if s.workflows[index].ID == version.WorkflowID && uint64(len(s.workflows[index].Versions)) == expected {
			s.workflows[index].Versions = append(s.workflows[index].Versions, version)
			return nil
		}
	}
	return ErrWorkflowConflict
}

func (s *workflowDefinitionRepositoryStub) UpdateLifecycle(_ context.Context, _ WorkflowMutation, expected uint64, version deploydomain.ReleaseWorkflowVersion) error {
	for workflowIndex := range s.workflows {
		for versionIndex := range s.workflows[workflowIndex].Versions {
			current := s.workflows[workflowIndex].Versions[versionIndex]
			if current.ID == version.ID && current.LockVersion == expected {
				s.workflows[workflowIndex].Versions[versionIndex] = version
				return nil
			}
		}
	}
	return ErrWorkflowConflict
}

type workflowAuthorizerStub struct{ allowed bool }

func (s workflowAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return s.allowed && request.Scope.Kind == authz.ScopePlatform, nil
}

type workflowClockStub struct{}

func (workflowClockStub) Now() time.Time {
	return time.Date(2026, 9, 3, 2, 3, 4, 0, time.UTC)
}
