package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestApplicationOnboardingRequiresDryRunConfirmationAndReadback(t *testing.T) {
	repository := newOnboardingRepositoryStub(validOnboardingTarget())
	manager := &onboardingManagerStub{observation: validOnboardingObservation(t, true), allowed: true}
	service := mustOnboardingService(t, repository, manager)
	principal := OnboardingPrincipal{UserID: uuid.New()}
	mutation := OnboardingMutation{ActorID: uuid.New(), RequestID: "request-1"}

	dryRun, err := service.DryRun(context.Background(), principal, mutation, repository.target.ApplicationID)
	if err != nil || dryRun.Status != argodomain.OnboardingAwaitingConfirmation || manager.disableCalls != 0 {
		t.Fatalf("dry-run result mismatch: %#v disable=%d err=%v", dryRun, manager.disableCalls, err)
	}
	managed, err := service.Confirm(context.Background(), principal, mutation, repository.target.ApplicationID, dryRun.Version)
	if err != nil {
		t.Fatalf("confirm onboarding: %v", err)
	}
	if managed.Status != argodomain.OnboardingManaged || managed.ManagedAt == nil || manager.disableCalls != 1 {
		t.Fatalf("managed state mismatch: %#v disable=%d", managed, manager.disableCalls)
	}
	if managed.ConfirmedBy == nil || *managed.ConfirmedBy != principal.UserID {
		t.Fatalf("confirmed actor = %v, want authenticated principal %s", managed.ConfirmedBy, principal.UserID)
	}
	if manager.observation.AutomatedSync {
		t.Fatal("automated sync should be disabled before Managed")
	}
}

func TestOnboardingValidationFailureDoesNotMutateArgoCD(t *testing.T) {
	target := validOnboardingTarget()
	target.CandidatePresent = false
	repository := newOnboardingRepositoryStub(target)
	observation := validOnboardingObservation(t, true)
	observation.Labels = map[string]string{}
	manager := &onboardingManagerStub{observation: observation, allowed: false}
	service := mustOnboardingService(t, repository, manager)
	principal := OnboardingPrincipal{UserID: uuid.New()}
	state, err := service.DryRun(context.Background(), principal, OnboardingMutation{ActorID: principal.UserID}, target.ApplicationID)
	if err != nil {
		t.Fatalf("persist validation failure: %v", err)
	}
	if state.Status != argodomain.OnboardingValidationFailed || len(state.ValidationIssues) < 3 || manager.disableCalls != 0 {
		t.Fatalf("validation should fail closed without mutation: %#v disable=%d", state, manager.disableCalls)
	}
	if _, err := service.Confirm(context.Background(), principal, OnboardingMutation{ActorID: principal.UserID}, target.ApplicationID, state.Version); !errors.Is(err, ErrOnboardingValidation) {
		t.Fatalf("invalid confirmation should fail validation: %v", err)
	}
	if manager.disableCalls != 0 {
		t.Fatal("failed confirmation mutated Argo CD")
	}
}

func TestApplyingOnboardingRecoversAfterWorkerRestart(t *testing.T) {
	target := validOnboardingTarget()
	repository := newOnboardingRepositoryStub(target)
	repository.state = argodomain.Onboarding{
		ApplicationID: target.ApplicationID, Status: argodomain.OnboardingApplying, Version: 4,
	}
	manager := &onboardingManagerStub{observation: validOnboardingObservation(t, true), allowed: true}
	service := mustOnboardingService(t, repository, manager)
	if err := service.RecoverApplying(context.Background()); err != nil {
		t.Fatalf("recover applying onboarding: %v", err)
	}
	if repository.state.Status != argodomain.OnboardingManaged || manager.disableCalls != 1 {
		t.Fatalf("restart recovery did not finish: %#v disable=%d", repository.state, manager.disableCalls)
	}
}

func validOnboardingTarget() argodomain.OnboardingTarget {
	return argodomain.OnboardingTarget{
		ApplicationID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		Production: true, CandidatePresent: true,
		Argo: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}, ArgoProject: "payment",
		DestinationServer: "https://kubernetes.default.svc", DestinationNamespace: "payment",
		Source: argodomain.CatalogSource{RepositoryURL: "https://git.example.com/manifests.git", TargetRevision: "main", Path: "production/payment"},
	}
}

func validOnboardingObservation(t *testing.T, automated bool) argodomain.Application {
	t.Helper()
	value, err := argodomain.NewApplication(argodomain.Application{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}, Project: "payment",
		Labels:        map[string]string{argodomain.ManagedLabelKey: argodomain.ManagedLabelValue},
		Sources:       []argodomain.Source{{RepositoryURL: "https://git.example.com/manifests.git", TargetRevision: "main", Path: "production/payment"}},
		Destination:   argodomain.Destination{Server: "https://kubernetes.default.svc", Namespace: "payment"},
		AutomatedSync: automated, ResourceVersion: "42",
	})
	if err != nil {
		t.Fatalf("create valid onboarding observation: %v", err)
	}
	return value
}

