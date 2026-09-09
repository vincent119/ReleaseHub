package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestAccessManagementFiltersRecordsToManageableProjects(t *testing.T) {
	organizationID := uuid.New()
	allowedProjectID, hiddenProjectID := uuid.New(), uuid.New()
	allowedGroupID, hiddenGroupID := uuid.New(), uuid.New()
	allowedRoleID, hiddenRoleID := uuid.New(), uuid.New()
	allowedUserID, hiddenUserID := uuid.New(), uuid.New()
	repository := &accessRepositoryStub{
		scopes: []ProjectScope{{OrganizationID: organizationID, ProjectID: allowedProjectID}, {OrganizationID: organizationID, ProjectID: hiddenProjectID}},
		snapshot: AccessSnapshot{
			Users:       []AccessUser{{ID: allowedUserID, Username: "allowed"}, {ID: hiddenUserID, Username: "hidden"}},
			Groups:      []AccessGroup{{ID: allowedGroupID, OwnerKind: "organization", OwnerID: &organizationID, Name: "allowed"}, {ID: hiddenGroupID, OwnerKind: "project", OwnerID: &hiddenProjectID, Name: "hidden"}},
			Roles:       []AccessRole{{ID: allowedRoleID, OwnerKind: "platform", Name: "viewer"}, {ID: hiddenRoleID, OwnerKind: "project", OwnerID: &hiddenProjectID, Name: "hidden"}},
			Memberships: []AccessMembership{{ID: uuid.New(), GroupID: allowedGroupID, UserID: allowedUserID}, {ID: uuid.New(), GroupID: hiddenGroupID, UserID: hiddenUserID}},
			Bindings:    []AccessBinding{{ID: uuid.New(), GroupID: allowedGroupID, RoleID: allowedRoleID, OrganizationID: organizationID, ProjectID: allowedProjectID}, {ID: uuid.New(), GroupID: hiddenGroupID, RoleID: hiddenRoleID, OrganizationID: organizationID, ProjectID: hiddenProjectID}},
			Denies:      []AccessDeny{},
		},
	}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowedProjectID: allowedProjectID})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}

	value, err := service.Load(context.Background(), AccessPrincipal{UserID: uuid.New()})
	if err != nil {
		t.Fatalf("load access management: %v", err)
	}
	if len(value.Users) != 1 || value.Users[0].ID != allowedUserID || len(value.Groups) != 1 || len(value.Roles) != 1 || len(value.Bindings) != 1 {
		t.Fatalf("access snapshot leaked hidden Project records: %#v", value)
	}
}

func TestProjectManagerCannotDelegateNonDelegablePermission(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	repository := &accessRepositoryStub{
		scopes:   []ProjectScope{{OrganizationID: organizationID, ProjectID: projectID}},
		snapshot: AccessSnapshot{Permissions: []AccessPermission{{Key: "project.manage", ProjectRoleDelegable: false}}},
	}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowedProjectID: projectID})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}
	_, err = service.CreateRole(context.Background(), AccessPrincipal{UserID: uuid.New()}, AccessMutation{}, CreateRoleInput{OwnerKind: "project", OwnerID: &projectID, Name: "unsafe", Permissions: []string{"project.manage"}})
	if !errors.Is(err, ErrAccessManagementNotFound) {
		t.Fatalf("non-delegable Role error = %v", err)
	}
}

func TestAccessManagementRejectsSelfDisable(t *testing.T) {
	actorID := uuid.New()
	repository := &accessRepositoryStub{snapshot: AccessSnapshot{Users: []AccessUser{{ID: actorID, Username: "admin"}}}}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowPlatform: true})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}
	err = service.DisableUser(context.Background(), AccessPrincipal{UserID: actorID}, AccessMutation{}, actorID)
	if !errors.Is(err, ErrSelfDisable) {
		t.Fatalf("self-disable error = %v", err)
	}
	if repository.disableUserCalls != 0 {
		t.Fatalf("repository disable calls = %d", repository.disableUserCalls)
	}
}

