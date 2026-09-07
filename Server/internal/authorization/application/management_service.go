package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// ErrAccessManagementNotFound hides access-management data from unauthorized users.
var ErrAccessManagementNotFound = errors.New("access management not found")

// AccessPrincipal is the authenticated local user requesting access-management data.
type AccessPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// ProjectScope identifies one active Project boundary available for policy evaluation.
type ProjectScope struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
}

// AccessUser is a user referenced by an included Group.
type AccessUser struct {
	ID         uuid.UUID
	Username   string
	DisabledAt *time.Time
}

// AccessGroup is a platform, Organization, or Project-owned Group.
type AccessGroup struct {
	ID             uuid.UUID
	OwnerKind      string
	OwnerID        *uuid.UUID
	Name           string
	OIDCViewerOnly bool
	DisabledAt     *time.Time
}

// AccessRole is a platform or Project-owned Role and its permissions.
type AccessRole struct {
	ID          uuid.UUID
	OwnerKind   string
	OwnerID     *uuid.UUID
	Name        string
	SystemKey   *string
	Active      bool
	Permissions []string
}

// AccessPermission describes whether a permission may be composed into a Project-owned Role.
type AccessPermission struct {
	Key                  string
	PlatformOnly         bool
	ProjectRoleDelegable bool
}

// AccessMembership associates one user with one Group.
type AccessMembership struct {
	ID      uuid.UUID
	GroupID uuid.UUID
	UserID  uuid.UUID
	Source  string
	Active  bool
}

// AccessBinding grants one Role to one Group at a resource scope.
type AccessBinding struct {
	ID             uuid.UUID
	GroupID        uuid.UUID
	RoleID         uuid.UUID
	OrganizationID uuid.UUID
	ScopeKind      string
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Active         bool
}

// AccessDeny explicitly denies one permission for a Group at a resource scope.
type AccessDeny struct {
	ID             uuid.UUID
	GroupID        uuid.UUID
	Permission     string
	OrganizationID uuid.UUID
	ScopeKind      string
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Active         bool
}

// AccessSnapshot is the filtered read model consumed by the administration UI.
type AccessSnapshot struct {
	CanManagePlatform bool
	ManagedProjects   []uuid.UUID
	Users             []AccessUser
	Groups            []AccessGroup
	Roles             []AccessRole
	Permissions       []AccessPermission
	Memberships       []AccessMembership
	Bindings          []AccessBinding
	Denies            []AccessDeny
}

// AccessManagementRepository loads policy records before application-layer filtering.
type AccessManagementRepository interface {
	ListProjectScopes(context.Context) ([]ProjectScope, error)
	LoadAccessSnapshot(context.Context) (AccessSnapshot, error)
	CreateGroup(context.Context, AccessMutation, CreateGroupInput) (AccessGroup, error)
	DisableGroup(context.Context, AccessMutation, uuid.UUID) error
	CreateRole(context.Context, AccessMutation, CreateRoleInput) (AccessRole, error)
	AddMembership(context.Context, AccessMutation, uuid.UUID, uuid.UUID) (AccessMembership, error)
	CreateBinding(context.Context, AccessMutation, CreateBindingInput) (AccessBinding, error)
	CreateDeny(context.Context, AccessMutation, CreateDenyInput) (AccessDeny, error)
	DisableUser(context.Context, AccessMutation, uuid.UUID) error
}

// AccessManagementAuthorizer revalidates sensitive access-management reads.
type AccessManagementAuthorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// AccessManagementService filters the access read model by manageable Project scopes.
type AccessManagementService struct {
	repository     AccessManagementRepository
	authorizer     AccessManagementAuthorizer
	platformManage authz.Permission
	groupManage    authz.Permission
	roleManage     authz.Permission
}

// NewAccessManagementService creates the access-management query service.
func NewAccessManagementService(repository AccessManagementRepository, authorizer AccessManagementAuthorizer) (*AccessManagementService, error) {
	if repository == nil || authorizer == nil {
		return nil, errors.New("access management dependencies are required")
	}
	platformManage, _ := authz.NewPermission("platform.manage")
	groupManage, _ := authz.NewPermission("group.manage")
	roleManage, _ := authz.NewPermission("role.manage")
	return &AccessManagementService{repository: repository, authorizer: authorizer, platformManage: platformManage, groupManage: groupManage, roleManage: roleManage}, nil
}

