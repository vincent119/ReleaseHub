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
	// ErrBindingForbidden hides an Environment binding from unauthorized callers.
	ErrBindingForbidden = errors.New("deployment binding operation is not allowed")
	// ErrBindingNotFound identifies a non-canonical Environment scope.
	ErrBindingNotFound = errors.New("deployment binding scope was not found")
	// ErrBindingConflict identifies stale versions or non-published definitions.
	ErrBindingConflict = errors.New("deployment binding change was rejected")
)

// DeploymentBindingRepository persists current Environment bindings atomically.
type DeploymentBindingRepository interface {
	ResolveEnvironment(context.Context, BindingScopeInput) (authz.Scope, error)
	Get(context.Context, authz.Scope) (*deploydomain.DeploymentBinding, error)
	Bind(context.Context, BindingMutation, authz.Scope, BindDefinitionsInput) (deploydomain.DeploymentBinding, error)
}

// BindingMutation carries immutable audit correlation data.
type BindingMutation struct {
	ActorID    uuid.UUID
	RequestID  string
	OccurredAt time.Time
}

// BindingScopeInput identifies one canonical Environment hierarchy.
type BindingScopeInput struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
}

// BindDefinitionsInput selects published definitions with optimistic locking.
type BindDefinitionsInput struct {
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	ExpectedVersion   uint64
	RequestID         string
}

// DeploymentBindingService manages one current binding per Environment.
type DeploymentBindingService struct {
	repository DeploymentBindingRepository
	authorizer WorkflowAuthorizer
	clock      WorkflowClock
	manage     authz.Permission
}

// NewDeploymentBindingService creates the binding use case.
func NewDeploymentBindingService(repository DeploymentBindingRepository, authorizer WorkflowAuthorizer, clock WorkflowClock) (*DeploymentBindingService, error) {
	if repository == nil || authorizer == nil || clock == nil {
		return nil, errors.New("deployment binding dependencies are required")
	}
	permission, _ := authz.NewPermission("deployment_plan.manage")
	return &DeploymentBindingService{repository: repository, authorizer: authorizer, clock: clock, manage: permission}, nil
}

// Get returns the current binding or nil when the Environment is unbound.
func (s *DeploymentBindingService) Get(ctx context.Context, principal PlanPrincipal, input BindingScopeInput) (*deploydomain.DeploymentBinding, error) {
	scope, err := s.authorizedScope(ctx, principal, input)
	if err != nil {
		return nil, err
	}
	return s.repository.Get(ctx, scope)
}

// Bind creates or switches one Environment binding.
func (s *DeploymentBindingService) Bind(ctx context.Context, principal PlanPrincipal, input BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	scope, err := s.authorizedScope(ctx, principal, BindingScopeInput{
		OrganizationID: input.OrganizationID, ProjectID: input.ProjectID, EnvironmentID: input.EnvironmentID,
	})
	if err != nil {
		return deploydomain.DeploymentBinding{}, err
	}
	mutation := BindingMutation{ActorID: principal.UserID, RequestID: input.RequestID, OccurredAt: s.clock.Now()}
	return s.repository.Bind(ctx, mutation, scope, input)
}

func (s *DeploymentBindingService) authorizedScope(ctx context.Context, principal PlanPrincipal, input BindingScopeInput) (authz.Scope, error) {
	scope, err := s.repository.ResolveEnvironment(ctx, input)
	if err != nil {
		return authz.Scope{}, err
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.manage, Scope: scope,
	})
	if err != nil {
		return authz.Scope{}, fmt.Errorf("authorize deployment binding management: %w", err)
	}
	if !allowed {
		return authz.Scope{}, ErrBindingForbidden
	}
	return scope, nil
}