func TestAccessActionsProtectBootstrapGroupAndSystemRole(t *testing.T) {
	repository := &accessRepositoryStub{snapshot: AccessSnapshot{
		Groups: []AccessGroup{
			{ID: uuid.New(), OwnerKind: "platform", Name: "platform_administrators", SystemKey: stringPointer("platform_administrators")},
			{ID: uuid.New(), OwnerKind: "platform", Name: "release_managers"},
		},
		Roles: []AccessRole{
			{ID: uuid.New(), OwnerKind: "platform", Name: "Platform administrator", SystemKey: stringPointer("platform_administrator"), Active: true},
			{ID: uuid.New(), OwnerKind: "platform", Name: "Release operator", Active: true},
		},
	}}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowPlatform: true})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}
	groups, err := service.ListGroups(context.Background(), AccessPrincipal{UserID: uuid.New()}, "", "", "", 20)
	if err != nil {
		t.Fatalf("list Groups: %v", err)
	}
	if containsAction(groups.Items[0].AllowedActions, "disable") || !containsAction(groups.Items[1].AllowedActions, "disable") {
		t.Fatalf("Group actions = %#v", groups.Items)
	}
	roles, err := service.ListRoles(context.Background(), AccessPrincipal{UserID: uuid.New()}, "", "", "", 20)
	if err != nil {
		t.Fatalf("list Roles: %v", err)
	}
	if containsAction(roles.Items[0].AllowedActions, "disable") || !containsAction(roles.Items[1].AllowedActions, "disable") {
		t.Fatalf("Role actions = %#v", roles.Items)
	}
}

func TestAccessCapabilitiesDoNotCrossDelegatedPermissionBoundaries(t *testing.T) {
	projectID := uuid.New()
	repository := &accessRepositoryStub{
		scopes: []ProjectScope{{OrganizationID: uuid.New(), ProjectID: projectID}},
		snapshot: AccessSnapshot{
			Groups: []AccessGroup{{ID: uuid.New(), OwnerKind: "project", OwnerID: &projectID, Name: "operators"}},
			Roles:  []AccessRole{{ID: uuid.New(), OwnerKind: "project", OwnerID: &projectID, Name: "operator", Active: true}},
		},
	}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowedProjectID: projectID, denyRoleManage: true})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}

	capabilities, err := service.Capabilities(context.Background(), AccessPrincipal{UserID: uuid.New()})
	if err != nil {
		t.Fatalf("load capabilities: %v", err)
	}
	if !collectionCanCreate(capabilities, "groups") || collectionCanCreate(capabilities, "roles") {
		t.Fatalf("collection capabilities crossed permission boundary: %#v", capabilities.Collections)
	}
	roles, err := service.ListRoles(context.Background(), AccessPrincipal{UserID: uuid.New()}, "", "", "", 20)
	if err != nil {
		t.Fatalf("list Roles: %v", err)
	}
	if len(roles.Items) != 1 || containsAction(roles.Items[0].AllowedActions, "disable") {
		t.Fatalf("Role actions crossed permission boundary: %#v", roles.Items)
	}
}