// Load returns all policy records for platform administrators or a Project-filtered snapshot for delegated managers.
func (s *AccessManagementService) Load(ctx context.Context, principal AccessPrincipal) (AccessSnapshot, error) {
	platformAllowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return AccessSnapshot{}, fmt.Errorf("authorize platform access management: %w", err)
	}
	if platformAllowed {
		value, loadErr := s.repository.LoadAccessSnapshot(ctx)
		value.CanManagePlatform = true
		return value, loadErr
	}
	if principal.Disabled {
		return AccessSnapshot{}, ErrAccessManagementNotFound
	}
	scopes, err := s.repository.ListProjectScopes(ctx)
	if err != nil {
		return AccessSnapshot{}, fmt.Errorf("list access management Project scopes: %w", err)
	}
	allowedProjects := make(map[uuid.UUID]struct{})
	for _, value := range scopes {
		scope, scopeErr := authz.NewProjectScope(value.OrganizationID, value.ProjectID)
		if scopeErr != nil {
			return AccessSnapshot{}, scopeErr
		}
		groupAllowed, authErr := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Permission: s.groupManage, Scope: scope})
		if authErr != nil {
			return AccessSnapshot{}, fmt.Errorf("authorize Group management: %w", authErr)
		}
		roleAllowed, authErr := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Permission: s.roleManage, Scope: scope})
		if authErr != nil {
			return AccessSnapshot{}, fmt.Errorf("authorize Role management: %w", authErr)
		}
		if groupAllowed || roleAllowed {
			allowedProjects[value.ProjectID] = struct{}{}
		}
	}
	if len(allowedProjects) == 0 {
		return AccessSnapshot{}, ErrAccessManagementNotFound
	}
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return AccessSnapshot{}, err
	}
	value = filterAccessSnapshot(value, allowedProjects)
	for projectID := range allowedProjects {
		value.ManagedProjects = append(value.ManagedProjects, projectID)
	}
	return value, nil
}

func filterAccessSnapshot(value AccessSnapshot, allowedProjects map[uuid.UUID]struct{}) AccessSnapshot {
	result := AccessSnapshot{Users: []AccessUser{}, Groups: []AccessGroup{}, Roles: []AccessRole{}, Permissions: []AccessPermission{}, Memberships: []AccessMembership{}, Bindings: []AccessBinding{}, Denies: []AccessDeny{}}
	includedGroups := make(map[uuid.UUID]struct{})
	includedRoles := make(map[uuid.UUID]struct{})
	for _, binding := range value.Bindings {
		if _, ok := allowedProjects[binding.ProjectID]; !ok {
			continue
		}
		result.Bindings = append(result.Bindings, binding)
		includedGroups[binding.GroupID] = struct{}{}
		includedRoles[binding.RoleID] = struct{}{}
	}
	for _, deny := range value.Denies {
		if _, ok := allowedProjects[deny.ProjectID]; !ok {
			continue
		}
		result.Denies = append(result.Denies, deny)
		includedGroups[deny.GroupID] = struct{}{}
	}
	for _, group := range value.Groups {
		if group.OwnerKind == "project" && group.OwnerID != nil {
			if _, ok := allowedProjects[*group.OwnerID]; ok {
				includedGroups[group.ID] = struct{}{}
			}
		}
		if _, ok := includedGroups[group.ID]; ok {
			result.Groups = append(result.Groups, group)
		}
	}
	includedUsers := make(map[uuid.UUID]struct{})
	for _, membership := range value.Memberships {
		if _, ok := includedGroups[membership.GroupID]; ok {
			result.Memberships = append(result.Memberships, membership)
			includedUsers[membership.UserID] = struct{}{}
		}
	}
	for _, user := range value.Users {
		if _, ok := includedUsers[user.ID]; ok {
			result.Users = append(result.Users, user)
		}
	}
	for _, role := range value.Roles {
		if role.OwnerKind == "project" && role.OwnerID != nil {
			if _, ok := allowedProjects[*role.OwnerID]; ok {
				includedRoles[role.ID] = struct{}{}
			}
		}
		if _, ok := includedRoles[role.ID]; ok {
			result.Roles = append(result.Roles, role)
		}
	}
	for _, permission := range value.Permissions {
		if permission.ProjectRoleDelegable {
			result.Permissions = append(result.Permissions, permission)
		}
	}
	return result
}
