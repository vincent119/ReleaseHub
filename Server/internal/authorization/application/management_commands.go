package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// AccessMutation identifies the administrator and request attached to one audited policy change.
type AccessMutation struct {
	ActorID   uuid.UUID
	RequestID string
}

// CreateGroupInput defines a Group owner and immutable initial attributes.
type CreateGroupInput struct {
	OwnerKind      string
	OwnerID        *uuid.UUID
	Name           string
	OIDCViewerOnly bool
}

// CreateRoleInput defines a Role and its initial permission set.
type CreateRoleInput struct {
	OwnerKind   string
	OwnerID     *uuid.UUID
	Name        string
	Permissions []string
}

// CreateBindingInput grants a Role to a Group at one exact scope.
type CreateBindingInput struct {
	GroupID        uuid.UUID
	RoleID         uuid.UUID
	OrganizationID uuid.UUID
	ScopeKind      string
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
}

// CreateDenyInput explicitly denies one permission at one exact scope.
type CreateDenyInput struct {
	GroupID        uuid.UUID
	Permission     string
	OrganizationID uuid.UUID
	ScopeKind      string
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
}

// CreateGroup creates a platform, Organization, or Project-owned Group within the caller's management boundary.
func (s *AccessManagementService) CreateGroup(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, input CreateGroupInput) (AccessGroup, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 || !validOwner(input.OwnerKind, input.OwnerID, true) {
		return AccessGroup{}, fmt.Errorf("%w: invalid Group input", ErrInvalidAccessRequest)
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.groupManage, input.OwnerKind, input.OwnerID)
	if err != nil || !allowed {
		return AccessGroup{}, accessDecision(err)
	}
	mutation.ActorID = principal.UserID
	return s.repository.CreateGroup(ctx, mutation, input)
}

// DisableGroup revokes one Group and relies on the database trigger to deactivate its policies.
func (s *AccessManagementService) DisableGroup(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, groupID uuid.UUID) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	group, ok := findGroup(value.Groups, groupID)
	if !ok {
		return ErrAccessManagementNotFound
	}
	if group.SystemKey != nil {
		return ErrProtectedAccessResource
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.groupManage, group.OwnerKind, group.OwnerID)
	if err != nil || !allowed {
		return accessDecision(err)
	}
	mutation.ActorID = principal.UserID
	return s.repository.DisableGroup(ctx, mutation, groupID)
}

// CreateRole creates a platform Role or a constrained Project-owned custom Role.
func (s *AccessManagementService) CreateRole(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, input CreateRoleInput) (AccessRole, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 || !validOwner(input.OwnerKind, input.OwnerID, false) || len(input.Permissions) == 0 {
		return AccessRole{}, fmt.Errorf("%w: invalid Role input", ErrInvalidAccessRequest)
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.roleManage, input.OwnerKind, input.OwnerID)
	if err != nil || !allowed {
		return AccessRole{}, accessDecision(err)
	}
	if input.OwnerKind == "project" {
		if err := s.validateDelegatedPermissions(ctx, principal, *input.OwnerID, input.Permissions); err != nil {
			return AccessRole{}, err
		}
	}
	mutation.ActorID = principal.UserID
	return s.repository.CreateRole(ctx, mutation, input)
}

// AddMembership manually adds a user to a manageable Group.
func (s *AccessManagementService) AddMembership(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, groupID, userID uuid.UUID) (AccessMembership, error) {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return AccessMembership{}, err
	}
	group, ok := findGroup(value.Groups, groupID)
	if !ok {
		return AccessMembership{}, ErrAccessManagementNotFound
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.groupManage, group.OwnerKind, group.OwnerID)
	if err != nil || !allowed {
		return AccessMembership{}, accessDecision(err)
	}
	mutation.ActorID = principal.UserID
	return s.repository.AddMembership(ctx, mutation, groupID, userID)
}

