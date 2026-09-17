//go:build integration

package database_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	auditinfra "github.com/vincent119/ReleaseHub/Server/internal/audit/infrastructure"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestAuditQueryAuthorizationAndStablePagination(t *testing.T) {
	ctx := context.Background()
	cfg := startPostgreSQL(t, ctx)
	if err := migrations.Up(database.URL(cfg)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(cfg, config.PoolConfig{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	organizationID, projectID, applicationID := uuid.New(), uuid.New(), uuid.New()
	allowedEnvironmentID, deniedEnvironmentID := uuid.New(), uuid.New()
	projectUserID, platformUserID, environmentUserID, applicationUserID, disabledUserID, noPermissionUserID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	projectGroupID, platformGroupID, environmentGroupID, applicationGroupID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO users (id, username) VALUES
(?, 'audit-project-user'), (?, 'audit-platform-user'), (?, 'audit-environment-user'),
(?, 'audit-application-user'), (?, 'audit-disabled-user'), (?, 'audit-no-permission-user')`,
		projectUserID, platformUserID, environmentUserID, applicationUserID, disabledUserID, noPermissionUserID).Error; err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if err := db.Exec(`INSERT INTO organizations (id, name) VALUES (?, 'audit-query-organization')`, organizationID).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if err := db.Exec(`INSERT INTO projects (id, organization_id, name) VALUES (?, ?, 'audit-query-project')`, projectID, organizationID).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO environments (id, organization_id, project_id, name, environment_type) VALUES
(?, ?, ?, 'audit-allowed', 'Development'), (?, ?, ?, 'audit-denied', 'Production')`,
		allowedEnvironmentID, organizationID, projectID, deniedEnvironmentID, organizationID, projectID).Error; err != nil {
		t.Fatalf("insert environments: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (
id, organization_id, project_id, environment_id, name,
argocd_namespace, argocd_application_name, argocd_project,
destination_server, destination_namespace, source_repo_url, source_target_revision, source_path
) VALUES (?, ?, ?, ?, 'audit-application', 'argocd', 'audit-application', 'default',
'https://kubernetes.default.svc', 'audit', 'https://example.invalid/audit.git', 'main', 'deploy/audit')`,
		applicationID, organizationID, projectID, allowedEnvironmentID).Error; err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_groups (id, owner_kind, owner_id, name) VALUES
(?, 'project', ?, 'audit-project-group'), (?, 'platform', NULL, 'audit-platform-group'),
(?, 'project', ?, 'audit-environment-group'), (?, 'project', ?, 'audit-application-group')`,
		projectGroupID, projectID, platformGroupID, environmentGroupID, projectID, applicationGroupID, projectID).Error; err != nil {
		t.Fatalf("insert groups: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_memberships (group_id, user_id, source) VALUES
(?, ?, 'manual'), (?, ?, 'manual'), (?, ?, 'manual'), (?, ?, 'manual'), (?, ?, 'manual')`,
		projectGroupID, projectUserID, platformGroupID, platformUserID, environmentGroupID, environmentUserID,
		applicationGroupID, applicationUserID, projectGroupID, disabledUserID).Error; err != nil {
		t.Fatalf("insert memberships: %v", err)
	}
	projectManagerRoleID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	platformAdministratorRoleID := uuid.MustParse("00000000-0000-0000-0000-000000000100")
	if err := db.Exec(`INSERT INTO authorization_group_role_bindings
(group_id, role_id, organization_id, scope_kind, project_id) VALUES (?, ?, ?, 'project', ?)`,
		projectGroupID, projectManagerRoleID, organizationID, projectID).Error; err != nil {
		t.Fatalf("insert project binding: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_platform_role_bindings (group_id, role_id) VALUES (?, ?)`, platformGroupID, platformAdministratorRoleID).Error; err != nil {
		t.Fatalf("insert platform binding: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_role_bindings
(group_id, role_id, organization_id, scope_kind, project_id, environment_id) VALUES (?, ?, ?, 'environment', ?, ?)`,
		environmentGroupID, projectManagerRoleID, organizationID, projectID, allowedEnvironmentID).Error; err != nil {
		t.Fatalf("insert environment binding: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_group_role_bindings
(group_id, role_id, organization_id, scope_kind, project_id, environment_id, application_id) VALUES (?, ?, ?, 'application', ?, ?, ?)`,
		applicationGroupID, projectManagerRoleID, organizationID, projectID, allowedEnvironmentID, applicationID).Error; err != nil {
		t.Fatalf("insert application binding: %v", err)
	}
	if err := db.Exec(`INSERT INTO authorization_deny_policies
(group_id, permission_key, organization_id, scope_kind, project_id, environment_id)
VALUES (?, 'audit.view', ?, 'environment', ?, ?)`, projectGroupID, organizationID, projectID, deniedEnvironmentID).Error; err != nil {
		t.Fatalf("insert explicit deny: %v", err)
	}

	baseTime := time.Date(2026, 9, 17, 2, 0, 0, 0, time.UTC)
	projectEventID, allowedEventID, deniedEventID, applicationEventID, unresolvedEventID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	insertAuditQueryFixture(t, db, projectEventID, baseTime, &organizationID, &projectID, nil, nil, "resolved", "project.updated")
	insertAuditQueryFixture(t, db, allowedEventID, baseTime, &organizationID, &projectID, &allowedEnvironmentID, nil, "resolved", "environment.updated")
	insertAuditQueryFixture(t, db, deniedEventID, baseTime, &organizationID, &projectID, &deniedEnvironmentID, nil, "resolved", "secret.denied")
	insertAuditQueryFixture(t, db, applicationEventID, baseTime, &organizationID, &projectID, &allowedEnvironmentID, &applicationID, "resolved", "application.updated")
	insertAuditQueryFixture(t, db, unresolvedEventID, baseTime, nil, nil, nil, nil, "unresolved", "legacy.updated")

	policyAdapter, err := authzinfra.NewPolicyAdapter(db)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := authzinfra.NewPolicyEngine(policyAdapter)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}
	repository, err := auditinfra.NewPostgresRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	visibility, err := auditinfra.NewVisibilityResolver(db)
	if err != nil {
		t.Fatal(err)
	}
	cursors, err := auditdomain.NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := auditapp.NewQueryService(repository, engine, visibility, visibility, cursors)
	if err != nil {
		t.Fatal(err)
	}
	projectCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: projectUserID})
	if err != nil || !projectCapabilities.Visible || len(projectCapabilities.ScopeRoots) != 1 || projectCapabilities.ScopeRoots[0].Scope.Kind != authz.ScopeProject {
		t.Fatalf("project capabilities = %#v, error = %v", projectCapabilities, err)
	}
	platformCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: platformUserID})
	if err != nil || !platformCapabilities.Visible || len(platformCapabilities.ScopeRoots) != 1 || platformCapabilities.ScopeRoots[0].Scope.Kind != authz.ScopePlatform {
		t.Fatalf("platform capabilities = %#v, error = %v", platformCapabilities, err)
	}
	environmentCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: environmentUserID})
	if err != nil || !environmentCapabilities.Visible || len(environmentCapabilities.ScopeRoots) != 1 || environmentCapabilities.ScopeRoots[0].Scope.Kind != authz.ScopeEnvironment {
		t.Fatalf("environment capabilities = %#v, error = %v", environmentCapabilities, err)
	}
	applicationCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: applicationUserID})
	if err != nil || !applicationCapabilities.Visible || len(applicationCapabilities.ScopeRoots) != 1 || applicationCapabilities.ScopeRoots[0].Scope.Kind != authz.ScopeApplication {
		t.Fatalf("application capabilities = %#v, error = %v", applicationCapabilities, err)
	}
	disabledCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: disabledUserID, Disabled: true})
	if err != nil || disabledCapabilities.Visible || len(disabledCapabilities.ScopeRoots) != 0 {
		t.Fatalf("disabled capabilities = %#v, error = %v", disabledCapabilities, err)
	}
	defaultDenyCapabilities, err := service.Capabilities(ctx, auditapp.Principal{UserID: noPermissionUserID})
	if err != nil || defaultDenyCapabilities.Visible || len(defaultDenyCapabilities.ScopeRoots) != 0 {
		t.Fatalf("default deny capabilities = %#v, error = %v", defaultDenyCapabilities, err)
	}
	projectScope, err := authz.NewProjectScope(organizationID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	optionFilter := auditdomain.FilterOptionFilter{
		Scope: projectScope, OccurredFrom: baseTime.Add(-time.Hour), OccurredTo: baseTime.Add(time.Hour),
		Field: auditdomain.FilterOptionAction, Search: "updated", Limit: 2,
	}
	actionOptions, err := service.FilterOptions(ctx, auditapp.Principal{UserID: projectUserID}, optionFilter)
	if err != nil || len(actionOptions) != 2 || actionOptions[0].Value != "application.updated" || actionOptions[1].Value != "environment.updated" {
		t.Fatalf("project action options = %#v, error = %v", actionOptions, err)
	}
	optionFilter.Search = "denied"
	actionOptions, err = service.FilterOptions(ctx, auditapp.Principal{UserID: projectUserID}, optionFilter)
	if err != nil || len(actionOptions) != 0 {
		t.Fatalf("denied action options = %#v, error = %v", actionOptions, err)
	}
	optionFilter.Field, optionFilter.Search = auditdomain.FilterOptionActor, "system"
	actorOptions, err := service.FilterOptions(ctx, auditapp.Principal{UserID: projectUserID}, optionFilter)
	if err != nil || len(actorOptions) != 1 || actorOptions[0].Value != "system" || actorOptions[0].Label != "ReleaseHub System" {
		t.Fatalf("actor options = %#v, error = %v", actorOptions, err)
	}
	filter := auditdomain.QueryFilter{Scope: projectScope, OccurredFrom: baseTime.Add(-time.Hour), OccurredTo: baseTime.Add(time.Hour), Limit: 1}
	first, err := service.List(ctx, auditapp.Principal{UserID: projectUserID}, filter, "")
	if err != nil || len(first.Items) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %#v, error = %v", first, err)
	}
	newEventID := uuid.New()
	insertAuditQueryFixture(t, db, newEventID, baseTime.Add(time.Minute), &organizationID, &projectID, nil, nil, "resolved", "project.created")
	visible := map[uuid.UUID]bool{first.Items[0].ID: true}
	cursor, hasMore := first.NextCursor, first.HasMore
	for pageNumber := 2; hasMore && pageNumber <= 10; pageNumber++ {
		page, pageErr := service.List(ctx, auditapp.Principal{UserID: projectUserID}, filter, cursor)
		if pageErr != nil || len(page.Items) == 0 {
			t.Fatalf("page %d = %#v, error = %v", pageNumber, page, pageErr)
		}
		for _, item := range page.Items {
			if visible[item.ID] {
				t.Fatalf("duplicate event across cursor pages: %s", item.ID)
			}
			visible[item.ID] = true
		}
		cursor, hasMore = page.NextCursor, page.HasMore
	}
	if hasMore || len(visible) != 3 || !visible[projectEventID] || !visible[allowedEventID] || !visible[applicationEventID] || visible[deniedEventID] || visible[unresolvedEventID] || visible[newEventID] {
		t.Fatalf("project-visible events = %#v", visible)
	}

	environmentScope, err := authz.NewEnvironmentScope(organizationID, projectID, allowedEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	environmentPage, err := service.List(ctx, auditapp.Principal{UserID: environmentUserID}, auditdomain.QueryFilter{
		Scope: environmentScope, OccurredFrom: baseTime.Add(-time.Hour), OccurredTo: baseTime.Add(time.Hour), Limit: 100,
	}, "")
	if err != nil || len(environmentPage.Items) != 2 {
		t.Fatalf("environment page = %#v, error = %v", environmentPage, err)
	}
	deniedScope, err := authz.NewEnvironmentScope(organizationID, projectID, deniedEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(ctx, auditapp.Principal{UserID: environmentUserID}, auditdomain.QueryFilter{Scope: deniedScope}, ""); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("environment actor cross-scope error = %v", err)
	}
	applicationScope, err := authz.NewApplicationScope(organizationID, projectID, allowedEnvironmentID, applicationID)
	if err != nil {
		t.Fatal(err)
	}
	applicationPage, err := service.List(ctx, auditapp.Principal{UserID: applicationUserID}, auditdomain.QueryFilter{
		Scope: applicationScope, OccurredFrom: baseTime.Add(-time.Hour), OccurredTo: baseTime.Add(time.Hour), Limit: 100,
	}, "")
	if err != nil || len(applicationPage.Items) != 1 || applicationPage.Items[0].ID != applicationEventID {
		t.Fatalf("application page = %#v, error = %v", applicationPage, err)
	}
	if _, err := service.List(ctx, auditapp.Principal{UserID: applicationUserID}, auditdomain.QueryFilter{Scope: environmentScope}, ""); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("application actor ancestor-scope error = %v", err)
	}

	platformPage, err := service.List(ctx, auditapp.Principal{UserID: platformUserID}, auditdomain.QueryFilter{
		Scope: authz.NewPlatformScope(), OccurredFrom: baseTime.Add(-time.Hour), OccurredTo: baseTime.Add(time.Hour), Limit: 100,
	}, "")
	if err != nil || len(platformPage.Items) != 6 {
		t.Fatalf("platform page count=%d error=%v", len(platformPage.Items), err)
	}
	assertAuditQueryPlanUsesProjectIndex(t, db, projectID, filter.OccurredFrom, filter.OccurredTo)
}

