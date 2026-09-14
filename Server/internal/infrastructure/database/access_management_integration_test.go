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
	platformUserID, platformGroupID, platformMembershipID, platformBindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO projects (id, organization_id, name) VALUES (?, ?, 'Payment')`, []any{projectID, organizationID}},
		{`INSERT INTO users (id, username) VALUES (?, 'project-manager')`, []any{userID}},
		{`INSERT INTO authorization_groups (id, owner_kind, owner_id, name) VALUES (?, 'project', ?, 'payment-managers')`, []any{groupID, projectID}},
		{`INSERT INTO authorization_group_memberships (id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, []any{membershipID, groupID, userID}},
		{`INSERT INTO authorization_group_role_bindings (id, group_id, role_id, organization_id, scope_kind, project_id) VALUES (?, ?, '00000000-0000-0000-0000-000000000101', ?, 'project', ?)`, []any{bindingID, groupID, organizationID, projectID}},
		{`INSERT INTO users (id, username) VALUES (?, 'platform-manager')`, []any{platformUserID}},
		{`INSERT INTO authorization_groups (id, owner_kind, name) VALUES (?, 'platform', 'integration-platform-managers')`, []any{platformGroupID}},
		{`INSERT INTO authorization_group_memberships (id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, []any{platformMembershipID, platformGroupID, platformUserID}},
		{`INSERT INTO authorization_platform_role_bindings (id, group_id, role_id) VALUES (?, ?, '00000000-0000-0000-0000-000000000100')`, []any{platformBindingID, platformGroupID}},
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
	guardMutation := authzapp.AccessMutation{ActorID: platformUserID, RequestID: "last-platform-manager-guard"}
	if err := repository.RevokeMembership(ctx, guardMutation, platformMembershipID); !errors.Is(err, authzapp.ErrLastPlatformManager) {
		t.Fatalf("last platform membership revoke error = %v", err)
	}
	var protectedMembershipActive bool
	if err := db.Table("authorization_group_memberships").Select("active").Where("id = ?", platformMembershipID).Scan(&protectedMembershipActive).Error; err != nil || !protectedMembershipActive {
		t.Fatalf("protected platform membership active = %t, error = %v", protectedMembershipActive, err)
	}
	if err := repository.RevokeBinding(ctx, guardMutation, platformBindingID); !errors.Is(err, authzapp.ErrLastPlatformManager) {
		t.Fatalf("last platform binding revoke error = %v", err)
	}
	var protectedBindingActive bool
	if err := db.Table("authorization_platform_role_bindings").Select("active").Where("id = ?", platformBindingID).Scan(&protectedBindingActive).Error; err != nil || !protectedBindingActive {
		t.Fatalf("protected platform binding active = %t, error = %v", protectedBindingActive, err)
	}
	scopes, err := repository.ListProjectScopes(ctx)
	if err != nil || len(scopes) != 1 || scopes[0].ProjectID != projectID {
		t.Fatalf("Project scopes = %#v, error = %v", scopes, err)
	}
	value, err := repository.LoadAccessSnapshot(ctx)
	if err != nil {
		t.Fatalf("load access snapshot: %v", err)
	}
	if len(value.Users) != 2 || len(value.Groups) != 2 || len(value.Memberships) != 2 || len(value.Bindings) != 2 || len(value.Roles) < 6 {
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
	createdMembership, err := repository.AddMembership(ctx, mutation, createdGroup.ID, userID)
	if err != nil {
		t.Fatalf("add membership: %v", err)
	}
	candidateAlphaID, candidateBravoID, disabledCandidateID := uuid.New(), uuid.New(), uuid.New()
	candidateSeeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, username) VALUES (?, 'membership-candidate-alpha')`, []any{candidateAlphaID}},
		{`INSERT INTO users (id, username) VALUES (?, 'membership-candidate-bravo')`, []any{candidateBravoID}},
		{`INSERT INTO users (id, username, disabled_at) VALUES (?, 'membership-candidate-disabled', now())`, []any{disabledCandidateID}},
	}
	for _, statement := range candidateSeeds {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed membership candidates: %v", err)
		}
	}
	firstCandidates, err := repository.FindMembershipCandidates(ctx, createdGroup.ID, "membership-candidate-", nil, 1)
	if err != nil || len(firstCandidates) != 1 || firstCandidates[0].ID != candidateAlphaID {
		t.Fatalf("first membership candidate page = %#v, error = %v", firstCandidates, err)
	}
	secondCandidates, err := repository.FindMembershipCandidates(ctx, createdGroup.ID, "membership-candidate-", &authzapp.MembershipCandidateCursor{Username: firstCandidates[0].Username, UserID: firstCandidates[0].ID}, 2)
	if err != nil || len(secondCandidates) != 1 || secondCandidates[0].ID != candidateBravoID {
		t.Fatalf("second membership candidate page = %#v, error = %v", secondCandidates, err)
	}
	allCandidates, err := repository.FindMembershipCandidates(ctx, createdGroup.ID, "", nil, 100)
	if err != nil {
		t.Fatalf("list membership candidates without query: %v", err)
	}
	assertCandidatePresence(t, allCandidates, candidateAlphaID, true)
	assertCandidatePresence(t, allCandidates, candidateBravoID, true)
	assertCandidatePresence(t, allCandidates, disabledCandidateID, false)
	assertCandidatePresence(t, allCandidates, userID, false)
	if _, err := repository.AddMembership(ctx, mutation, createdGroup.ID, candidateAlphaID); err != nil {
		t.Fatalf("add selected membership candidate: %v", err)
	}
	remainingCandidates, err := repository.FindMembershipCandidates(ctx, createdGroup.ID, "membership-candidate-", nil, 10)
	if err != nil {
		t.Fatalf("reload membership candidates after add: %v", err)
	}
	assertCandidatePresence(t, remainingCandidates, candidateAlphaID, false)
	assertCandidatePresence(t, remainingCandidates, candidateBravoID, true)
	createdBinding, err := repository.CreateBinding(ctx, mutation, authzapp.CreateBindingInput{GroupID: createdGroup.ID, RoleID: createdRole.ID, OrganizationID: organizationID, ScopeKind: "project", ProjectID: projectID})
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	createdDeny, err := repository.CreateDeny(ctx, mutation, authzapp.CreateDenyInput{GroupID: createdGroup.ID, Permission: "application.refresh", OrganizationID: organizationID, ScopeKind: "project", ProjectID: projectID})
	if err != nil {
		t.Fatalf("create deny: %v", err)
	}
	if err := repository.RevokeMembership(ctx, mutation, createdMembership.ID); err != nil {
		t.Fatalf("revoke membership: %v", err)
	}
	var revisionBeforeReactivation int64
	if err := db.Raw(`SELECT revision FROM authorization_policy_revision WHERE singleton`).Scan(&revisionBeforeReactivation).Error; err != nil {
		t.Fatalf("read policy revision before membership reactivation: %v", err)
	}
	reactivatedMembership, err := repository.AddMembership(ctx, mutation, createdGroup.ID, userID)
	if err != nil {
		t.Fatalf("reactivate membership: %v", err)
	}
	if reactivatedMembership.ID != createdMembership.ID || !reactivatedMembership.Active {
		t.Fatalf("reactivated membership = %#v, original ID = %s", reactivatedMembership, createdMembership.ID)
	}
	reactivatedCandidates, err := repository.FindMembershipCandidates(ctx, createdGroup.ID, "project-manager", nil, 10)
	if err != nil {
		t.Fatalf("reload candidates after membership reactivation: %v", err)
	}
	assertCandidatePresence(t, reactivatedCandidates, userID, false)
	var revisionAfterReactivation int64
	if err := db.Raw(`SELECT revision FROM authorization_policy_revision WHERE singleton`).Scan(&revisionAfterReactivation).Error; err != nil || revisionAfterReactivation != revisionBeforeReactivation+1 {
		t.Fatalf("policy revision after membership reactivation = %d, before = %d, error = %v", revisionAfterReactivation, revisionBeforeReactivation, err)
	}
	if _, err := repository.AddMembership(ctx, mutation, createdGroup.ID, userID); !errors.Is(err, authzapp.ErrAccessManagementConflict) {
		t.Fatalf("add active membership error = %v", err)
	}
	var revisionAfterConflict int64
	if err := db.Raw(`SELECT revision FROM authorization_policy_revision WHERE singleton`).Scan(&revisionAfterConflict).Error; err != nil || revisionAfterConflict != revisionAfterReactivation {
		t.Fatalf("policy revision after membership conflict = %d, want %d, error = %v", revisionAfterConflict, revisionAfterReactivation, err)
	}
	if err := repository.RevokeMembership(ctx, mutation, createdMembership.ID); err != nil {
		t.Fatalf("revoke reactivated membership: %v", err)
	}
	if err := repository.RevokeMembership(ctx, mutation, createdMembership.ID); !errors.Is(err, authzapp.ErrAccessManagementConflict) {
		t.Fatalf("revoke inactive membership error = %v", err)
	}
	if err := repository.RevokeBinding(ctx, mutation, createdBinding.ID); err != nil {
		t.Fatalf("revoke binding: %v", err)
	}
	if err := repository.RevokeDeny(ctx, mutation, createdDeny.ID); err != nil {
		t.Fatalf("revoke deny: %v", err)
	}
	if err := repository.DisableRole(ctx, mutation, createdRole.ID); err != nil {
		t.Fatalf("disable Role: %v", err)
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
	if auditCount != 14 || outboxCount < 14 {
		t.Fatalf("audit/outbox counts = %d/%d", auditCount, outboxCount)
	}
	var reactivatedAuditCount, reactivatedOutboxCount int64
	if err := db.Table("audit_logs").Where("request_id = ? AND action = ? AND resource_id = ?", mutation.RequestID, "authorization.group_membership.reactivated", createdMembership.ID.String()).Count(&reactivatedAuditCount).Error; err != nil {
		t.Fatalf("count membership reactivation audit: %v", err)
	}
	if err := db.Table("outbox_events").Where("event_type = ? AND aggregate_id = ?", "authorization.group_membership.reactivated", createdMembership.ID.String()).Count(&reactivatedOutboxCount).Error; err != nil {
		t.Fatalf("count membership reactivation outbox: %v", err)
	}
	if reactivatedAuditCount != 1 || reactivatedOutboxCount != 1 {
		t.Fatalf("membership reactivation audit/outbox counts = %d/%d", reactivatedAuditCount, reactivatedOutboxCount)
	}

	secondPlatformUserID, secondPlatformGroupID := uuid.New(), uuid.New()
	secondPlatformMembershipID := uuid.New()
	continuitySeeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, username) VALUES (?, 'second-platform-manager')`, []any{secondPlatformUserID}},
		{`INSERT INTO authorization_groups (id, owner_kind, name) VALUES (?, 'platform', 'second-platform-managers')`, []any{secondPlatformGroupID}},
		{`INSERT INTO authorization_group_memberships (id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, []any{secondPlatformMembershipID, secondPlatformGroupID, secondPlatformUserID}},
	}
	for _, statement := range continuitySeeds {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed second platform path: %v", err)
		}
	}
	secondPlatformBinding, err := repository.CreateBinding(ctx, authzapp.AccessMutation{ActorID: platformUserID, RequestID: "create-second-platform-path"}, authzapp.CreateBindingInput{GroupID: secondPlatformGroupID, RoleID: uuid.MustParse("00000000-0000-0000-0000-000000000100"), ScopeKind: "platform"})
	if err != nil {
		t.Fatalf("create second platform binding: %v", err)
	}
	secondPlatformBindingID := secondPlatformBinding.ID

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, id := range []uuid.UUID{platformBindingID, secondPlatformBindingID} {
		wait.Add(1)
		go func(index int, id uuid.UUID) {
			defer wait.Done()
			results <- repository.RevokeBinding(ctx, authzapp.AccessMutation{ActorID: platformUserID, RequestID: fmt.Sprintf("concurrent-revoke-%d", index)}, id)
		}(index, id)
	}
	wait.Wait()
	close(results)
	var successful, protected int
	for result := range results {
		if result == nil {
			successful++
		} else if errors.Is(result, authzapp.ErrLastPlatformManager) {
			protected++
		} else {
			t.Fatalf("unexpected concurrent revoke error: %v", result)
		}
	}
	if successful != 1 || protected != 1 {
		t.Fatalf("concurrent revoke results success/protected = %d/%d", successful, protected)
	}
	var activePlatformBindings int64
	if err := db.Table("authorization_platform_role_bindings").Where("active").Count(&activePlatformBindings).Error; err != nil || activePlatformBindings != 1 {
		t.Fatalf("active platform bindings = %d, error = %v", activePlatformBindings, err)
	}
}

func assertCandidatePresence(t *testing.T, candidates []authzapp.AccessUser, userID uuid.UUID, expected bool) {
	t.Helper()
	found := false
	for _, candidate := range candidates {
		if candidate.ID == userID {
			found = true
			break
		}
	}
	if found != expected {
		t.Fatalf("candidate %s presence = %t, expected %t", userID, found, expected)
	}
}