// CreateBinding assigns a Group and Role to a validated Project descendant scope.
func (s *AccessManagementService) CreateBinding(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, input CreateBindingInput) (AccessBinding, error) {
	if input.ScopeKind == "platform" {
		platform, err := s.platformAllowed(ctx, principal)
		if err != nil || !platform {
			return AccessBinding{}, accessDecision(err)
		}
		value, err := s.repository.LoadAccessSnapshot(ctx)
		if err != nil {
			return AccessBinding{}, err
		}
		group, groupOK := findGroup(value.Groups, input.GroupID)
		role, roleOK := findRole(value.Roles, input.RoleID)
		if !groupOK || !roleOK || group.DisabledAt != nil || !role.Active || group.OwnerKind != "platform" || role.OwnerKind != "platform" {
			return AccessBinding{}, ErrAccessManagementNotFound
		}
		mutation.ActorID = principal.UserID
		return s.repository.CreateBinding(ctx, mutation, input)
	}
	scope, err := commandScope(input.OrganizationID, input.ProjectID, input.ScopeKind, input.EnvironmentID, input.ApplicationID)
	if err != nil {
		return AccessBinding{}, fmt.Errorf("%w: %v", ErrInvalidAccessRequest, err)
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.groupManage, Scope: scope})
	platform, platformErr := s.platformAllowed(ctx, principal)
	if err != nil || platformErr != nil || (!allowed && !platform) {
		return AccessBinding{}, accessDecision(errorsJoin(err, platformErr))
	}
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return AccessBinding{}, err
	}
	group, groupOK := findGroup(value.Groups, input.GroupID)
	role, roleOK := findRole(value.Roles, input.RoleID)
	if !groupOK || !roleOK || group.DisabledAt != nil || !role.Active || (!platform && role.SystemKey != nil && *role.SystemKey == "project_manager") || !groupMayBind(group, input.OrganizationID, input.ProjectID, platform) {
		return AccessBinding{}, ErrAccessManagementNotFound
	}
	mutation.ActorID = principal.UserID
	return s.repository.CreateBinding(ctx, mutation, input)
}

// CreateDeny creates an explicit deny policy at a validated Project descendant scope.
func (s *AccessManagementService) CreateDeny(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, input CreateDenyInput) (AccessDeny, error) {
	scope, err := commandScope(input.OrganizationID, input.ProjectID, input.ScopeKind, input.EnvironmentID, input.ApplicationID)
	if err != nil {
		return AccessDeny{}, fmt.Errorf("%w: %v", ErrInvalidAccessRequest, err)
	}
	permission, err := authz.NewPermission(input.Permission)
	if err != nil {
		return AccessDeny{}, fmt.Errorf("%w: invalid permission", ErrInvalidAccessRequest)
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.groupManage, Scope: scope})
	platform, platformErr := s.platformAllowed(ctx, principal)
	if err != nil || platformErr != nil || (!allowed && !platform) {
		return AccessDeny{}, accessDecision(errorsJoin(err, platformErr))
	}
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return AccessDeny{}, err
	}
	group, ok := findGroup(value.Groups, input.GroupID)
	if !ok || group.DisabledAt != nil || !groupMayBind(group, input.OrganizationID, input.ProjectID, platform) {
		return AccessDeny{}, ErrAccessManagementNotFound
	}
	input.Permission = string(permission)
	mutation.ActorID = principal.UserID
	return s.repository.CreateDeny(ctx, mutation, input)
}

// DisableUser locally disables an account and revokes all active sessions after platform authorization.
func (s *AccessManagementService) DisableUser(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, userID uuid.UUID) error {
	allowed, err := s.platformAllowed(ctx, principal)
	if err != nil || !allowed {
		return accessDecision(err)
	}
	if principal.UserID == userID {
		return ErrSelfDisable
	}
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	var found bool
	for _, user := range value.Users {
		if user.ID == userID && user.DisabledAt == nil {
			found = true
			break
		}
	}
	if !found {
		return ErrAccessManagementNotFound
	}
	mutation.ActorID = principal.UserID
	return s.repository.DisableUser(ctx, mutation, userID)
}

