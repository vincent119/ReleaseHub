//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	migrationlib "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestAuditTrailFoundationMigration(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}
	migrator, err := migrationlib.NewWithSourceInstance("iofs", source, database.URL(cfg))
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _, _ = migrator.Close() })
	if err := migrator.Migrate(20260916001700); err != nil {
		t.Fatalf("apply migrations before audit trail: %v", err)
	}

	db, err := database.Open(cfg, config.PoolConfig{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	actorID, organizationID := uuid.New(), uuid.New()
	projectID, environmentID, applicationID := uuid.New(), uuid.New(), uuid.New()
	applicationAuditID, unresolvedAuditID := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES (?, 'audit-actor')`, actorID).Error; err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	if err := db.Exec(`INSERT INTO organizations (id, name) VALUES (?, 'audit-organization')`, organizationID).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if err := db.Exec(`INSERT INTO projects (id, organization_id, name) VALUES (?, ?, 'audit-project')`, projectID, organizationID).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO environments (id, organization_id, project_id, name, environment_type)
VALUES (?, ?, ?, 'audit-environment', 'Production')`, environmentID, organizationID, projectID).Error; err != nil {
		t.Fatalf("insert environment: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (
    id, organization_id, project_id, environment_id, name,
    argocd_namespace, argocd_application_name, argocd_project,
    destination_server, destination_namespace,
    source_repo_url, source_target_revision, source_path
) VALUES (?, ?, ?, ?, 'audit-application', 'argocd', 'audit-application', 'default',
          'https://kubernetes.default.svc', 'default', 'https://example.invalid/repository.git', 'main', 'deploy')`,
		applicationID, organizationID, projectID, environmentID).Error; err != nil {
		t.Fatalf("insert application: %v", err)
	}
	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Exec(`INSERT INTO audit_logs (id, occurred_at, actor_id, action, resource_type, resource_id, request_id)
VALUES (?, ?, ?, 'application.updated', 'application', ?, 'audit-request')`, applicationAuditID, occurredAt, actorID, applicationID.String()).Error; err != nil {
		t.Fatalf("insert resolvable audit: %v", err)
	}
	if err := db.Exec(`INSERT INTO audit_logs (id, occurred_at, actor_id, action, resource_type, resource_id)
VALUES (?, ?, ?, 'custom.updated', 'custom_resource', 'not-a-uuid')`, unresolvedAuditID, occurredAt, actorID).Error; err != nil {
		t.Fatalf("insert unresolved audit: %v", err)
	}

	if err := migrator.Migrate(20260917001800); err != nil {
		t.Fatalf("apply audit trail migration: %v", err)
	}

	type auditScope struct {
		OrganizationID   *uuid.UUID
		ProjectID        *uuid.UUID
		EnvironmentID    *uuid.UUID
		ApplicationID    *uuid.UUID
		ActorDisplayName *string
		ScopeResolution  string
	}
	var resolved auditScope
	if err := db.Raw(`SELECT organization_id, project_id, environment_id, application_id,
actor_display_name, scope_resolution FROM audit_logs WHERE id = ?`, applicationAuditID).Scan(&resolved).Error; err != nil {
		t.Fatalf("read resolved audit: %v", err)
	}
	if resolved.OrganizationID == nil || *resolved.OrganizationID != organizationID ||
		resolved.ProjectID == nil || *resolved.ProjectID != projectID ||
		resolved.EnvironmentID == nil || *resolved.EnvironmentID != environmentID ||
		resolved.ApplicationID == nil || *resolved.ApplicationID != applicationID ||
		resolved.ActorDisplayName == nil || *resolved.ActorDisplayName != "audit-actor" ||
		resolved.ScopeResolution != "resolved" {
		t.Fatalf("resolved audit scope = %#v", resolved)
	}
	var unresolved auditScope
	if err := db.Raw(`SELECT organization_id, project_id, environment_id, application_id,
actor_display_name, scope_resolution FROM audit_logs WHERE id = ?`, unresolvedAuditID).Scan(&unresolved).Error; err != nil {
		t.Fatalf("read unresolved audit: %v", err)
	}
	if unresolved.OrganizationID != nil || unresolved.ProjectID != nil || unresolved.EnvironmentID != nil ||
		unresolved.ApplicationID != nil || unresolved.ActorDisplayName == nil || *unresolved.ActorDisplayName != "audit-actor" ||
		unresolved.ScopeResolution != "unresolved" {
		t.Fatalf("unresolved audit scope = %#v", unresolved)
	}
	if err := database.WithinTransaction(ctx, db, func(tx *gorm.DB) error {
		return database.AppendAudit(ctx, tx, database.AuditRecord{
			OccurredAt: occurredAt.Add(time.Second), ActorID: &actorID,
			Action: "application.refreshed", ResourceType: "application", ResourceID: applicationID.String(),
			RequestID: "runtime-audit-request", Metadata: map[string]any{"result": "accepted"},
		})
	}); err != nil {
		t.Fatalf("append runtime audit: %v", err)
	}
	var runtimeAudit auditScope
	if err := db.Raw(`SELECT organization_id, project_id, environment_id, application_id,
actor_display_name, scope_resolution FROM audit_logs WHERE request_id = 'runtime-audit-request'`).Scan(&runtimeAudit).Error; err != nil {
		t.Fatalf("read runtime audit: %v", err)
	}
	if runtimeAudit.OrganizationID == nil || *runtimeAudit.OrganizationID != organizationID ||
		runtimeAudit.ProjectID == nil || *runtimeAudit.ProjectID != projectID ||
		runtimeAudit.EnvironmentID == nil || *runtimeAudit.EnvironmentID != environmentID ||
		runtimeAudit.ApplicationID == nil || *runtimeAudit.ApplicationID != applicationID ||
		runtimeAudit.ActorDisplayName == nil || *runtimeAudit.ActorDisplayName != "audit-actor" ||
		runtimeAudit.ScopeResolution != "resolved" {
		t.Fatalf("runtime audit scope = %#v", runtimeAudit)
	}

	assertAuditTrailFoundationPermissionSeed(t, db)
	var indexCount int64
	if err := db.Raw(`SELECT count(*) FROM pg_indexes
WHERE schemaname = current_schema() AND tablename = 'audit_logs' AND indexname IN (
    'audit_logs_occurred_id_idx',
    'audit_logs_project_occurred_idx',
    'audit_logs_environment_occurred_idx',
    'audit_logs_application_occurred_idx',
    'audit_logs_request_id_idx'
)`).Scan(&indexCount).Error; err != nil || indexCount != 5 {
		t.Fatalf("audit trail index count=%d error=%v", indexCount, err)
	}
	if err := db.Exec(`UPDATE audit_logs SET action = 'tampered' WHERE id = ?`, applicationAuditID).Error; err == nil {
		t.Fatal("audit update should be rejected")
	}
	if err := db.Exec(`DELETE FROM audit_logs WHERE id = ?`, applicationAuditID).Error; err == nil {
		t.Fatal("audit delete should be rejected")
	}
	assertCount(t, db, "audit_logs", 3)

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("roll back audit trail migration: %v", err)
	}
	var columnCount int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'audit_logs'
  AND column_name IN ('project_id', 'environment_id', 'application_id', 'actor_display_name', 'scope_resolution')`).Scan(&columnCount).Error; err != nil || columnCount != 0 {
		t.Fatalf("audit trail columns after rollback=%d error=%v", columnCount, err)
	}
	var permissionCount int64
	if err := db.Table("authorization_permissions").Where("key = ?", "audit.view").Count(&permissionCount).Error; err != nil || permissionCount != 0 {
		t.Fatalf("audit permission after rollback=%d error=%v", permissionCount, err)
	}

	if err := migrator.Steps(1); err != nil {
		t.Fatalf("reapply audit trail migration: %v", err)
	}
	assertAuditTrailFoundationPermissionSeed(t, db)
}

