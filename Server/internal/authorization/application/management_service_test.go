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

type accessRepositoryStub struct {
	scopes   []ProjectScope
	snapshot AccessSnapshot
}

func (s *accessRepositoryStub) ListProjectScopes(context.Context) ([]ProjectScope, error) {
	return s.scopes, nil
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
	return nil
}

type accessAuthorizerStub struct{ allowedProjectID uuid.UUID }

func (s *accessAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	return request.Scope.Kind == authz.ScopeProject && request.Scope.ProjectID == s.allowedProjectID, nil
}