func insertAuditQueryFixture(t *testing.T, db *gorm.DB, id uuid.UUID, occurredAt time.Time, organizationID, projectID, environmentID, applicationID *uuid.UUID, resolution, action string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO audit_logs
(id, occurred_at, organization_id, project_id, environment_id, application_id, scope_resolution, action, resource_type, resource_id, request_id, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'audit_fixture', ?, 'audit-query-request', '{"safe":"value"}'::jsonb)`,
		id, occurredAt, organizationID, projectID, environmentID, applicationID, resolution, action, id.String()).Error; err != nil {
		t.Fatalf("insert audit fixture: %v", err)
	}
}

func assertAuditQueryPlanUsesProjectIndex(t *testing.T, db *gorm.DB, projectID uuid.UUID, from, to time.Time) {
	t.Helper()
	var lines []string
	err := db.Transaction(func(tx *gorm.DB) error {
		if execErr := tx.Exec(`SET LOCAL enable_seqscan = off`).Error; execErr != nil {
			return execErr
		}
		rows, queryErr := tx.Raw(`EXPLAIN (FORMAT TEXT)
SELECT id FROM audit_logs WHERE scope_resolution = 'resolved' AND project_id = ?
AND occurred_at >= ? AND occurred_at < ? ORDER BY occurred_at DESC, id DESC LIMIT 21`, projectID, from, to).Rows()
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if scanErr := rows.Scan(&line); scanErr != nil {
				return scanErr
			}
			lines = append(lines, line)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("explain audit query: %v", err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "audit_logs_project_occurred_idx") {
		t.Fatalf("audit query plan does not use project index: %v", lines)
	}
}
