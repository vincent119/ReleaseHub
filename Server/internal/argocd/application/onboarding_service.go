package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

var (
	// ErrOnboardingNotFound intentionally hides missing and unauthorized targets.
	ErrOnboardingNotFound = errors.New("onboarding target not found")
	// ErrOnboardingValidation indicates that the persisted dry-run result is not confirmable.
	ErrOnboardingValidation = errors.New("onboarding validation failed")
	// ErrOnboardingConflict indicates a stale version or invalid lifecycle transition.
	ErrOnboardingConflict = errors.New("onboarding state conflict")
)

// OnboardingMutation identifies the authenticated actor attached to an audited command.
type OnboardingMutation struct {
	ActorID   uuid.UUID
	RequestID string
}

// OnboardingRepository is the transactional persistence boundary for onboarding state.
type OnboardingRepository interface {
	LoadTarget(context.Context, uuid.UUID) (argodomain.OnboardingTarget, error)
	SaveValidation(context.Context, OnboardingMutation, argodomain.OnboardingTarget, []argodomain.ValidationIssueCode, string, time.Time) (argodomain.Onboarding, error)
	BeginApplying(context.Context, OnboardingMutation, uuid.UUID, uint64, string, time.Time) (argodomain.Onboarding, error)
	MarkManaged(context.Context, *uuid.UUID, string, uuid.UUID, uint64, time.Time) (argodomain.Onboarding, error)
	RecordOperationError(context.Context, *uuid.UUID, string, uuid.UUID, uint64, string, time.Time) (argodomain.Onboarding, error)
	ListApplying(context.Context) ([]ApplyingOnboarding, error)
}

// IdempotentConfirmationRepository atomically reserves a confirm command and records Applying.
type IdempotentConfirmationRepository interface {
	BeginApplyingIdempotent(context.Context, OnboardingMutation, uuid.UUID, uint64, string, OnboardingCommand, time.Time) (argodomain.Onboarding, OnboardingCommand, bool, error)
}

// ArgoCDManager exposes the only external reads and mutation allowed by onboarding.
type ArgoCDManager interface {
	GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error)
	CanUpdateApplication(context.Context, argodomain.ApplicationIdentity, string) (bool, error)
	DisableAutomatedSync(context.Context, argodomain.ApplicationIdentity, string) error
}

// OnboardingAuthorizer evaluates a complete Application scope.
type OnboardingAuthorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// OnboardingPrincipal is the authenticated local user issuing a command.
type OnboardingPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// ApplyingOnboarding is the recoverable external operation persisted before Argo CD mutation.
type ApplyingOnboarding struct {
	Target     argodomain.OnboardingTarget
	Onboarding argodomain.Onboarding
}

// OnboardingService coordinates validation, confirmation, readback, and restart recovery.
type OnboardingService struct {
	repository OnboardingRepository
	manager    ArgoCDManager
	authorizer OnboardingAuthorizer
	now        func() time.Time
	onboard    authz.Permission
	disable    authz.Permission
}

// NewOnboardingService creates the production onboarding use case.
func NewOnboardingService(repository OnboardingRepository, manager ArgoCDManager, authorizer OnboardingAuthorizer) (*OnboardingService, error) {
	if repository == nil || manager == nil || authorizer == nil {
		return nil, fmt.Errorf("invalid onboarding service dependencies")
	}
	onboard, err := authz.NewPermission("application.onboard")
	if err != nil {
		return nil, err
	}
	disable, err := authz.NewPermission("application.automated_sync.disable")
	if err != nil {
		return nil, err
	}
	return &OnboardingService{repository: repository, manager: manager, authorizer: authorizer, now: time.Now, onboard: onboard, disable: disable}, nil
}

