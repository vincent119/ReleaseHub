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
	// ErrPlanForbidden hides Plans from unauthorized callers.
	ErrPlanForbidden = errors.New("deployment plan operation is not allowed")
	// ErrPlanNotFound identifies an unavailable Plan or Version.
	ErrPlanNotFound = errors.New("deployment plan was not found")
	// ErrPlanConflict identifies stale commands and duplicate Drafts.
	ErrPlanConflict = errors.New("deployment plan changed concurrently")
	// ErrPlanInvalid identifies a rejected Plan graph or lifecycle command.
	ErrPlanInvalid = errors.New("deployment plan is invalid")
)

// PlanPrincipal is the authenticated identity for Plan operations.
type PlanPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// PlanMutation carries immutable audit correlation data.
type PlanMutation struct {
	ActorID    uuid.UUID
	RequestID  string
	OccurredAt time.Time
}

// DeploymentPlanRepository persists Plan aggregates atomically.
type DeploymentPlanRepository interface {
	List(context.Context, uuid.UUID) ([]deploydomain.DeploymentPlan, error)
	Load(context.Context, uuid.UUID) (deploydomain.DeploymentPlan, error)
	Create(context.Context, PlanMutation, deploydomain.DeploymentPlan) error
	AppendVersion(context.Context, PlanMutation, uint64, deploydomain.DeploymentPlanVersion) error
	UpdateLifecycle(context.Context, PlanMutation, uint64, deploydomain.DeploymentPlanVersion) error
}

// PlanScopeResolver resolves Project ownership without trusting caller input.
type PlanScopeResolver interface {
	ResolveProject(context.Context, uuid.UUID) (authz.Scope, error)
}

// PlanDefinitionService manages immutable Deployment Plan Versions.
type PlanDefinitionService struct {
	repository DeploymentPlanRepository
	authorizer WorkflowAuthorizer
	scopes     PlanScopeResolver
	clock      WorkflowClock
	manage     authz.Permission
}

// PlanDefinitionServiceOptions groups Plan use-case dependencies.
type PlanDefinitionServiceOptions struct {
	Repository DeploymentPlanRepository
	Authorizer WorkflowAuthorizer
	Scopes     PlanScopeResolver
	Clock      WorkflowClock
}

// NewPlanDefinitionService creates the Plan definition use case.
func NewPlanDefinitionService(options PlanDefinitionServiceOptions) (*PlanDefinitionService, error) {
	if options.Repository == nil || options.Authorizer == nil || options.Scopes == nil || options.Clock == nil {
		return nil, errors.New("deployment plan dependencies are required")
	}
	permission, _ := authz.NewPermission("deployment_plan.manage")
	return &PlanDefinitionService{
		repository: options.Repository, authorizer: options.Authorizer,
		scopes: options.Scopes, clock: options.Clock, manage: permission,
	}, nil
}

// List returns platform and Project Plans visible through management permission.
func (s *PlanDefinitionService) List(ctx context.Context, principal PlanPrincipal, projectID uuid.UUID) ([]deploydomain.DeploymentPlan, error) {
	scope, err := s.authorizeProject(ctx, principal, projectID)
	if err != nil {
		return nil, err
	}
	return s.repository.List(ctx, scope.ProjectID)
}

// Create stores a Plan and its first Draft Version.
func (s *PlanDefinitionService) Create(ctx context.Context, principal PlanPrincipal, input CreatePlanInput) (deploydomain.DeploymentPlan, error) {
	if err := s.authorizeOwner(ctx, principal, input.OwnerKind, input.OwnerProjectID); err != nil {
		return deploydomain.DeploymentPlan{}, err
	}
	now := s.clock.Now()
	plan, err := deploydomain.NewDeploymentPlan(deploydomain.DeploymentPlan{
		ID: uuid.New(), OwnerKind: input.OwnerKind, OwnerProjectID: input.OwnerProjectID,
		Name: input.Name, Description: input.Description, CreatedBy: principal.UserID, CreatedAt: now,
	}, input.Document)
	if err != nil {
		return deploydomain.DeploymentPlan{}, planInvalid(err)
	}
	if err := s.repository.Create(ctx, planMutation(principal.UserID, input.RequestID, now), plan); err != nil {
		return deploydomain.DeploymentPlan{}, err
	}
	return plan, nil
}

// CreateVersion appends the next immutable Draft Version.
func (s *PlanDefinitionService) CreateVersion(ctx context.Context, principal PlanPrincipal, input CreatePlanVersionInput) (deploydomain.DeploymentPlanVersion, error) {
	plan, err := s.authorizedPlan(ctx, principal, input.PlanID)
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	version, err := s.newVersion(plan, principal.UserID, input)
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	mutation := planMutation(principal.UserID, input.RequestID, s.clock.Now())
	if err := s.repository.AppendVersion(ctx, mutation, input.ExpectedVersion, version); err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	return version, nil
}