// RevokeMembership deactivates one manual Group membership.
func (s *AccessManagementService) RevokeMembership(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, membershipID uuid.UUID) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	var membership AccessMembership
	for _, candidate := range value.Memberships {
		if candidate.ID == membershipID {
			membership = candidate
			break
		}
	}
	if membership.ID == uuid.Nil {
		return ErrAccessManagementNotFound
	}
	if !membership.Active {
		return ErrAccessManagementConflict
	}
	if membership.Source != "manual" {
		return ErrProtectedAccessResource
	}
	group, ok := findGroup(value.Groups, membership.GroupID)
	if !ok {
		return ErrAccessManagementNotFound
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.groupManage, group.OwnerKind, group.OwnerID)
	if err != nil || !allowed {
		return accessDecision(err)
	}
	mutation.ActorID = principal.UserID
	return s.repository.RevokeMembership(ctx, mutation, membershipID)
}

// DisableRole deactivates one custom Role while preserving its audit history.
func (s *AccessManagementService) DisableRole(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, roleID uuid.UUID) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	role, ok := findRole(value.Roles, roleID)
	if !ok {
		return ErrAccessManagementNotFound
	}
	if !role.Active {
		return ErrAccessManagementConflict
	}
	if role.SystemKey != nil {
		return ErrProtectedAccessResource
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.roleManage, role.OwnerKind, role.OwnerID)
	if err != nil || !allowed {
		return accessDecision(err)
	}
	mutation.ActorID = principal.UserID
	return s.repository.DisableRole(ctx, mutation, roleID)
}

// RevokeBinding deactivates one Project-descendant Role binding.
func (s *AccessManagementService) RevokeBinding(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, bindingID uuid.UUID) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	var binding AccessBinding
	for _, candidate := range value.Bindings {
		if candidate.ID == bindingID {
			binding = candidate
			break
		}
	}
	if binding.ID == uuid.Nil {
		return ErrAccessManagementNotFound
	}
	if !binding.Active {
		return ErrAccessManagementConflict
	}
	if binding.ScopeKind == "platform" {
		platform, err := s.platformAllowed(ctx, principal)
		if err != nil || !platform {
			return accessDecision(err)
		}
		mutation.ActorID = principal.UserID
		return s.repository.RevokeBinding(ctx, mutation, bindingID)
	}
	scope, err := commandScope(binding.OrganizationID, binding.ProjectID, binding.ScopeKind, binding.EnvironmentID, binding.ApplicationID)
	if err != nil {
		return ErrAccessManagementNotFound
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.groupManage, Scope: scope})
	platform, platformErr := s.platformAllowed(ctx, principal)
	if err != nil || platformErr != nil || (!allowed && !platform) {
		return accessDecision(errorsJoin(err, platformErr))
	}
	mutation.ActorID = principal.UserID
	return s.repository.RevokeBinding(ctx, mutation, bindingID)
}

// RevokeDeny deactivates one explicit deny policy.
func (s *AccessManagementService) RevokeDeny(ctx context.Context, principal AccessPrincipal, mutation AccessMutation, denyID uuid.UUID) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	var deny AccessDeny
	for _, candidate := range value.Denies {
		if candidate.ID == denyID {
			deny = candidate
			break
		}
	}
	if deny.ID == uuid.Nil {
		return ErrAccessManagementNotFound
	}
	if !deny.Active {
		return ErrAccessManagementConflict
	}
	scope, err := commandScope(deny.OrganizationID, deny.ProjectID, deny.ScopeKind, deny.EnvironmentID, deny.ApplicationID)
	if err != nil {
		return ErrAccessManagementNotFound
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.groupManage, Scope: scope})
	platform, platformErr := s.platformAllowed(ctx, principal)
	if err != nil || platformErr != nil || (!allowed && !platform) {
		return accessDecision(errorsJoin(err, platformErr))
	}
	mutation.ActorID = principal.UserID
	return s.repository.RevokeDeny(ctx, mutation, denyID)
}

