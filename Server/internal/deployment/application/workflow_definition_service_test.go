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

func TestCreateWorkflowVersionDistinguishesStaleVersionAndExistingDraft(t *testing.T) {
	repository := &workflowDefinitionRepositoryStub{}
	service := mustWorkflowDefinitionService(t, repository, true)
	principal := WorkflowPrincipal{UserID: uuid.New()}
	created, err := service.Create(context.Background(), principal, CreateWorkflowInput{
		Name: "Production approval", Document: definitionTestDocument(),
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	input := CreateWorkflowVersionInput{
		WorkflowID: created.ID, ExpectedVersion: 1, Document: definitionTestDocument(),
	}
	if _, err := service.CreateVersion(context.Background(), principal, input); !errors.Is(err, ErrWorkflowDraftExists) {
		t.Fatalf("existing draft error = %v", err)
	}

	input.ExpectedVersion = 2
	if _, err := service.CreateVersion(context.Background(), principal, input); !errors.Is(err, ErrWorkflowVersionConflict) {
		t.Fatalf("stale workflow version error = %v", err)
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

func TestWorkflowReviewOptionsRequireWorkflowManagementPermission(t *testing.T) {
	userID, roleID := uuid.New(), uuid.New()
	repository := &workflowDefinitionRepositoryStub{reviewOptions: WorkflowReviewOptions{
		Users: []WorkflowReviewUserOption{{ID: userID, Username: "reviewer", Assignable: true}},
		Roles: []WorkflowReviewRoleOption{{ID: roleID, Name: "release_approver", OwnerKind: "platform", Assignable: true}},
	}}
	allowed := mustWorkflowDefinitionService(t, repository, true)
	options, err := allowed.ReviewOptions(context.Background(), WorkflowPrincipal{UserID: uuid.New()})
	if err != nil || len(options.Users) != 1 || options.Users[0].ID != userID || len(options.Roles) != 1 || options.Roles[0].ID != roleID {
		t.Fatalf("review options = %#v, %v", options, err)
	}

	denied := mustWorkflowDefinitionService(t, repository, false)
	_, err = denied.ReviewOptions(context.Background(), WorkflowPrincipal{UserID: uuid.New()})
	if !errors.Is(err, ErrWorkflowForbidden) {
		t.Fatalf("denied review options error = %v", err)
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

func TestDeleteWorkflowRequiresPermissionAndDelegatesOptimisticVersion(t *testing.T) {
	workflowID := uuid.New()
	repository := &workflowDefinitionRepositoryStub{}
	principal := WorkflowPrincipal{UserID: uuid.New()}
	service := mustWorkflowDefinitionService(t, repository, true)
	if err := service.Delete(context.Background(), principal, DeleteWorkflowInput{
		WorkflowID: workflowID, ExpectedVersion: 3, RequestID: "delete-workflow",
	}); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	if repository.deletedWorkflowID != workflowID || repository.deletedExpectedVersion != 3 {
		t.Fatalf("delete input = %s version %d", repository.deletedWorkflowID, repository.deletedExpectedVersion)
	}
	if repository.deletedMutation.ActorID != principal.UserID || repository.deletedMutation.RequestID != "delete-workflow" {
		t.Fatalf("delete mutation = %#v", repository.deletedMutation)
	}

	deniedRepository := &workflowDefinitionRepositoryStub{}
	deniedService := mustWorkflowDefinitionService(t, deniedRepository, false)
	err := deniedService.Delete(context.Background(), principal, DeleteWorkflowInput{WorkflowID: workflowID, ExpectedVersion: 3})
	if !errors.Is(err, ErrWorkflowForbidden) || deniedRepository.deletedWorkflowID != uuid.Nil {
		t.Fatalf("denied delete = %v, repository called = %t", err, deniedRepository.deletedWorkflowID != uuid.Nil)
	}
}

func TestDeleteWorkflowRejectsInvalidOptimisticVersion(t *testing.T) {
	repository := &workflowDefinitionRepositoryStub{}
	service := mustWorkflowDefinitionService(t, repository, true)
	err := service.Delete(context.Background(), WorkflowPrincipal{UserID: uuid.New()}, DeleteWorkflowInput{WorkflowID: uuid.New()})
	if !errors.Is(err, ErrWorkflowInvalid) || repository.deletedWorkflowID != uuid.Nil {
		t.Fatalf("invalid delete = %v, repository called = %t", err, repository.deletedWorkflowID != uuid.Nil)
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
	workflows              []deploydomain.ReleaseWorkflow
	reviewOptions          WorkflowReviewOptions
	deletedWorkflowID      uuid.UUID
	deletedExpectedVersion uint64
	deletedMutation        WorkflowMutation
}

func (s *workflowDefinitionRepositoryStub) List(context.Context) ([]deploydomain.ReleaseWorkflow, error) {
	return s.workflows, nil
}

func (s *workflowDefinitionRepositoryStub) ListReviewOptions(context.Context) (WorkflowReviewOptions, error) {
	return s.reviewOptions, nil
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

func (s *workflowDefinitionRepositoryStub) DeleteUnusedDraft(_ context.Context, mutation WorkflowMutation, workflowID uuid.UUID, expected uint64) error {
	s.deletedMutation = mutation
	s.deletedWorkflowID = workflowID
	s.deletedExpectedVersion = expected
	return nil
}

type workflowAuthorizerStub struct{ allowed bool }

func (s workflowAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return s.allowed && request.Scope.Kind == authz.ScopePlatform, nil
}

type workflowClockStub struct{}

func (workflowClockStub) Now() time.Time {
	return time.Date(2026, 9, 3, 2, 3, 4, 0, time.UTC)
}
