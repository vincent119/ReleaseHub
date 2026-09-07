package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	// ErrWorkflowForbidden hides workflow management resources from unauthorized users.
	ErrWorkflowForbidden = errors.New("workflow operation is not allowed")
	// ErrWorkflowNotFound identifies an unavailable workflow or version.
	ErrWorkflowNotFound = errors.New("workflow was not found")
	// ErrWorkflowConflict identifies stale versions and duplicate drafts.
	ErrWorkflowConflict = errors.New("workflow changed concurrently")
	// ErrWorkflowInvalid identifies a workflow document or lifecycle rejected by domain rules.
	ErrWorkflowInvalid = errors.New("workflow definition is invalid")
)

// WorkflowPrincipal is the authenticated identity used by workflow operations.
type WorkflowPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// WorkflowMutation carries immutable audit correlation data.
type WorkflowMutation struct {
	ActorID    uuid.UUID
	RequestID  string
	OccurredAt time.Time
}

// WorkflowDefinitionRepository persists workflow aggregates atomically.
type WorkflowDefinitionRepository interface {
	List(context.Context) ([]deploydomain.ReleaseWorkflow, error)
	Load(context.Context, uuid.UUID) (deploydomain.ReleaseWorkflow, error)
	Create(context.Context, WorkflowMutation, deploydomain.ReleaseWorkflow) error
	AppendVersion(context.Context, WorkflowMutation, uint64, deploydomain.ReleaseWorkflowVersion) error
	UpdateLifecycle(context.Context, WorkflowMutation, uint64, deploydomain.ReleaseWorkflowVersion) error
}

// WorkflowAuthorizer evaluates sensitive workflow permissions against fresh policy.
type WorkflowAuthorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// WorkflowClock provides deterministic workflow timestamps.
type WorkflowClock interface{ Now() time.Time }

// SystemWorkflowClock returns UTC wall-clock time for runtime composition.
type SystemWorkflowClock struct{}

// Now returns the current UTC time.
func (SystemWorkflowClock) Now() time.Time { return time.Now().UTC() }

// WorkflowDefinitionService manages immutable release workflow versions.
type WorkflowDefinitionService struct {
	repository WorkflowDefinitionRepository
	authorizer WorkflowAuthorizer
	clock      WorkflowClock
	manage     authz.Permission
}

// NewWorkflowDefinitionService creates the workflow definition use case.
func NewWorkflowDefinitionService(repository WorkflowDefinitionRepository, authorizer WorkflowAuthorizer, clock WorkflowClock) (*WorkflowDefinitionService, error) {
	if repository == nil || authorizer == nil || clock == nil {
		return nil, errors.New("workflow repository, authorizer, and clock are required")
	}
	permission, _ := authz.NewPermission("workflow.manage")
	return &WorkflowDefinitionService{repository: repository, authorizer: authorizer, clock: clock, manage: permission}, nil
}

// List returns workflow definitions to an active authenticated user.
func (s *WorkflowDefinitionService) List(ctx context.Context, principal WorkflowPrincipal) ([]deploydomain.ReleaseWorkflow, error) {
	if principal.UserID == uuid.Nil || principal.Disabled {
		return nil, ErrWorkflowForbidden
	}
	return s.repository.List(ctx)
}

// Create stores a workflow and its first draft version.
func (s *WorkflowDefinitionService) Create(ctx context.Context, principal WorkflowPrincipal, input CreateWorkflowInput) (deploydomain.ReleaseWorkflow, error) {
	if err := s.authorizeManage(ctx, principal); err != nil {
		return deploydomain.ReleaseWorkflow{}, err
	}
	now := s.clock.Now()
	workflow, err := deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: input.Name, Description: input.Description,
		CreatedBy: principal.UserID, CreatedAt: now,
	}, input.Document)
	if err != nil {
		return deploydomain.ReleaseWorkflow{}, fmt.Errorf("%w: %v", ErrWorkflowInvalid, err)
	}
	mutation := workflowMutation(principal.UserID, input.RequestID, now)
	if err := s.repository.Create(ctx, mutation, workflow); err != nil {
		return deploydomain.ReleaseWorkflow{}, err
	}
	return workflow, nil
}

// CreateVersion appends the next draft without changing existing versions.
func (s *WorkflowDefinitionService) CreateVersion(ctx context.Context, principal WorkflowPrincipal, input CreateWorkflowVersionInput) (deploydomain.ReleaseWorkflowVersion, error) {
	if err := s.authorizeManage(ctx, principal); err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	workflow, err := s.repository.Load(ctx, input.WorkflowID)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	version, err := s.newVersion(workflow, principal.UserID, input)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	mutation := workflowMutation(principal.UserID, input.RequestID, s.clock.Now())
	if err := s.repository.AppendVersion(ctx, mutation, input.ExpectedVersion, version); err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	return version, nil
}

