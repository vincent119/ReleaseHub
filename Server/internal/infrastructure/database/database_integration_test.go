//go:build integration

package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	migrationlib "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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

	localRepository, err := identityinfra.NewLocalAuthRepository(db)
	if err != nil {
		t.Fatalf("create local authentication repository: %v", err)
	}
	localService, err := identityapp.NewLocalAuthService(localRepository, sessionService)
	if err != nil {
		t.Fatalf("create local authentication service: %v", err)
	}
	if err := localService.Bootstrap(ctx, "admin"); err != nil {
		t.Fatalf("bootstrap local manager: %v", err)
	}
	if err := localService.Bootstrap(ctx, "must-not-overwrite"); err != nil {
		t.Fatalf("repeat local manager bootstrap: %v", err)
	}
	manager, mustChange, _, _, _, err := localService.Login(ctx, "admin", "admin")
	if err != nil || manager.Username != "admin" || !mustChange {
		t.Fatalf("initial manager login = %#v, mustChange=%t, error=%v", manager, mustChange, err)
	}
	if err := localService.ChangePassword(ctx, manager.ID, "admin", "changed-password"); err != nil {
		t.Fatalf("change initial manager password: %v", err)
	}
	if _, _, _, _, _, err := localService.Login(ctx, "admin", "admin"); err == nil {
		t.Fatal("initial manager password should no longer authenticate")
	}
	_, mustChange, _, _, _, err = localService.Login(ctx, "admin", "changed-password")
	if err != nil || mustChange {
		t.Fatalf("changed manager login mustChange=%t, error=%v", mustChange, err)
	}
	var managerBindings int64
	if err := db.Raw(`SELECT count(*) FROM authorization_platform_role_bindings b
		JOIN authorization_group_memberships m ON m.group_id = b.group_id
		WHERE m.user_id = ? AND m.active AND b.active AND b.role_id = '00000000-0000-0000-0000-000000000100'`, manager.ID).Scan(&managerBindings).Error; err != nil || managerBindings != 1 {
		t.Fatalf("initial manager platform binding count=%d, error=%v", managerBindings, err)
	}
	createdUser, err := localService.CreateUser(ctx, manager.ID, "create-local-user", "operator", "initial-password")
	if err != nil {
		t.Fatalf("create local user: %v", err)
	}
	var credential struct {
		MustChangePassword bool
		PasswordHash       []byte
	}
	if err := db.Raw(`SELECT must_change_password, password_hash FROM local_credentials WHERE user_id = ?`, createdUser.ID).Scan(&credential).Error; err != nil {
		t.Fatalf("read created local credential: %v", err)
	}
	if !credential.MustChangePassword || string(credential.PasswordHash) == "initial-password" {
		t.Fatalf("created local credential is not protected: mustChange=%t", credential.MustChangePassword)
	}
	var createdMemberships int64
	if err := db.Raw(`SELECT count(*) FROM authorization_group_memberships WHERE user_id = ?`, createdUser.ID).Scan(&createdMemberships).Error; err != nil || createdMemberships != 0 {
		t.Fatalf("new local user received automatic access: memberships=%d error=%v", createdMemberships, err)
	}
	_, createdMustChange, _, _, _, err := localService.Login(ctx, "operator", "initial-password")
	if err != nil || !createdMustChange {
		t.Fatalf("created local user login mustChange=%t error=%v", createdMustChange, err)
	}
	if _, err := localService.CreateUser(ctx, manager.ID, "duplicate-local-user", "operator", "another-password"); !errors.Is(err, identityapp.ErrLocalUsernameConflict) {
		t.Fatalf("duplicate local username error=%v", err)
	}

	verifyAuthorizationPolicy(t, ctx, cfg, db)
	verifyAccessAndLocalAuthenticationMigrationRollback(t, cfg, db)
}