func TestAuditPlatformAdministratorPermissionMigration(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}
	migrator, err := migrationlib.NewWithSourceInstance("iofs", source, database.URL(cfg))
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _, _ = migrator.Close() })
	if err := migrator.Migrate(20260917001800); err != nil {
		t.Fatalf("apply migrations through audit trail foundation: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	assertAuditTrailFoundationPermissionSeed(t, db)
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("grant audit permission to platform administrator: %v", err)
	}
	assertAuditTrailPermissionSeed(t, db)

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("roll back platform administrator audit permission: %v", err)
	}
	assertAuditTrailFoundationPermissionSeed(t, db)
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("reapply platform administrator audit permission: %v", err)
	}
	assertAuditTrailPermissionSeed(t, db)
}

func assertAuditTrailFoundationPermissionSeed(t *testing.T, db *gorm.DB) {
	t.Helper()
	var roleCount int64
	if err := db.Raw(`SELECT count(*) FROM authorization_role_permissions
WHERE permission_key = 'audit.view'
  AND role_id IN ('00000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000102')`).Scan(&roleCount).Error; err != nil || roleCount != 2 {
		t.Fatalf("audit foundation management role count=%d error=%v", roleCount, err)
	}
	var platformAdministratorCount int64
	if err := db.Raw(`SELECT count(*) FROM authorization_role_permissions
WHERE permission_key = 'audit.view'
  AND role_id = '00000000-0000-0000-0000-000000000100'`).Scan(&platformAdministratorCount).Error; err != nil || platformAdministratorCount != 0 {
		t.Fatalf("audit foundation platform administrator role count=%d error=%v", platformAdministratorCount, err)
	}
}

func assertAuditTrailPermissionSeed(t *testing.T, db *gorm.DB) {
	t.Helper()
	var permissionCount int64
	if err := db.Raw(`SELECT count(*) FROM authorization_permissions
WHERE key = 'audit.view' AND NOT platform_only AND project_role_delegable`).Scan(&permissionCount).Error; err != nil || permissionCount != 1 {
		t.Fatalf("audit permission count=%d error=%v", permissionCount, err)
	}
	var roleCount int64
	if err := db.Raw(`SELECT count(*) FROM authorization_role_permissions
WHERE permission_key = 'audit.view'
  AND role_id IN (
    '00000000-0000-0000-0000-000000000100',
    '00000000-0000-0000-0000-000000000101',
    '00000000-0000-0000-0000-000000000102'
  )`).Scan(&roleCount).Error; err != nil || roleCount != 3 {
		t.Fatalf("audit management role count=%d error=%v", roleCount, err)
	}
	var unexpectedRoleCount int64
	if err := db.Raw(`SELECT count(*) FROM authorization_role_permissions
WHERE permission_key = 'audit.view'
  AND role_id NOT IN (
    '00000000-0000-0000-0000-000000000100',
    '00000000-0000-0000-0000-000000000101',
    '00000000-0000-0000-0000-000000000102'
  )`).Scan(&unexpectedRoleCount).Error; err != nil || unexpectedRoleCount != 0 {
		t.Fatalf("unexpected audit role count=%d error=%v", unexpectedRoleCount, err)
	}
}