// DryRun validates a fresh Application read and persists no Argo CD mutation.
func (s *OnboardingService) DryRun(ctx context.Context, principal OnboardingPrincipal, mutation OnboardingMutation, applicationID uuid.UUID) (argodomain.Onboarding, error) {
	mutation.ActorID = principal.UserID
	target, err := s.authorizedTarget(ctx, principal, applicationID, s.onboard)
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	observation, issues := s.validateFresh(ctx, target, true)
	resourceVersion := observation.ResourceVersion
	state, err := s.repository.SaveValidation(ctx, mutation, target, issues, resourceVersion, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	return state, nil
}

// Confirm revalidates, records intent, disables automated sync, and requires a fresh readback before Managed.
func (s *OnboardingService) Confirm(ctx context.Context, principal OnboardingPrincipal, mutation OnboardingMutation, applicationID uuid.UUID, expectedVersion uint64) (argodomain.Onboarding, error) {
	mutation.ActorID = principal.UserID
	target, err := s.authorizedTarget(ctx, principal, applicationID, s.onboard, s.disable)
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	observation, issues := s.validateFresh(ctx, target, true)
	if len(issues) > 0 {
		state, saveErr := s.repository.SaveValidation(ctx, mutation, target, issues, observation.ResourceVersion, s.now().UTC())
		if saveErr != nil {
			return argodomain.Onboarding{}, saveErr
		}
		return state, ErrOnboardingValidation
	}
	applying, err := s.repository.BeginApplying(ctx, mutation, applicationID, expectedVersion, observation.ResourceVersion, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	return s.finishApplying(ctx, target, applying, &mutation.ActorID, mutation.RequestID)
}

// ConfirmIdempotent confirms onboarding once for a caller supplied idempotency key.
func (s *OnboardingService) ConfirmIdempotent(ctx context.Context, principal OnboardingPrincipal, mutation OnboardingMutation, applicationID uuid.UUID, expectedVersion uint64, idempotencyKey string) (argodomain.Onboarding, error) {
	repository, ok := s.repository.(IdempotentConfirmationRepository)
	if !ok {
		return argodomain.Onboarding{}, errors.New("onboarding idempotency repository is unavailable")
	}
	mutation.ActorID = principal.UserID
	target, err := s.authorizedTarget(ctx, principal, applicationID, s.onboard, s.disable)
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	observation, issues := s.validateFresh(ctx, target, true)
	if len(issues) > 0 {
		state, saveErr := s.repository.SaveValidation(ctx, mutation, target, issues, observation.ResourceVersion, s.now().UTC())
		if saveErr != nil {
			return argodomain.Onboarding{}, saveErr
		}
		return state, ErrOnboardingValidation
	}
	command, err := NewOnboardingCommand(principal.UserID, applicationID, CommandConfirm, idempotencyKey, &expectedVersion, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	applying, persisted, replayed, err := repository.BeginApplyingIdempotent(ctx, mutation, applicationID, expectedVersion, observation.ResourceVersion, command, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	if replayed {
		if persisted.State == CommandCompleted {
			return argodomain.Onboarding{ApplicationID: applicationID, Status: persisted.OnboardingStatus, Version: dereferenceVersion(persisted.OnboardingVersion)}, nil
		}
		return argodomain.Onboarding{}, ErrCommandInProgress
	}
	return s.finishApplying(ctx, target, applying, &mutation.ActorID, mutation.RequestID)
}

func dereferenceVersion(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

// RecoverApplying resumes commands that crossed the PostgreSQL and Argo CD boundary before a restart.
func (s *OnboardingService) RecoverApplying(ctx context.Context) error {
	values, err := s.repository.ListApplying(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, value := range values {
		if _, err := s.finishApplying(ctx, value.Target, value.Onboarding, nil, "worker-recovery"); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (s *OnboardingService) finishApplying(ctx context.Context, target argodomain.OnboardingTarget, applying argodomain.Onboarding, actorID *uuid.UUID, requestID string) (argodomain.Onboarding, error) {
	observation, issues := s.validateFresh(ctx, target, false)
	if len(issues) > 0 {
		return s.operationFailed(ctx, actorID, requestID, applying, "pre_mutation_validation_failed")
	}
	if observation.AutomatedSync {
		if err := s.manager.DisableAutomatedSync(ctx, target.Argo, target.ArgoProject); err != nil {
			return s.operationFailed(ctx, actorID, requestID, applying, "automated_sync_disable_failed")
		}
	}
	readback, err := s.manager.GetApplication(ctx, target.Argo, target.ArgoProject)
	if err != nil {
		return s.operationFailed(ctx, actorID, requestID, applying, "readback_failed")
	}
	issues = target.ValidateObservation(readback)
	if readback.AutomatedSync {
		issues = append(issues, argodomain.IssueAutomatedSyncStillActive)
	}
	if len(issues) > 0 {
		return s.operationFailed(ctx, actorID, requestID, applying, "readback_validation_failed")
	}
	state, err := s.repository.MarkManaged(ctx, actorID, requestID, applying.ApplicationID, applying.Version, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	return state, nil
}

func (s *OnboardingService) operationFailed(ctx context.Context, actorID *uuid.UUID, requestID string, applying argodomain.Onboarding, code string) (argodomain.Onboarding, error) {
	state, err := s.repository.RecordOperationError(ctx, actorID, requestID, applying.ApplicationID, applying.Version, code, s.now().UTC())
	if err != nil {
		return argodomain.Onboarding{}, err
	}
	return state, fmt.Errorf("onboarding external operation is pending recovery: %s", code)
}

func (s *OnboardingService) validateFresh(ctx context.Context, target argodomain.OnboardingTarget, checkPermission bool) (argodomain.Application, []argodomain.ValidationIssueCode) {
	observation, err := s.manager.GetApplication(ctx, target.Argo, target.ArgoProject)
	if err != nil {
		return argodomain.Application{}, []argodomain.ValidationIssueCode{argodomain.IssueApplicationUnavailable}
	}
	issues := target.ValidateObservation(observation)
	if checkPermission {
		allowed, err := s.manager.CanUpdateApplication(ctx, target.Argo, target.ArgoProject)
		if err != nil || !allowed {
			issues = append(issues, argodomain.IssueUpdatePermissionDenied)
		}
	}
	return observation, issues
}

func (s *OnboardingService) authorizedTarget(ctx context.Context, principal OnboardingPrincipal, applicationID uuid.UUID, permissions ...authz.Permission) (argodomain.OnboardingTarget, error) {
	target, err := s.repository.LoadTarget(ctx, applicationID)
	if err != nil {
		return argodomain.OnboardingTarget{}, ErrOnboardingNotFound
	}
	scope, err := authz.NewApplicationScope(target.OrganizationID, target.ProjectID, target.EnvironmentID, target.ApplicationID)
	if err != nil {
		return argodomain.OnboardingTarget{}, err
	}
	for _, permission := range permissions {
		allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
			UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
		})
		if err != nil {
			return argodomain.OnboardingTarget{}, fmt.Errorf("authorize Application onboarding: %w", err)
		}
		if !allowed {
			return argodomain.OnboardingTarget{}, ErrOnboardingNotFound
		}
	}
	return target, nil
}
