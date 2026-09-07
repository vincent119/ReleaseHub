//go:build integration

package database_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identityinfra "github.com/vincent119/ReleaseHub/Server/internal/identity/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestMigrationsAndTransactionalAuditOutbox(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	pool := config.PoolConfig{MaxOpenConnections: 7, MaxIdleConnections: 3, ConnectionLifetime: time.Minute}
	db, err := database.Open(cfg, pool)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQL pool: %v", err)
	}
	if sqlDB.Stats().MaxOpenConnections != pool.MaxOpenConnections {
		t.Fatalf("pool max open connections mismatch, got %d", sqlDB.Stats().MaxOpenConnections)
	}
	if err := db.Exec(`CREATE TABLE transaction_probes (id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create transaction probe table: %v", err)
	}

	event := newEvent(t)
	sentinel := errors.New("rollback requested")
	err = database.WithinTransaction(ctx, db, func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO transaction_probes (id) VALUES (?)`, "rollback").Error; err != nil {
			return err
		}
		if err := database.AppendAudit(ctx, tx, auditRecord()); err != nil {
			return err
		}
		if err := database.AppendOutbox(ctx, tx, event); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction should return work error, got %v", err)
	}
	assertCount(t, db, "transaction_probes", 0)
	assertCount(t, db, "audit_logs", 0)
	assertCount(t, db, "outbox_events", 0)

	err = database.WithinTransaction(ctx, db, func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO transaction_probes (id) VALUES (?)`, "commit").Error; err != nil {
			return err
		}
		if err := database.AppendAudit(ctx, tx, auditRecord()); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
	if err != nil {
		t.Fatalf("commit transactional records: %v", err)
	}
	assertCount(t, db, "transaction_probes", 1)
	assertCount(t, db, "audit_logs", 1)
	assertCount(t, db, "outbox_events", 1)

	identityRepository, err := identityinfra.NewIdentityRepository(db)
	if err != nil {
		t.Fatalf("create identity repository: %v", err)
	}
	identityService, err := identityapp.NewIdentityService(identityRepository)
	if err != nil {
		t.Fatalf("create identity service: %v", err)
	}
	user, err := identityService.ResolveOIDCIdentity(ctx, "https://issuer.example", "subject-1", "vincent")
	if err != nil {
		t.Fatalf("create OIDC identity: %v", err)
	}
	existing, err := identityService.ResolveOIDCIdentity(ctx, "https://issuer.example", "subject-1", "renamed")
	if err != nil || existing.ID != user.ID || existing.Username != "vincent" {
		t.Fatalf("existing identity was mutated: %#v %v", existing, err)
	}
	if _, err := identityService.ResolveOIDCIdentity(ctx, "https://issuer.example", "subject-2", "vincent"); err == nil {
		t.Fatal("username conflict should be rejected")
	}

	sessionRepository, err := identityinfra.NewSessionRepository(db)
	if err != nil {
		t.Fatalf("create session repository: %v", err)
	}
	sessionService, err := identityapp.NewSessionService(sessionRepository, identityapp.SystemClock{}, 30*time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("create session service: %v", err)
	}
	sessionToken, _, err := sessionService.Create(ctx, user.ID, []byte("encrypted-refresh-token"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := sessionService.Authenticate(ctx, sessionToken); err != nil {
		t.Fatalf("authenticate session: %v", err)
	}
	if err := sessionService.RevokeUserSessions(ctx, user.ID); err != nil {
		t.Fatalf("revoke user sessions: %v", err)
	}
	if _, err := sessionService.Authenticate(ctx, sessionToken); err == nil {
		t.Fatal("revoked session should be rejected")
	}
	if err := db.Exec("UPDATE users SET disabled_at = now() WHERE id = ?", user.ID).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := identityService.FindUser(ctx, user.ID); err == nil {
		t.Fatal("disabled user should be rejected immediately")
	}

	verifyAuthorizationPolicy(t, ctx, cfg, db)
}

func verifyAuthorizationPolicy(t *testing.T, ctx context.Context, cfg config.DatabaseConfig, db *gorm.DB) {
	t.Helper()
	identityRepository, _ := identityinfra.NewIdentityRepository(db)
	identityService, _ := identityapp.NewIdentityService(identityRepository)
	user, err := identityService.ResolveOIDCIdentity(ctx, "https://issuer.example", "authorization-subject", "authorization-user")
	if err != nil {
		t.Fatalf("create authorization test user: %v", err)
	}
	organizationID, projectID := uuid.New(), uuid.New()
	devEnvironmentID, productionEnvironmentID, applicationID := uuid.New(), uuid.New(), uuid.New()
	groupID := uuid.New()
	if err := db.Exec(`INSERT INTO authorization_groups (id, owner_kind, owner_id, name) VALUES (?, 'organization', ?, 'team-a')`, groupID, organizationID).Error; err != nil {
		t.Fatalf("create authorization group: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_memberships (group_id, user_id, source) VALUES (?, ?, 'manual')`, groupID, user.ID).Error; err != nil {
		t.Fatalf("create manual group membership: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_role_bindings (group_id, role_id, organization_id, scope_kind, project_id) VALUES (?, '00000000-0000-0000-0000-000000000105', ?, 'project', ?)`, groupID, organizationID, projectID).Error; err != nil {
		t.Fatalf("bind viewer role: %v", err)
	}
	devopsGroupID := uuid.New()
	if err := db.Exec(`INSERT INTO authorization_groups (id, owner_kind, owner_id, name) VALUES (?, 'project', ?, 'devops')`, devopsGroupID, projectID).Error; err != nil {
		t.Fatalf("create second authorization group: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_memberships (group_id, user_id, source) VALUES (?, ?, 'manual')`, devopsGroupID, user.ID).Error; err != nil {
		t.Fatalf("create second group membership: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_role_bindings (group_id, role_id, organization_id, scope_kind, project_id, environment_id) VALUES (?, '00000000-0000-0000-0000-000000000104', ?, 'environment', ?, ?)`, devopsGroupID, organizationID, projectID, devEnvironmentID).Error; err != nil {
		t.Fatalf("bind devops role: %v", err)
	}
	platformGroupID := uuid.New()
	if err := db.Exec(`INSERT INTO authorization_groups (id, owner_kind, name) VALUES (?, 'platform', 'platform-administrators')`, platformGroupID).Error; err != nil {
		t.Fatalf("create platform administrator group: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_memberships (group_id, user_id, source) VALUES (?, ?, 'manual')`, platformGroupID, user.ID).Error; err != nil {
		t.Fatalf("create platform administrator membership: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_platform_role_bindings (group_id, role_id) VALUES (?, '00000000-0000-0000-0000-000000000100')`, platformGroupID).Error; err != nil {
		t.Fatalf("bind platform administrator role: %v", err)
	}
	adapter, err := authzinfra.NewPolicyAdapter(db)
	if err != nil {
		t.Fatalf("create policy adapter: %v", err)
	}
	engine, err := authzinfra.NewPolicyEngine(adapter)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}
	listener, err := authzinfra.NewPolicyRevisionListener(database.PostgreSQLURL(cfg), engine)
	if err != nil {
		t.Fatalf("create policy listener: %v", err)
	}
	listenerCtx, cancelListener := context.WithCancel(ctx)
	listenerDone := make(chan error, 1)
	go func() { listenerDone <- listener.Run(listenerCtx) }()
	select {
	case <-listener.Ready():
	case <-time.After(5 * time.Second):
		cancelListener()
		t.Fatal("policy revision listener did not become ready")
	}
	t.Cleanup(func() {
		cancelListener()
		select {
		case err := <-listenerDone:
			if err != nil {
				t.Errorf("stop policy revision listener: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("policy revision listener did not stop")
		}
	})
	view, _ := authz.NewPermission("resource.view")
	devScope, _ := authz.NewApplicationScope(organizationID, projectID, devEnvironmentID, applicationID)
	productionScope, _ := authz.NewApplicationScope(organizationID, projectID, productionEnvironmentID, applicationID)
	request := authz.AuthorizationRequest{UserID: user.ID, Permission: view, Scope: devScope}
	allowed, err := engine.Authorize(ctx, request)
	if err != nil || !allowed {
		t.Fatalf("Project scope should inherit to dev Application: %v %v", allowed, err)
	}
	refresh, _ := authz.NewPermission("application.refresh")
	allowed, err = engine.Authorize(ctx, authz.AuthorizationRequest{UserID: user.ID, Permission: refresh, Scope: devScope})
	if err != nil || !allowed {
		t.Fatalf("permissions from multiple group roles should form a union: %v %v", allowed, err)
	}
	candidateView, _ := authz.NewPermission("argocd.candidate.view")
	allowed, err = engine.Authorize(ctx, authz.AuthorizationRequest{UserID: user.ID, Permission: candidateView, Scope: authz.NewPlatformScope()})
	if err != nil || !allowed {
		t.Fatalf("platform administrator should view candidate queue: %v %v", allowed, err)
	}
	previousRevision := engine.Revision()
	if err := db.Exec(`INSERT INTO authorization_deny_policies (group_id, permission_key, organization_id, scope_kind, project_id, environment_id) VALUES (?, 'resource.view', ?, 'environment', ?, ?)`, groupID, organizationID, projectID, productionEnvironmentID).Error; err != nil {
		t.Fatalf("create production deny: %v", err)
	}
	waitForPolicyRevision(t, engine, previousRevision)
	allowed, err = engine.Authorize(ctx, authz.AuthorizationRequest{UserID: user.ID, Permission: view, Scope: productionScope})
	if err != nil || allowed {
		t.Fatalf("Environment deny should override Project allow: %v %v", allowed, err)
	}

	previousRevision = engine.Revision()
	if err := db.Exec(`UPDATE authorization_groups SET disabled_at = now() WHERE id IN (?, ?)`, groupID, devopsGroupID).Error; err != nil {
		t.Fatalf("disable group: %v", err)
	}
	waitForPolicyRevision(t, engine, previousRevision)
	allowed, err = engine.Authorize(ctx, request)
	if err != nil || allowed {
		t.Fatalf("disabled group should revoke inherited permissions: %v %v", allowed, err)
	}
	if err := db.Exec(`UPDATE authorization_groups SET disabled_at = NULL WHERE id IN (?, ?)`, groupID, devopsGroupID).Error; err != nil {
		t.Fatalf("reactivate group: %v", err)
	}
	allowed, err = engine.AuthorizeFresh(ctx, request)
	if err != nil || allowed {
		t.Fatalf("reactivating group must not restore disabled bindings: %v %v", allowed, err)
	}

	projectRoleID := uuid.New()
	if err := db.Exec(`INSERT INTO authorization_roles (id, owner_kind, owner_id, name) VALUES (?, 'project', ?, 'unsafe-role')`, projectRoleID, projectID).Error; err != nil {
		t.Fatalf("create Project role: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES (?, 'project.manage')`, projectRoleID).Error; err == nil {
		t.Fatal("Project-owned role should reject management permission")
	}

	verifyOIDCGroupMapping(t, ctx, db, user.ID, organizationID)
}

func verifyOIDCGroupMapping(t *testing.T, ctx context.Context, db *gorm.DB, userID, organizationID uuid.UUID) {
	t.Helper()
	viewerGroupID := uuid.New()
	if err := db.Exec(`INSERT INTO authorization_groups (id, owner_kind, owner_id, name, oidc_viewer_only) VALUES (?, 'organization', ?, 'oidc-viewers', true)`, viewerGroupID, organizationID).Error; err != nil {
		t.Fatalf("create OIDC viewer group: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_oidc_group_mappings (issuer, provider_group, group_id) VALUES ('https://issuer.example', 'known-viewers', ?)`, viewerGroupID).Error; err != nil {
		t.Fatalf("create OIDC group mapping: %v", err)
	}
	repository, _ := authzinfra.NewOIDCGroupRepository(db)
	service, _ := authzapp.NewOIDCGroupService(repository)
	if err := service.SyncOIDCGroups(ctx, userID, "https://issuer.example", []string{"unknown-group"}); err != nil {
		t.Fatalf("synchronize unknown OIDC group: %v", err)
	}
	var count int64
	if err := db.Table("authorization_group_memberships").Where("user_id = ? AND source = 'oidc' AND active", userID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("unknown OIDC group must not create membership: %d %v", count, err)
	}
	auditBefore, outboxBefore := tableCount(t, db, "audit_logs"), tableCount(t, db, "outbox_events")
	if err := service.SyncOIDCGroups(ctx, userID, "https://issuer.example", []string{"known-viewers"}); err != nil {
		t.Fatalf("synchronize known OIDC group: %v", err)
	}
	if err := db.Table("authorization_group_memberships").Where("user_id = ? AND group_id = ? AND source = 'oidc' AND active", userID, viewerGroupID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("known OIDC viewer mapping should create one membership: %d %v", count, err)
	}
	if tableCount(t, db, "audit_logs") != auditBefore+1 || tableCount(t, db, "outbox_events") != outboxBefore+1 {
		t.Fatal("OIDC membership change must append audit and outbox records atomically")
	}
}

func waitForPolicyRevision(t *testing.T, engine *authzinfra.PolicyEngine, previous uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if engine.Revision() > previous {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("policy cache did not observe PostgreSQL revision notification")
}

func tableCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func startPostgreSQL(t *testing.T, ctx context.Context) config.DatabaseConfig {
	t.Helper()
	container, err := testcontainers.Run(ctx, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_USER":     "releasehub",
			"POSTGRES_PASSWORD": "releasehub-test-password",
			"POSTGRES_DB":       "releasehub",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForAll(
			wait.ForListeningPort("5432/tcp"),
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		).WithDeadline(time.Minute)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get PostgreSQL host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("get PostgreSQL port: %v", err)
	}
	portNumber, err := strconv.Atoi(port.Port())
	if err != nil {
		t.Fatalf("parse PostgreSQL port: %v", err)
	}
	return config.DatabaseConfig{
		Server: host, Port: portNumber, User: "releasehub", Password: "releasehub-test-password",
		Database: "releasehub", SSLMode: "disable", Timezone: "UTC",
	}
}

func newEvent(t *testing.T) domain.Event {
	t.Helper()
	event, err := domain.NewEvent(
		"ApplicationOnboarded", "application", "app-1", []byte(`{"state":"managed"}`), time.Now(),
	)
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	return event
}

func auditRecord() database.AuditRecord {
	return database.AuditRecord{
		OccurredAt: time.Now(), Action: "application.onboard", ResourceType: "application", ResourceID: "app-1",
		RequestID: "request-1", Metadata: map[string]any{"source": "integration-test"},
	}
}

func assertCount(t *testing.T, db *gorm.DB, table string, expected int64) {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != expected {
		t.Fatalf("%s count mismatch, got %d want %d", table, count, expected)
	}
}
