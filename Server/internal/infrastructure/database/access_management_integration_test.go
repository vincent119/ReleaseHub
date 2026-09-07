//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

func TestAccessManagementRepositoryLoadsNullableOwnersAndPolicyRecords(t *testing.T) {
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

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectID, userID, groupID, membershipID, bindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO projects (id, organization_id, name) VALUES (?, ?, 'Payment')`, []any{projectID, organizationID}},
		{`INSERT INTO users (id, username) VALUES (?, 'project-manager')`, []any{userID}},
		{`INSERT INTO authorization_groups (id, owner_kind, owner_id, name) VALUES (?, 'project', ?, 'payment-managers')`, []any{groupID, projectID}},
		{`INSERT INTO authorization_group_memberships (id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, []any{membershipID, groupID, userID}},
		{`INSERT INTO authorization_group_role_bindings (id, group_id, role_id, organization_id, scope_kind, project_id) VALUES (?, ?, '00000000-0000-0000-0000-000000000101', ?, 'project', ?)`, []any{bindingID, groupID, organizationID, projectID}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed access management: %v", err)
		}
	}

	repository, err := authzinfra.NewAccessManagementRepository(db)
	if err != nil {
		t.Fatalf("create access management repository: %v", err)
	}
	scopes, err := repository.ListProjectScopes(ctx)
	if err != nil || len(scopes) != 1 || scopes[0].ProjectID != projectID {
		t.Fatalf("Project scopes = %#v, error = %v", scopes, err)
	}
	value, err := repository.LoadAccessSnapshot(ctx)
	if err != nil {
		t.Fatalf("load access snapshot: %v", err)
	}
	if len(value.Users) != 1 || len(value.Groups) != 1 || len(value.Memberships) != 1 || len(value.Bindings) != 1 || len(value.Roles) < 6 {
		t.Fatalf("access snapshot did not preserve policy records: %#v", value)
	}
	var platformRoleFound bool
	for _, role := range value.Roles {
		if role.SystemKey != nil && *role.SystemKey == "platform_administrator" && role.OwnerID == nil {
			platformRoleFound = true
		}
	}
	if !platformRoleFound {
		t.Fatal("platform Role nullable owner was not preserved")
	}

	mutation := authzapp.AccessMutation{ActorID: userID, RequestID: "access-integration"}
	createdGroup, err := repository.CreateGroup(ctx, mutation, authzapp.CreateGroupInput{OwnerKind: "project", OwnerID: &projectID, Name: "release-operators"})
	if err != nil {
		t.Fatalf("create Group: %v", err)
	}
	createdRole, err := repository.CreateRole(ctx, mutation, authzapp.CreateRoleInput{OwnerKind: "project", OwnerID: &projectID, Name: "release-viewer", Permissions: []string{"resource.view"}})
	if err != nil {
		t.Fatalf("create Role: %v", err)
	}
	if _, err := repository.AddMembership(ctx, mutation, createdGroup.ID, userID); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if _, err := repository.CreateBinding(ctx, mutation, authzapp.CreateBindingInput{GroupID: createdGroup.ID, RoleID: createdRole.ID, OrganizationID: organizationID, ScopeKind: "project", ProjectID: projectID}); err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if _, err := repository.CreateDeny(ctx, mutation, authzapp.CreateDenyInput{GroupID: createdGroup.ID, Permission: "application.refresh", OrganizationID: organizationID, ScopeKind: "project", ProjectID: projectID}); err != nil {
		t.Fatalf("create deny: %v", err)
	}
	if err := repository.DisableGroup(ctx, mutation, createdGroup.ID); err != nil {
		t.Fatalf("disable Group: %v", err)
	}
	var activePolicies int64
	if err := db.Raw(`SELECT (SELECT count(*) FROM authorization_group_role_bindings WHERE group_id = ? AND active) + (SELECT count(*) FROM authorization_deny_policies WHERE group_id = ? AND active)`, createdGroup.ID, createdGroup.ID).Scan(&activePolicies).Error; err != nil || activePolicies != 0 {
		t.Fatalf("active Group policies = %d, error = %v", activePolicies, err)
	}

	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, refresh_token_ciphertext, identity_verified_at, created_at, last_seen_at, idle_expires_at, absolute_expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New(), userID, []byte("token"), []byte("csrf"), []byte("refresh"), now, now, now, now.Add(time.Minute), now.Add(time.Hour)).Error; err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	if err := repository.DisableUser(ctx, mutation, userID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	var activeSessions int64
	if err := db.Table("sessions").Where("user_id = ? AND revoked_at IS NULL", userID).Count(&activeSessions).Error; err != nil || activeSessions != 0 {
		t.Fatalf("active Sessions = %d, error = %v", activeSessions, err)
	}
	var auditCount, outboxCount int64
	if err := db.Table("audit_logs").Where("request_id = ?", mutation.RequestID).Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit records: %v", err)
	}
	if err := db.Table("outbox_events").Where("aggregate_type LIKE 'authorization_%' OR event_type = 'identity.user.disabled'").Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox records: %v", err)
	}
	if auditCount != 7 || outboxCount < 7 {
		t.Fatalf("audit/outbox counts = %d/%d", auditCount, outboxCount)
	}
}