// ChangeLifecycle publishes an executable Draft or disables a Published Version.
func (s *PlanDefinitionService) ChangeLifecycle(ctx context.Context, principal PlanPrincipal, input ChangePlanLifecycleInput) (deploydomain.DeploymentPlanVersion, error) {
	plan, err := s.authorizedPlan(ctx, principal, input.PlanID)
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	version, err := findPlanVersion(plan, input.VersionID)
	if err != nil || version.LockVersion != input.ExpectedVersion {
		return deploydomain.DeploymentPlanVersion{}, planLifecycleLookupError(err)
	}
	updated, err := version.ChangeLifecycle(input.Lifecycle, input.ExpectedVersion, s.clock.Now())
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, planInvalid(err)
	}
	mutation := planMutation(principal.UserID, input.RequestID, s.clock.Now())
	if err := s.repository.UpdateLifecycle(ctx, mutation, input.ExpectedVersion, updated); err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	return updated, nil
}

func (s *PlanDefinitionService) authorizedPlan(ctx context.Context, principal PlanPrincipal, planID uuid.UUID) (deploydomain.DeploymentPlan, error) {
	plan, err := s.repository.Load(ctx, planID)
	if err != nil {
		return deploydomain.DeploymentPlan{}, err
	}
	if err := s.authorizeOwner(ctx, principal, plan.OwnerKind, plan.OwnerProjectID); err != nil {
		return deploydomain.DeploymentPlan{}, err
	}
	return plan, nil
}

func (s *PlanDefinitionService) authorizeOwner(ctx context.Context, principal PlanPrincipal, kind deploydomain.DeploymentPlanOwnerKind, projectID *uuid.UUID) error {
	if kind == deploydomain.DeploymentPlanOwnerPlatform {
		return s.authorize(ctx, principal, authz.NewPlatformScope())
	}
	if projectID == nil {
		return ErrPlanInvalid
	}
	_, err := s.authorizeProject(ctx, principal, *projectID)
	return err
}

func (s *PlanDefinitionService) authorizeProject(ctx context.Context, principal PlanPrincipal, projectID uuid.UUID) (authz.Scope, error) {
	if projectID == uuid.Nil {
		return authz.Scope{}, ErrPlanNotFound
	}
	scope, err := s.scopes.ResolveProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			return authz.Scope{}, ErrPlanNotFound
		}
		return authz.Scope{}, fmt.Errorf("resolve deployment plan Project scope: %w", err)
	}
	return scope, s.authorize(ctx, principal, scope)
}

func (s *PlanDefinitionService) authorize(ctx context.Context, principal PlanPrincipal, scope authz.Scope) error {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.manage, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize deployment plan management: %w", err)
	}
	if !allowed {
		return ErrPlanForbidden
	}
	return nil
}

func (s *PlanDefinitionService) newVersion(plan deploydomain.DeploymentPlan, actorID uuid.UUID, input CreatePlanVersionInput) (deploydomain.DeploymentPlanVersion, error) {
	latest, ok := latestPlanVersion(plan)
	if !ok || latest.VersionNumber != input.ExpectedVersion || latest.Lifecycle == deploydomain.DefinitionDraft {
		return deploydomain.DeploymentPlanVersion{}, ErrPlanConflict
	}
	version, err := deploydomain.NewDeploymentPlanVersion(deploydomain.DeploymentPlanVersionDraft{
		PlanID: plan.ID, VersionNumber: latest.VersionNumber + 1, ActorID: actorID,
		Document: input.Document, CreatedAt: s.clock.Now(),
	})
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, planInvalid(err)
	}
	return version, nil
}

func latestPlanVersion(plan deploydomain.DeploymentPlan) (deploydomain.DeploymentPlanVersion, bool) {
	var latest deploydomain.DeploymentPlanVersion
	for _, version := range plan.Versions {
		if version.VersionNumber > latest.VersionNumber {
			latest = version
		}
	}
	return latest, latest.VersionNumber > 0
}

func findPlanVersion(plan deploydomain.DeploymentPlan, versionID uuid.UUID) (deploydomain.DeploymentPlanVersion, error) {
	for _, version := range plan.Versions {
		if version.ID == versionID {
			return version, nil
		}
	}
	return deploydomain.DeploymentPlanVersion{}, ErrPlanNotFound
}

func planLifecycleLookupError(err error) error {
	if err != nil {
		return err
	}
	return ErrPlanConflict
}

func planInvalid(err error) error { return fmt.Errorf("%w: %v", ErrPlanInvalid, err) }

func planMutation(actorID uuid.UUID, requestID string, now time.Time) PlanMutation {
	return PlanMutation{ActorID: actorID, RequestID: requestID, OccurredAt: now.UTC()}
}

// CreatePlanInput contains one Plan creation command.
type CreatePlanInput struct {
	OwnerKind      deploydomain.DeploymentPlanOwnerKind
	OwnerProjectID *uuid.UUID
	Name           string
	Description    string
	Document       deploydomain.DeploymentPlanDocument
	RequestID      string
}

// CreatePlanVersionInput contains one immutable Draft append command.
type CreatePlanVersionInput struct {
	PlanID          uuid.UUID
	ExpectedVersion uint64
	Document        deploydomain.DeploymentPlanDocument
	RequestID       string
}

// ChangePlanLifecycleInput contains one optimistic lifecycle command.
type ChangePlanLifecycleInput struct {
	PlanID          uuid.UUID
	VersionID       uuid.UUID
	ExpectedVersion uint64
	Lifecycle       deploydomain.DefinitionLifecycle
	RequestID       string
}
