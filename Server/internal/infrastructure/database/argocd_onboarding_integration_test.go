//go:build integration

package database_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestApplicationOnboardingLifecycleRecoveryAndDrift(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 5, MaxIdleConnections: 2})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	catalogRepository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		t.Fatalf("create catalog repository: %v", err)
	}
	mutation := catalogapp.Mutation{RequestID: "onboarding-integration"}
	organization := mustOrganization(t, "onboarding-tenant")
	project := mustProject(t, organization.ID, "payment")
	environment := mustEnvironment(t, organization.ID, project.ID, "production", catalog.EnvironmentProduction)
	mustCreate(t, catalogRepository.CreateOrganization(ctx, mutation, organization))
	mustCreate(t, catalogRepository.CreateProject(ctx, mutation, project))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, environment))
	applicationA := mustApplication(t, organization.ID, project.ID, environment.ID, "api", "payment-api-production", "https://git.example.com/payment/api.git", "deploy/production")
	applicationB := mustApplication(t, organization.ID, project.ID, environment.ID, "worker", "payment-worker-production", "https://git.example.com/platform/manifests.git", "production/payment-worker")
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, applicationA))
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, applicationB))

	reconciliationRepository, err := argoinfra.NewReconciliationRepository(db)
	if err != nil {
		t.Fatalf("create reconciliation repository: %v", err)
	}
	observedA := mustObservedApplication(t, applicationA.Argo.Name, applicationA.Source.RepositoryURL, applicationA.Source.Path, true)
	observedB := mustObservedApplication(t, applicationB.Argo.Name, applicationB.Source.RepositoryURL, applicationB.Source.Path, true)
	observedA.Project = applicationA.ArgoProject
	observedA.Destination = argodomain.Destination{Server: applicationA.DestinationServer, Namespace: applicationA.DestinationNamespace}
	observedB.Project = applicationB.ArgoProject
	observedB.Destination = argodomain.Destination{Server: applicationB.DestinationServer, Namespace: applicationB.DestinationNamespace}
	if _, err := reconciliationRepository.Reconcile(ctx, []argodomain.Application{observedA, observedB}, time.Now().UTC()); err != nil {
		t.Fatalf("persist onboarding candidates: %v", err)
	}

	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, ?)`, actorID, "onboarding-actor").Error; err != nil {
		t.Fatalf("create onboarding actor: %v", err)
	}
	onboardingRepository, err := argoinfra.NewOnboardingRepository(db)
	if err != nil {
		t.Fatalf("create onboarding repository: %v", err)
	}
	manager := newIntegrationArgoManager(observedA, observedB)
	service, err := argoapp.NewOnboardingService(onboardingRepository, manager, allowOnboardingAuthorizer{})
	if err != nil {
		t.Fatalf("create onboarding service: %v", err)
	}
	principal := argoapp.OnboardingPrincipal{UserID: actorID}

	dryRunA, err := service.DryRun(ctx, principal, argoapp.OnboardingMutation{ActorID: actorID, RequestID: "dry-run-a"}, applicationA.ID)
	if err != nil || dryRunA.Status != argodomain.OnboardingAwaitingConfirmation {
		t.Fatalf("dry-run state = %#v, error = %v", dryRunA, err)
	}
	managedA, err := service.ConfirmIdempotent(ctx, principal, argoapp.OnboardingMutation{ActorID: actorID, RequestID: "confirm-a"}, applicationA.ID, dryRunA.Version, "confirm-application-a")
	if err != nil || managedA.Status != argodomain.OnboardingManaged {
		t.Fatalf("managed state = %#v, error = %v", managedA, err)
	}
	if manager.disableCount(applicationA.Argo.Name) != 1 {
		t.Fatalf("Application A disable count = %d, want 1", manager.disableCount(applicationA.Argo.Name))
	}
	var commandCount int64
	if err := db.Table("onboarding_commands").Where("actor_id = ? AND operation = ? AND idempotency_key = ?", actorID, string(argoapp.CommandConfirm), "confirm-application-a").Count(&commandCount).Error; err != nil || commandCount != 1 {
		t.Fatalf("persisted onboarding command count = %d, error = %v", commandCount, err)
	}
	replayedA, err := service.ConfirmIdempotent(ctx, principal, argoapp.OnboardingMutation{ActorID: actorID, RequestID: "confirm-a-replay"}, applicationA.ID, dryRunA.Version, "confirm-application-a")
	if err != nil || replayedA.Status != argodomain.OnboardingManaged || manager.disableCount(applicationA.Argo.Name) != 1 {
		t.Fatalf("idempotent replay = %#v, disables = %d, error = %v", replayedA, manager.disableCount(applicationA.Argo.Name), err)
	}

	dryRunB, err := service.DryRun(ctx, principal, argoapp.OnboardingMutation{ActorID: actorID, RequestID: "dry-run-b"}, applicationB.ID)
	if err != nil {
		t.Fatalf("dry-run Application B: %v", err)
	}
	manager.failNextReadAfterDisable(applicationB.Argo.Name)
	applyingB, err := service.Confirm(ctx, principal, argoapp.OnboardingMutation{ActorID: actorID, RequestID: "confirm-b"}, applicationB.ID, dryRunB.Version)
	if err == nil || applyingB.Status != argodomain.OnboardingApplying || applyingB.LastErrorCode != "readback_failed" {
		t.Fatalf("interrupted applying state = %#v, error = %v", applyingB, err)
	}

	restartedService, err := argoapp.NewOnboardingService(onboardingRepository, manager, allowOnboardingAuthorizer{})
	if err != nil {
		t.Fatalf("create restarted onboarding service: %v", err)
	}
	if err := restartedService.RecoverApplying(ctx); err != nil {
		t.Fatalf("recover applying onboarding: %v", err)
	}
	assertOnboardingStatus(t, db, applicationB.ID, argodomain.OnboardingManaged)

	driftedA := observedA
	driftedA.Labels = map[string]string{argodomain.ManagedLabelKey: "false"}
	driftedA.AutomatedSync = true
	managedB := manager.application(applicationB.Argo.Name)
	if _, err := reconciliationRepository.Reconcile(ctx, []argodomain.Application{driftedA, managedB}, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("persist managed drift: %v", err)
	}
	var driftReasons []string
	if err := db.Raw(`SELECT jsonb_array_elements_text(drift_reasons) FROM application_onboardings WHERE application_id = ?`, applicationA.ID).Scan(&driftReasons).Error; err != nil {
		t.Fatalf("read drift reasons: %v", err)
	}
	if !containsString(driftReasons, string(argodomain.DriftManagedLabelMissing)) || !containsString(driftReasons, string(argodomain.DriftAutomatedSyncEnabled)) {
		t.Fatalf("drift reasons = %#v", driftReasons)
	}
	assertOnboardingStatus(t, db, applicationA.ID, argodomain.OnboardingConfigurationDrift)
	driftAuditCount := countOnboardingDriftAudits(t, db, applicationA.ID)

	healthyA := manager.application(applicationA.Argo.Name)
	if _, err := reconciliationRepository.Reconcile(ctx, []argodomain.Application{healthyA, managedB}, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("persist healthy observation after drift: %v", err)
	}
	assertOnboardingStatus(t, db, applicationA.ID, argodomain.OnboardingConfigurationDrift)
	if got := countOnboardingDriftAudits(t, db, applicationA.ID); got != driftAuditCount {
		t.Fatalf("configuration drift audit count = %d, want %d", got, driftAuditCount)
	}
}

func TestOnboardingCommandIdempotencyPersistence(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 5, MaxIdleConnections: 2})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	catalogRepository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		t.Fatalf("create catalog repository: %v", err)
	}
	mutation := catalogapp.Mutation{RequestID: "onboarding-command-integration"}
	organization := mustOrganization(t, "command-tenant")
	project := mustProject(t, organization.ID, "payments")
	environment := mustEnvironment(t, organization.ID, project.ID, "production", catalog.EnvironmentProduction)
	application := mustApplication(t, organization.ID, project.ID, environment.ID, "api", "payments-api-production", "https://git.example.com/payments/api.git", "deploy/production")
	mustCreate(t, catalogRepository.CreateOrganization(ctx, mutation, organization))
	mustCreate(t, catalogRepository.CreateProject(ctx, mutation, project))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, environment))
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, application))

	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, ?)`, actorID, "command-actor").Error; err != nil {
		t.Fatalf("create command actor: %v", err)
	}
	repository, err := argoinfra.NewCommandRepository(db)
	if err != nil {
		t.Fatalf("create command repository: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	expectedVersion := uint64(7)
	command, err := argoapp.NewOnboardingCommand(actorID, application.ID, argoapp.CommandConfirm, "confirm-payment-api-v1", &expectedVersion, now)
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	reserved, replayed, err := repository.Reserve(ctx, command)
	if err != nil || replayed || reserved.ID != command.ID || reserved.State != argoapp.CommandAccepted {
		t.Fatalf("first reserve = %#v, replayed = %t, error = %v", reserved, replayed, err)
	}
	replayedCommand, replayed, err := repository.Reserve(ctx, command)
	if err != nil || !replayed || replayedCommand.ID != command.ID || replayedCommand.State != argoapp.CommandAccepted {
		t.Fatalf("replayed reserve = %#v, replayed = %t, error = %v", replayedCommand, replayed, err)
	}

	otherVersion := uint64(8)
	conflicting, err := argoapp.NewOnboardingCommand(actorID, application.ID, argoapp.CommandConfirm, "confirm-payment-api-v1", &otherVersion, now.Add(time.Second))
	if err != nil {
		t.Fatalf("create conflicting command: %v", err)
	}
	if _, _, err := repository.Reserve(ctx, conflicting); !errors.Is(err, argoapp.ErrIdempotencyKeyReuse) {
		t.Fatalf("conflicting key error = %v, want ErrIdempotencyKeyReuse", err)
	}

	completed, err := repository.Complete(ctx, command.ID, argodomain.OnboardingManaged, 8, "managed", now.Add(2*time.Second))
	if err != nil || completed.State != argoapp.CommandCompleted || completed.OnboardingStatus != argodomain.OnboardingManaged || completed.OnboardingVersion == nil || *completed.OnboardingVersion != 8 {
		t.Fatalf("complete command = %#v, error = %v", completed, err)
	}
	completedReplay, replayed, err := repository.Reserve(ctx, command)
	if err != nil || !replayed || completedReplay.State != argoapp.CommandCompleted || completedReplay.ResponseCode != "managed" {
		t.Fatalf("completed replay = %#v, replayed = %t, error = %v", completedReplay, replayed, err)
	}
}

func assertOnboardingStatus(t *testing.T, db *gorm.DB, applicationID uuid.UUID, want argodomain.OnboardingStatus) {
	t.Helper()
	var got string
	if err := db.Raw(`SELECT status FROM application_onboardings WHERE application_id = ?`, applicationID).Scan(&got).Error; err != nil {
		t.Fatalf("read onboarding status: %v", err)
	}
	if got != string(want) {
		t.Fatalf("onboarding status = %q, want %q", got, want)
	}
}

func countOnboardingDriftAudits(t *testing.T, db *gorm.DB, applicationID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := db.Table("audit_logs").Where("resource_type = ? AND resource_id = ? AND metadata ->> 'state' = ?", "application_onboarding", applicationID.String(), string(argodomain.OnboardingConfigurationDrift)).Count(&count).Error; err != nil {
		t.Fatalf("count drift audits: %v", err)
	}
	return count
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type allowOnboardingAuthorizer struct{}

func (allowOnboardingAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return true, nil
}

type integrationArgoManager struct {
	mu                   sync.Mutex
	applications         map[string]argodomain.Application
	disables             map[string]int
	failReadAfterDisable map[string]bool
}

func newIntegrationArgoManager(values ...argodomain.Application) *integrationArgoManager {
	manager := &integrationArgoManager{
		applications: make(map[string]argodomain.Application), disables: make(map[string]int), failReadAfterDisable: make(map[string]bool),
	}
	for _, value := range values {
		manager.applications[value.Identity.Name] = value
	}
	return manager
}

func (m *integrationArgoManager) GetApplication(_ context.Context, identity argodomain.ApplicationIdentity, project string) (argodomain.Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, found := m.applications[identity.Name]
	if !found || value.Identity != identity || value.Project != project {
		return argodomain.Application{}, errors.New("application unavailable")
	}
	if m.failReadAfterDisable[identity.Name] && m.disables[identity.Name] > 0 {
		delete(m.failReadAfterDisable, identity.Name)
		return argodomain.Application{}, errors.New("temporary readback failure")
	}
	return value, nil
}

func (m *integrationArgoManager) CanUpdateApplication(context.Context, argodomain.ApplicationIdentity, string) (bool, error) {
	return true, nil
}

func (m *integrationArgoManager) DisableAutomatedSync(_ context.Context, identity argodomain.ApplicationIdentity, project string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, found := m.applications[identity.Name]
	if !found || value.Project != project {
		return fmt.Errorf("application unavailable")
	}
	value.AutomatedSync = false
	m.applications[identity.Name] = value
	m.disables[identity.Name]++
	return nil
}

func (m *integrationArgoManager) failNextReadAfterDisable(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failReadAfterDisable[name] = true
}

func (m *integrationArgoManager) disableCount(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disables[name]
}

func (m *integrationArgoManager) application(name string) argodomain.Application {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applications[name]
}