func (s *AccessManagementService) platformAllowed(ctx context.Context, principal AccessPrincipal) (bool, error) {
	return s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
}

func (s *AccessManagementService) mayManageOwner(ctx context.Context, principal AccessPrincipal, permission authz.Permission, ownerKind string, ownerID *uuid.UUID) (bool, error) {
	platform, err := s.platformAllowed(ctx, principal)
	if err != nil || platform || ownerKind != "project" || ownerID == nil {
		return platform, err
	}
	scopes, err := s.repository.ListProjectScopes(ctx)
	if err != nil {
		return false, err
	}
	for _, candidate := range scopes {
		if candidate.ProjectID != *ownerID {
			continue
		}
		scope, _ := authz.NewProjectScope(candidate.OrganizationID, candidate.ProjectID)
		return s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope})
	}
	return false, nil
}

func (s *AccessManagementService) validateDelegatedPermissions(ctx context.Context, principal AccessPrincipal, projectID uuid.UUID, permissions []string) error {
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return err
	}
	delegable := make(map[string]bool)
	for _, permission := range value.Permissions {
		delegable[permission.Key] = permission.ProjectRoleDelegable
	}
	projects, err := s.repository.ListProjectScopes(ctx)
	if err != nil {
		return err
	}
	var project ProjectScope
	for _, candidate := range projects {
		if candidate.ProjectID == projectID {
			project = candidate
		}
	}
	if project.ProjectID == uuid.Nil {
		return ErrAccessManagementNotFound
	}
	scope, _ := authz.NewProjectScope(project.OrganizationID, project.ProjectID)
	for _, key := range permissions {
		permission, permissionErr := authz.NewPermission(key)
		if permissionErr != nil || !delegable[key] {
			return ErrAccessManagementNotFound
		}
		allowed, authErr := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope})
		if authErr != nil || !allowed {
			return accessDecision(authErr)
		}
	}
	return nil
}

func validOwner(kind string, id *uuid.UUID, groups bool) bool {
	if kind == "platform" {
		return id == nil
	}
	if kind == "project" {
		return id != nil && *id != uuid.Nil
	}
	return groups && kind == "organization" && id != nil && *id != uuid.Nil
}

func commandScope(organizationID, projectID uuid.UUID, kind string, environmentID, applicationID *uuid.UUID) (authz.Scope, error) {
	switch kind {
	case "project":
		return authz.NewProjectScope(organizationID, projectID)
	case "environment":
		if environmentID == nil {
			return authz.Scope{}, fmt.Errorf("%w: environment ID is required", ErrInvalidAccessRequest)
		}
		return authz.NewEnvironmentScope(organizationID, projectID, *environmentID)
	case "application":
		if environmentID == nil || applicationID == nil {
			return authz.Scope{}, fmt.Errorf("%w: environment and Application IDs are required", ErrInvalidAccessRequest)
		}
		return authz.NewApplicationScope(organizationID, projectID, *environmentID, *applicationID)
	default:
		return authz.Scope{}, fmt.Errorf("%w: unsupported scope kind %q", ErrInvalidAccessRequest, kind)
	}
}

func findGroup(values []AccessGroup, id uuid.UUID) (AccessGroup, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return AccessGroup{}, false
}

func findRole(values []AccessRole, id uuid.UUID) (AccessRole, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return AccessRole{}, false
}

func groupMayBind(group AccessGroup, organizationID, projectID uuid.UUID, platform bool) bool {
	if platform {
		return true
	}
	return group.OwnerID != nil && ((group.OwnerKind == "project" && *group.OwnerID == projectID) || (group.OwnerKind == "organization" && *group.OwnerID == organizationID))
}

func accessDecision(err error) error {
	if err != nil {
		return err
	}
	return ErrAccessManagementNotFound
}

func errorsJoin(first, second error) error {
	if first != nil {
		return first
	}
	return second
}