// ChangeLifecycle publishes a draft or disables a published workflow version.
func (s *WorkflowDefinitionService) ChangeLifecycle(ctx context.Context, principal WorkflowPrincipal, input ChangeWorkflowLifecycleInput) (deploydomain.ReleaseWorkflowVersion, error) {
	if err := s.authorizeManage(ctx, principal); err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	workflow, err := s.repository.Load(ctx, input.WorkflowID)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	updated, err := changeWorkflowLifecycle(workflow, input, s.clock.Now())
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	mutation := workflowMutation(principal.UserID, input.RequestID, s.clock.Now())
	if err := s.repository.UpdateLifecycle(ctx, mutation, input.ExpectedVersion, updated); err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	return updated, nil
}

func changeWorkflowLifecycle(workflow deploydomain.ReleaseWorkflow, input ChangeWorkflowLifecycleInput, now time.Time) (deploydomain.ReleaseWorkflowVersion, error) {
	version, err := findWorkflowVersion(workflow, input.VersionID)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	if version.LockVersion != input.ExpectedVersion {
		return deploydomain.ReleaseWorkflowVersion{}, ErrWorkflowConflict
	}
	updated, err := version.ChangeLifecycle(input.Lifecycle, input.ExpectedVersion, now)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, fmt.Errorf("%w: %v", ErrWorkflowInvalid, err)
	}
	return updated, nil
}

func (s *WorkflowDefinitionService) authorizeManage(ctx context.Context, principal WorkflowPrincipal) error {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled,
		Permission: s.manage, Scope: authz.NewPlatformScope(),
	})
	if err != nil {
		return fmt.Errorf("authorize workflow management: %w", err)
	}
	if !allowed {
		return ErrWorkflowForbidden
	}
	return nil
}

func (s *WorkflowDefinitionService) newVersion(workflow deploydomain.ReleaseWorkflow, actorID uuid.UUID, input CreateWorkflowVersionInput) (deploydomain.ReleaseWorkflowVersion, error) {
	latest, ok := latestWorkflowVersion(workflow)
	if !ok || latest.VersionNumber != input.ExpectedVersion || latest.Lifecycle == deploydomain.DefinitionDraft {
		return deploydomain.ReleaseWorkflowVersion{}, ErrWorkflowConflict
	}
	version, err := deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: workflow.ID, VersionNumber: latest.VersionNumber + 1, ActorID: actorID,
		Document: input.Document, CreatedAt: s.clock.Now(),
	})
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, fmt.Errorf("%w: %v", ErrWorkflowInvalid, err)
	}
	return version, nil
}

func latestWorkflowVersion(workflow deploydomain.ReleaseWorkflow) (deploydomain.ReleaseWorkflowVersion, bool) {
	var latest deploydomain.ReleaseWorkflowVersion
	for _, version := range workflow.Versions {
		if version.VersionNumber > latest.VersionNumber {
			latest = version
		}
	}
	return latest, latest.VersionNumber > 0
}

func findWorkflowVersion(workflow deploydomain.ReleaseWorkflow, versionID uuid.UUID) (deploydomain.ReleaseWorkflowVersion, error) {
	for _, version := range workflow.Versions {
		if version.ID == versionID {
			return version, nil
		}
	}
	return deploydomain.ReleaseWorkflowVersion{}, ErrWorkflowNotFound
}

func workflowMutation(actorID uuid.UUID, requestID string, now time.Time) WorkflowMutation {
	return WorkflowMutation{ActorID: actorID, RequestID: requestID, OccurredAt: now.UTC()}
}

// CreateWorkflowInput contains one workflow creation command.
type CreateWorkflowInput struct {
	Name        string
	Description string
	Document    deploydomain.WorkflowDocument
	RequestID   string
}

// CreateWorkflowVersionInput contains one immutable draft append command.
type CreateWorkflowVersionInput struct {
	WorkflowID      uuid.UUID
	ExpectedVersion uint64
	Document        deploydomain.WorkflowDocument
	RequestID       string
}

// ChangeWorkflowLifecycleInput contains one optimistic lifecycle command.
type ChangeWorkflowLifecycleInput struct {
	WorkflowID      uuid.UUID
	VersionID       uuid.UUID
	ExpectedVersion uint64
	Lifecycle       deploydomain.DefinitionLifecycle
	RequestID       string
}
