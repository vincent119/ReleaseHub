//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	ecrapp "github.com/vincent119/ReleaseHub/Server/internal/ecr/application"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
	ecrinfra "github.com/vincent119/ReleaseHub/Server/internal/ecr/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestImageDigestSnapshotsPersistWithAuditAndOutbox(t *testing.T) {
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
	organization := mustOrganization(t, "ecr-tenant")
	project := mustProject(t, organization.ID, "payment")
	environment := mustEnvironment(t, organization.ID, project.ID, "production", catalog.EnvironmentProduction)
	mutation := catalogapp.Mutation{RequestID: "ecr-catalog"}
	mustCreate(t, catalogRepository.CreateOrganization(ctx, mutation, organization))
	mustCreate(t, catalogRepository.CreateProject(ctx, mutation, project))
	mustCreate(t, catalogRepository.CreateEnvironment(ctx, mutation, environment))
	application := mustApplication(t, organization.ID, project.ID, environment.ID, "api", "payment-api-production", "https://git.example.com/platform/manifests.git", "production/payment-api")
	mustCreate(t, catalogRepository.CreateApplication(ctx, mutation, application))
	if err := db.Exec(`INSERT INTO application_onboardings (application_id, status) VALUES (?, 'Managed')`, application.ID).Error; err != nil {
		t.Fatalf("create managed onboarding: %v", err)
	}
	actorID := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, ?)`, actorID, "ecr-actor").Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}

	repository, err := ecrinfra.NewDigestRepository(db)
	if err != nil {
		t.Fatalf("create digest repository: %v", err)
	}
	target, err := repository.LoadDigestTarget(ctx, application.ID)
	if err != nil || target.ApplicationID != application.ID || target.Argo.Namespace != application.Argo.Namespace || target.Argo.Name != application.Argo.Name {
		t.Fatalf("load digest target = %#v, %v", target, err)
	}
	scope, err := ecrdomain.NewRegistryScope("123456789012", "ap-northeast-1", []string{"platform/payment-api"})
	if err != nil {
		t.Fatalf("create registry scope: %v", err)
	}
	reference, err := scope.ParseImageReference("123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/payment-api:v1.2.3")
	if err != nil {
		t.Fatalf("parse image reference: %v", err)
	}
	resolvedAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	available, err := ecrdomain.NewAvailableDigestSnapshot(application.ID, "commit-a", reference, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", resolvedAt)
	if err != nil {
		t.Fatalf("create available snapshot: %v", err)
	}
	unavailable, err := ecrdomain.NewUnavailableDigestSnapshot(application.ID, "commit-a", "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/payment-worker:v1.2.3", "repository_out_of_scope", resolvedAt)
	if err != nil {
		t.Fatalf("create unavailable snapshot: %v", err)
	}
	if err := repository.AppendDigestSnapshots(ctx, ecrapp.DigestMutation{ActorID: actorID, RequestID: "ecr-resolve"}, target, []ecrdomain.DigestSnapshot{available, unavailable}); err != nil {
		t.Fatalf("persist digest snapshots: %v", err)
	}

	var values []struct {
		Status         string
		ResolvedDigest string
		ErrorCode      string
	}
	if err := db.Table("application_image_digest_snapshots").Select("status, resolved_digest, error_code").Where("application_id = ?", application.ID).Find(&values).Error; err != nil {
		t.Fatalf("load image digest snapshots: %v", err)
	}
	var foundAvailable, foundUnavailable bool
	for _, value := range values {
		foundAvailable = foundAvailable || (value.Status == "Available" && value.ResolvedDigest != "" && value.ErrorCode == "")
		foundUnavailable = foundUnavailable || (value.Status == "Unavailable" && value.ResolvedDigest == "" && value.ErrorCode == "repository_out_of_scope")
	}
	if len(values) != 2 || !foundAvailable || !foundUnavailable {
		t.Fatalf("persisted snapshots = %#v", values)
	}
	var auditCount, outboxCount int64
	if err := db.Table("audit_logs").Where("action = ? AND resource_id = ?", "application.image_digest.resolve", application.ID.String()).Count(&auditCount).Error; err != nil {
		t.Fatalf("count digest audit records: %v", err)
	}
	if err := db.Table("outbox_events").Where("event_type = ? AND aggregate_id = ?", "ecr.image_digest_snapshots_recorded", application.ID.String()).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count digest outbox records: %v", err)
	}
	if auditCount != 1 || outboxCount != 1 {
		t.Fatalf("digest audit/outbox counts = %d/%d", auditCount, outboxCount)
	}
}
