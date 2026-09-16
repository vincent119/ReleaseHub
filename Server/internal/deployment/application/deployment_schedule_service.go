package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	ErrDeploymentScheduleNotFound = errors.New("deployment schedule scope was not found")
	ErrDeploymentScheduleConflict = errors.New("deployment schedule changed concurrently")
	ErrDeploymentScheduleInvalid  = errors.New("deployment schedule is invalid")
)

// DeploymentScheduleMutation carries one optimistic and idempotent policy command.
type DeploymentScheduleMutation struct {
	ActorID         uuid.UUID
	OrganizationID  uuid.UUID
	RequestID       string
	IdempotencyKey  string
	ExpectedVersion uint64
	OccurredAt      time.Time
}

// DeploymentScheduleRepository persists Environment policies atomically.
type DeploymentScheduleRepository interface {
	ResolveEnvironment(context.Context, uuid.UUID) (authz.Scope, error)
	Get(context.Context, uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error)
	Put(context.Context, DeploymentScheduleMutation, deploydomain.DeploymentSchedulePolicy) (deploydomain.DeploymentSchedulePolicy, error)
}

// DeploymentScheduleView combines the effective policy and current mutation capability.
type DeploymentScheduleView struct {
	Policy    deploydomain.DeploymentSchedulePolicy
	CanManage bool
}

// UpdateDeploymentScheduleInput contains one policy replacement command.
type UpdateDeploymentScheduleInput struct {
	EnvironmentID   uuid.UUID
	Enabled         bool
	TimeZone        string
	WeeklyWindows   []deploydomain.DeploymentScheduleWeeklyWindow
	Blackouts       []deploydomain.DeploymentScheduleBlackout
	ExpectedVersion uint64
	IdempotencyKey  string
	RequestID       string
}

// DeploymentScheduleService manages Environment deployment windows.
type DeploymentScheduleService struct {
	repository DeploymentScheduleRepository
	authorizer WorkflowAuthorizer
	clock      WorkflowClock
	view       authz.Permission
	manage     authz.Permission
}

// NewDeploymentScheduleService creates the schedule policy use case.
func NewDeploymentScheduleService(repository DeploymentScheduleRepository, authorizer WorkflowAuthorizer, clock WorkflowClock) (*DeploymentScheduleService, error) {
	if repository == nil || authorizer == nil || clock == nil {
		return nil, errors.New("deployment schedule dependencies are required")
	}
	view, _ := authz.NewPermission("deployment_request.view")
	manage, _ := authz.NewPermission("deployment_schedule.manage")
	return &DeploymentScheduleService{repository: repository, authorizer: authorizer, clock: clock, view: view, manage: manage}, nil
}

// Get returns the effective policy after current authorization is evaluated.
func (s *DeploymentScheduleService) Get(ctx context.Context, principal PlanPrincipal, environmentID uuid.UUID) (DeploymentScheduleView, error) {
	scope, err := s.repository.ResolveEnvironment(ctx, environmentID)
	if err != nil {
		return DeploymentScheduleView{}, scheduleScopeError(err)
	}
	canManage, err := s.authorizeRead(ctx, principal, scope)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	policy, err := s.repository.Get(ctx, environmentID)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	policy, err = effectiveDeploymentSchedule(environmentID, policy)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	return DeploymentScheduleView{Policy: *policy, CanManage: canManage}, nil
}

// Put replaces one policy using optimistic concurrency and idempotency.
func (s *DeploymentScheduleService) Put(ctx context.Context, principal PlanPrincipal, input UpdateDeploymentScheduleInput) (DeploymentScheduleView, error) {
	scope, err := s.authorizeMutation(ctx, principal, input.EnvironmentID)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 255 {
		return DeploymentScheduleView{}, ErrDeploymentScheduleInvalid
	}
	policy, err := newDeploymentSchedulePolicy(input)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	mutation := s.deploymentScheduleMutation(principal, scope, input)
	stored, err := s.repository.Put(ctx, mutation, policy)
	if err != nil {
		return DeploymentScheduleView{}, err
	}
	return DeploymentScheduleView{Policy: stored, CanManage: true}, nil
}

func (s *DeploymentScheduleService) authorizeRead(ctx context.Context, principal PlanPrincipal, scope authz.Scope) (bool, error) {
	canManage, err := s.allowed(ctx, principal, s.manage, scope)
	if err != nil || canManage {
		return canManage, err
	}
	canView, err := s.allowed(ctx, principal, s.view, scope)
	if err != nil {
		return false, err
	}
	if !canView {
		return false, ErrDeploymentScheduleNotFound
	}
	return false, nil
}

func (s *DeploymentScheduleService) authorizeMutation(ctx context.Context, principal PlanPrincipal, environmentID uuid.UUID) (authz.Scope, error) {
	scope, err := s.repository.ResolveEnvironment(ctx, environmentID)
	if err != nil {
		return authz.Scope{}, scheduleScopeError(err)
	}
	allowed, err := s.allowed(ctx, principal, s.manage, scope)
	if err != nil {
		return authz.Scope{}, err
	}
	if !allowed {
		return authz.Scope{}, ErrDeploymentScheduleNotFound
	}
	return scope, nil
}

func effectiveDeploymentSchedule(environmentID uuid.UUID, policy *deploydomain.DeploymentSchedulePolicy) (*deploydomain.DeploymentSchedulePolicy, error) {
	if policy != nil {
		return policy, nil
	}
	value, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{EnvironmentID: environmentID})
	if err != nil {
		return nil, fmt.Errorf("create default deployment schedule: %w", err)
	}
	return &value, nil
}

func newDeploymentSchedulePolicy(input UpdateDeploymentScheduleInput) (deploydomain.DeploymentSchedulePolicy, error) {
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: input.EnvironmentID, Enabled: input.Enabled, TimeZone: input.TimeZone,
		WeeklyWindows: input.WeeklyWindows, Blackouts: input.Blackouts, Version: input.ExpectedVersion + 1,
	})
	if err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, fmt.Errorf("%w: %v", ErrDeploymentScheduleInvalid, err)
	}
	return policy, nil
}

func (s *DeploymentScheduleService) deploymentScheduleMutation(principal PlanPrincipal, scope authz.Scope, input UpdateDeploymentScheduleInput) DeploymentScheduleMutation {
	return DeploymentScheduleMutation{
		ActorID: principal.UserID, OrganizationID: scope.OrganizationID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		ExpectedVersion: input.ExpectedVersion, OccurredAt: s.clock.Now().UTC(),
	}
}

func (s *DeploymentScheduleService) allowed(ctx context.Context, principal PlanPrincipal, permission authz.Permission, scope authz.Scope) (bool, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return false, fmt.Errorf("authorize deployment schedule: %w", err)
	}
	return allowed, nil
}

func scheduleScopeError(err error) error {
	if errors.Is(err, ErrDeploymentScheduleNotFound) {
		return ErrDeploymentScheduleNotFound
	}
	return fmt.Errorf("resolve deployment schedule Environment: %w", err)
}