func TestAccessUserPaginationUsesStableRowCursor(t *testing.T) {
	first, second, third := uuid.New(), uuid.New(), uuid.New()
	repository := &accessRepositoryStub{snapshot: AccessSnapshot{Users: []AccessUser{
		{ID: first, Username: "alpha"},
		{ID: second, Username: "bravo"},
		{ID: third, Username: "charlie"},
	}}}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowPlatform: true})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}

	page, err := service.ListUsers(context.Background(), AccessPrincipal{UserID: uuid.New()}, "", "active", "", 2)
	if err != nil {
		t.Fatalf("list first user page: %v", err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.NextCursor != second.String() {
		t.Fatalf("first user page = %#v", page)
	}
	next, err := service.ListUsers(context.Background(), AccessPrincipal{UserID: uuid.New()}, "", "active", page.NextCursor, 2)
	if err != nil {
		t.Fatalf("list second user page: %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != third || next.HasMore || next.NextCursor != "" {
		t.Fatalf("second user page = %#v", next)
	}
}

func TestDelegatedMembershipCandidateLookupRequiresExactUsername(t *testing.T) {
	projectID, groupID := uuid.New(), uuid.New()
	repository := &accessRepositoryStub{
		scopes:   []ProjectScope{{OrganizationID: uuid.New(), ProjectID: projectID}},
		snapshot: AccessSnapshot{Groups: []AccessGroup{{ID: groupID, OwnerKind: "project", OwnerID: &projectID, Name: "operators"}}},
	}
	service, err := NewAccessManagementService(repository, &accessAuthorizerStub{allowedProjectID: projectID})
	if err != nil {
		t.Fatalf("create access management service: %v", err)
	}

	if _, err := service.MembershipCandidates(context.Background(), AccessPrincipal{UserID: uuid.New()}, groupID, "operator", 20); err != nil {
		t.Fatalf("find delegated membership candidate: %v", err)
	}
	if !repository.candidateExact || repository.candidateQuery != "operator" {
		t.Fatalf("delegated candidate lookup exact=%t query=%q", repository.candidateExact, repository.candidateQuery)
	}
}

func collectionCanCreate(value AccessCapabilities, key string) bool {
	for _, collection := range value.Collections {
		if collection.Key == key {
			return collection.CanCreate
		}
	}
	return false
}

func containsAction(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string { return &value }

type accessRepositoryStub struct {
	scopes           []ProjectScope
	snapshot         AccessSnapshot
	disableUserCalls int
	candidateExact   bool
	candidateQuery   string
}

func (s *accessRepositoryStub) ListProjectScopes(context.Context) ([]ProjectScope, error) {
	return s.scopes, nil
}

func (s *accessRepositoryStub) ResolveAccessScope(context.Context, string, uuid.UUID) (authz.Scope, error) {
	return authz.Scope{}, nil
}

func (s *accessRepositoryStub) FindMembershipCandidates(_ context.Context, _ uuid.UUID, query string, exact bool, _ int) ([]AccessUser, error) {
	s.candidateExact = exact
	s.candidateQuery = query
	return []AccessUser{}, nil
}

func (s *accessRepositoryStub) LoadAccessSnapshot(context.Context) (AccessSnapshot, error) {
	return s.snapshot, nil
}

func (s *accessRepositoryStub) CreateGroup(context.Context, AccessMutation, CreateGroupInput) (AccessGroup, error) {
	return AccessGroup{}, nil
}

func (s *accessRepositoryStub) DisableGroup(context.Context, AccessMutation, uuid.UUID) error {
	return nil
}

func (s *accessRepositoryStub) CreateRole(context.Context, AccessMutation, CreateRoleInput) (AccessRole, error) {
	return AccessRole{}, nil
}

func (s *accessRepositoryStub) AddMembership(context.Context, AccessMutation, uuid.UUID, uuid.UUID) (AccessMembership, error) {
	return AccessMembership{}, nil
}

func (s *accessRepositoryStub) CreateBinding(context.Context, AccessMutation, CreateBindingInput) (AccessBinding, error) {
	return AccessBinding{}, nil
}

func (s *accessRepositoryStub) CreateDeny(context.Context, AccessMutation, CreateDenyInput) (AccessDeny, error) {
	return AccessDeny{}, nil
}

func (s *accessRepositoryStub) DisableUser(context.Context, AccessMutation, uuid.UUID) error {
	s.disableUserCalls++
	return nil
}

func (s *accessRepositoryStub) RevokeMembership(context.Context, AccessMutation, uuid.UUID) error {
	return nil
}

func (s *accessRepositoryStub) DisableRole(context.Context, AccessMutation, uuid.UUID) error {
	return nil
}

func (s *accessRepositoryStub) RevokeBinding(context.Context, AccessMutation, uuid.UUID) error {
	return nil
}

func (s *accessRepositoryStub) RevokeDeny(context.Context, AccessMutation, uuid.UUID) error {
	return nil
}

type accessAuthorizerStub struct {
	allowedProjectID uuid.UUID
	allowPlatform    bool
	denyGroupManage  bool
	denyRoleManage   bool
}

func (s *accessAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	if request.Scope.Kind == authz.ScopePlatform {
		return s.allowPlatform, nil
	}
	if request.Permission == authz.Permission("group.manage") && s.denyGroupManage {
		return false, nil
	}
	if request.Permission == authz.Permission("role.manage") && s.denyRoleManage {
		return false, nil
	}
	return request.Scope.Kind == authz.ScopeProject && request.Scope.ProjectID == s.allowedProjectID, nil
}