func mustOnboardingService(t *testing.T, repository *onboardingRepositoryStub, manager *onboardingManagerStub) *OnboardingService {
	t.Helper()
	service, err := NewOnboardingService(repository, manager, allowOnboardingAuthorizer{})
	if err != nil {
		t.Fatalf("create onboarding service: %v", err)
	}
	service.now = func() time.Time { return time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC) }
	return service
}

type onboardingRepositoryStub struct {
	target argodomain.OnboardingTarget
	state  argodomain.Onboarding
}

func newOnboardingRepositoryStub(target argodomain.OnboardingTarget) *onboardingRepositoryStub {
	return &onboardingRepositoryStub{target: target}
}

func (s *onboardingRepositoryStub) LoadTarget(_ context.Context, applicationID uuid.UUID) (argodomain.OnboardingTarget, error) {
	if applicationID != s.target.ApplicationID {
		return argodomain.OnboardingTarget{}, errors.New("not found")
	}
	return s.target, nil
}

func (s *onboardingRepositoryStub) SaveValidation(_ context.Context, _ OnboardingMutation, target argodomain.OnboardingTarget, issues []argodomain.ValidationIssueCode, resourceVersion string, now time.Time) (argodomain.Onboarding, error) {
	status := argodomain.OnboardingAwaitingConfirmation
	if len(issues) > 0 {
		status = argodomain.OnboardingValidationFailed
	}
	s.state = argodomain.Onboarding{
		ApplicationID: target.ApplicationID, Status: status, ValidationIssues: issues,
		ValidatedResourceVersion: resourceVersion, ValidatedAt: &now, Version: s.state.Version + 1,
	}
	return s.state, nil
}

func (s *onboardingRepositoryStub) BeginApplying(_ context.Context, mutation OnboardingMutation, applicationID uuid.UUID, expectedVersion uint64, resourceVersion string, now time.Time) (argodomain.Onboarding, error) {
	if s.state.Status != argodomain.OnboardingAwaitingConfirmation || s.state.Version != expectedVersion {
		return argodomain.Onboarding{}, ErrOnboardingConflict
	}
	s.state.Status = argodomain.OnboardingApplying
	s.state.Version++
	s.state.ValidatedResourceVersion = resourceVersion
	s.state.ConfirmedBy = &mutation.ActorID
	s.state.ConfirmedAt = &now
	return s.state, nil
}

func (s *onboardingRepositoryStub) MarkManaged(_ context.Context, _ *uuid.UUID, _ string, applicationID uuid.UUID, expectedVersion uint64, now time.Time) (argodomain.Onboarding, error) {
	if s.state.ApplicationID != applicationID || s.state.Status != argodomain.OnboardingApplying || s.state.Version != expectedVersion {
		return argodomain.Onboarding{}, ErrOnboardingConflict
	}
	s.state.Status = argodomain.OnboardingManaged
	s.state.Version++
	s.state.ManagedAt = &now
	s.state.LastErrorCode = ""
	return s.state, nil
}

func (s *onboardingRepositoryStub) RecordOperationError(_ context.Context, _ *uuid.UUID, _ string, applicationID uuid.UUID, expectedVersion uint64, code string, _ time.Time) (argodomain.Onboarding, error) {
	if s.state.ApplicationID != applicationID || s.state.Version != expectedVersion {
		return argodomain.Onboarding{}, ErrOnboardingConflict
	}
	s.state.Version++
	s.state.LastErrorCode = code
	return s.state, nil
}

func (s *onboardingRepositoryStub) ListApplying(context.Context) ([]ApplyingOnboarding, error) {
	if s.state.Status != argodomain.OnboardingApplying {
		return []ApplyingOnboarding{}, nil
	}
	return []ApplyingOnboarding{{Target: s.target, Onboarding: s.state}}, nil
}

type onboardingManagerStub struct {
	observation  argodomain.Application
	allowed      bool
	disableCalls int
}

func (s *onboardingManagerStub) GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	return s.observation, nil
}

func (s *onboardingManagerStub) CanUpdateApplication(context.Context, argodomain.ApplicationIdentity, string) (bool, error) {
	return s.allowed, nil
}

func (s *onboardingManagerStub) DisableAutomatedSync(context.Context, argodomain.ApplicationIdentity, string) error {
	s.disableCalls++
	s.observation.AutomatedSync = false
	return nil
}

type allowOnboardingAuthorizer struct{}

func (allowOnboardingAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return true, nil
}