func verifyAccessAndLocalAuthenticationMigrationRollback(t *testing.T, cfg config.DatabaseConfig, db *gorm.DB) {
	t.Helper()
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}
	migrator, err := migrationlib.NewWithSourceInstance("iofs", source, database.URL(cfg))
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _, _ = migrator.Close() })
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("roll back deployment schedule migration: %v", err)
	}
	var scheduleRemoved bool
	if err := db.Raw(`SELECT to_regclass('public.deployment_schedule_policies') IS NULL`).Scan(&scheduleRemoved).Error; err != nil || !scheduleRemoved {
		t.Fatalf("deployment schedule migration was not removed: removed=%t error=%v", scheduleRemoved, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("roll back access lifecycle migration: %v", err)
	}
	var systemKeyColumns int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'authorization_groups' AND column_name = 'system_key'`).Scan(&systemKeyColumns).Error; err != nil || systemKeyColumns != 0 {
		t.Fatalf("access lifecycle migration was not removed: columns=%d error=%v", systemKeyColumns, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("roll back local authentication migration: %v", err)
	}
	var removed bool
	if err := db.Raw(`SELECT to_regclass('public.local_credentials') IS NULL`).Scan(&removed).Error; err != nil || !removed {
		t.Fatalf("local credential table was not removed: removed=%t error=%v", removed, err)
	}
	if err := migrator.Steps(3); err != nil {
		t.Fatalf("reapply local, access lifecycle, and deployment schedule migrations: %v", err)
	}
	var restored bool
	if err := db.Raw(`SELECT to_regclass('public.local_credentials') IS NOT NULL`).Scan(&restored).Error; err != nil || !restored {
		t.Fatalf("local credential table was not restored: restored=%t error=%v", restored, err)
	}
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'authorization_groups' AND column_name = 'system_key'`).Scan(&systemKeyColumns).Error; err != nil || systemKeyColumns != 1 {
		t.Fatalf("access lifecycle migration was not restored: columns=%d error=%v", systemKeyColumns, err)
	}
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
	if port := os.Getenv("RELEASEHUB_TEST_POSTGRES_PORT"); port != "" {
		return startExternalPostgreSQL(t, port)
	}
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

func startExternalPostgreSQL(t *testing.T, portValue string) config.DatabaseConfig {
	t.Helper()
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("parse external PostgreSQL port: %v", err)
	}
	user := os.Getenv("RELEASEHUB_TEST_POSTGRES_USER")
	if user == "" {
		t.Fatal("RELEASEHUB_TEST_POSTGRES_USER is required with RELEASEHUB_TEST_POSTGRES_PORT")
	}
	base := config.DatabaseConfig{
		Server: "127.0.0.1", Port: port, User: user,
		Database: "postgres", SSLMode: "disable", Timezone: "UTC",
	}
	name := "releasehub_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin := openExternalPostgreSQLAdmin(t, base)
	if err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %q`, name)).Error; err != nil {
		t.Fatalf("create isolated PostgreSQL database: %v", err)
	}
	if err := database.Close(admin); err != nil {
		t.Fatalf("close PostgreSQL administration connection: %v", err)
	}
	adminConfig := base
	t.Cleanup(func() { dropExternalPostgreSQLDatabase(t, adminConfig, name) })
	base.Database = name
	return base
}

func openExternalPostgreSQLAdmin(t *testing.T, cfg config.DatabaseConfig) *gorm.DB {
	t.Helper()
	db, err := database.Open(cfg, config.PoolConfig{MaxOpenConnections: 2, MaxIdleConnections: 1})
	if err != nil {
		t.Fatalf("open external PostgreSQL administration connection: %v", err)
	}
	return db
}

func dropExternalPostgreSQLDatabase(t *testing.T, cfg config.DatabaseConfig, name string) {
	t.Helper()
	admin := openExternalPostgreSQLAdmin(t, cfg)
	defer func() { _ = database.Close(admin) }()
	if err := admin.Exec(fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, name)).Error; err != nil {
		t.Errorf("drop isolated PostgreSQL database: %v", err)
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
