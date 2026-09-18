package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// Capabilities returns collection-level actions without exposing permission composition to the Web client.
func (s *AccessManagementService) Capabilities(ctx context.Context, principal AccessPrincipal) (AccessCapabilities, error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessCapabilities{}, err
	}
	canCreateGroupPolicy := value.CanManagePlatform || len(value.managedGroups) > 0
	canCreateRole := value.CanManagePlatform || len(value.managedRoles) > 0
	return AccessCapabilities{Collections: []AccessCollectionCapability{
		{Key: "users", Visible: true, CanCreate: value.CanManagePlatform},
		{Key: "groups", Visible: true, CanCreate: canCreateGroupPolicy},
		{Key: "roles", Visible: true, CanCreate: canCreateRole},
		{Key: "memberships", Visible: true, CanCreate: false},
		{Key: "bindings", Visible: true, CanCreate: canCreateGroupPolicy},
		{Key: "denies", Visible: true, CanCreate: canCreateGroupPolicy},
	}, Permissions: value.Permissions}, nil
}

// MembershipCandidates returns one stable page of minimal user identities for one manageable Group.
func (s *AccessManagementService) MembershipCandidates(ctx context.Context, principal AccessPrincipal, groupID uuid.UUID, query, cursor string, limit int) (AccessListPage[AccessUser], error) {
	query = strings.TrimSpace(query)
	if len(query) > 128 {
		return AccessListPage[AccessUser]{}, ErrInvalidAccessRequest
	}
	after, err := decodeMembershipCandidateCursor(cursor, query)
	if err != nil {
		return AccessListPage[AccessUser]{}, err
	}
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessUser]{}, err
	}
	group, ok := findGroup(value.Groups, groupID)
	if !ok || group.DisabledAt != nil {
		return AccessListPage[AccessUser]{}, ErrAccessManagementNotFound
	}
	allowed, err := s.mayManageOwner(ctx, principal, s.groupManage, group.OwnerKind, group.OwnerID)
	if err != nil || !allowed {
		return AccessListPage[AccessUser]{}, accessDecision(err)
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	items, err := s.repository.FindMembershipCandidates(ctx, groupID, query, after, limit+1)
	if err != nil {
		return AccessListPage[AccessUser]{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		next, err = encodeMembershipCandidateCursor(query, items[len(items)-1])
		if err != nil {
			return AccessListPage[AccessUser]{}, err
		}
	}
	return AccessListPage[AccessUser]{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

type membershipCandidateCursorPayload struct {
	Query    string    `json:"query"`
	Username string    `json:"username"`
	UserID   uuid.UUID `json:"userId"`
}

func encodeMembershipCandidateCursor(query string, user AccessUser) (string, error) {
	value, err := json.Marshal(membershipCandidateCursorPayload{Query: strings.ToLower(query), Username: strings.ToLower(user.Username), UserID: user.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func decodeMembershipCandidateCursor(cursor, query string) (*MembershipCandidateCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	value, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrInvalidAccessRequest
	}
	var payload membershipCandidateCursorPayload
	if err := json.Unmarshal(value, &payload); err != nil || payload.UserID == uuid.Nil || payload.Username == "" || payload.Query != strings.ToLower(query) {
		return nil, ErrInvalidAccessRequest
	}
	return &MembershipCandidateCursor{Username: payload.Username, UserID: payload.UserID}, nil
}

// ScopeOptions returns legal Group, Role, and Permission choices after a scope is selected.
func (s *AccessManagementService) ScopeOptions(ctx context.Context, principal AccessPrincipal, scopeKind, scopeID string) (AccessScopeOptions, error) {
	platform, err := s.platformAllowed(ctx, principal)
	if err != nil {
		return AccessScopeOptions{}, err
	}
	var scope authz.Scope
	if scopeKind == "platform" {
		if !platform || scopeID != "platform" {
			return AccessScopeOptions{}, ErrAccessManagementNotFound
		}
		scope = authz.NewPlatformScope()
	} else {
		id, parseErr := uuid.Parse(scopeID)
		if parseErr != nil {
			return AccessScopeOptions{}, ErrAccessManagementNotFound
		}
		scope, err = s.repository.ResolveAccessScope(ctx, scopeKind, id)
		if err != nil {
			return AccessScopeOptions{}, ErrAccessManagementNotFound
		}
		allowed, authErr := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.groupManage, Scope: scope})
		if authErr != nil || (!allowed && !platform) {
			return AccessScopeOptions{}, accessDecision(authErr)
		}
	}
	value, err := s.repository.LoadAccessSnapshot(ctx)
	if err != nil {
		return AccessScopeOptions{}, err
	}
	result := AccessScopeOptions{Groups: []AccessGroup{}, Roles: []AccessRole{}, Permissions: []AccessPermission{}}
	for _, group := range value.Groups {
		if group.DisabledAt != nil {
			continue
		}
		if scopeKind == "platform" {
			if group.OwnerKind == "platform" {
				result.Groups = append(result.Groups, group)
			}
			continue
		}
		if groupMayBind(group, scope.OrganizationID, scope.ProjectID, platform) {
			result.Groups = append(result.Groups, group)
		}
	}
	for _, role := range value.Roles {
		if !role.Active || !roleHasEffectivePermission(role, value.Permissions, scopeKind) {
			continue
		}
		if scopeKind == "platform" {
			if role.OwnerKind == "platform" {
				result.Roles = append(result.Roles, role)
			}
			continue
		}
		if role.OwnerKind == "project" && role.OwnerID != nil && *role.OwnerID != scope.ProjectID {
			continue
		}
		if !platform && role.SystemKey != nil && *role.SystemKey == "project_manager" {
			continue
		}
		result.Roles = append(result.Roles, role)
	}
	for _, permission := range value.Permissions {
		if scopeKind == "platform" || permission.ProjectRoleDelegable {
			result.Permissions = append(result.Permissions, permission)
		}
	}
	return result, nil
}

// ListUsers returns one filtered user page.
func (s *AccessManagementService) ListUsers(ctx context.Context, principal AccessPrincipal, search, status, cursor string, limit int) (AccessListPage[AccessUser], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessUser]{}, err
	}
	items := make([]AccessUser, 0, len(value.Users))
	for _, item := range value.Users {
		if !matchesSearch(item.Username, search) || !matchesStatus(item.DisabledAt == nil, status) {
			continue
		}
		if value.CanManagePlatform && item.DisabledAt == nil && item.ID != principal.UserID {
			item.AllowedActions = []string{"disable"}
		} else {
			item.AllowedActions = []string{}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessUser) uuid.UUID { return item.ID })
}

// ListGroups returns one filtered Group page.
func (s *AccessManagementService) ListGroups(ctx context.Context, principal AccessPrincipal, search, status, cursor string, limit int) (AccessListPage[AccessGroup], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessGroup]{}, err
	}
	items := make([]AccessGroup, 0, len(value.Groups))
	for _, item := range value.Groups {
		if !matchesSearch(item.Name, search) || !matchesStatus(item.DisabledAt == nil, status) {
			continue
		}
		item.AllowedActions = []string{"viewMemberships"}
		if item.DisabledAt == nil && value.canManageGroup(item) {
			item.AllowedActions = append(item.AllowedActions, "addMember")
			if item.SystemKey == nil {
				item.AllowedActions = append(item.AllowedActions, "disable")
			}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessGroup) uuid.UUID { return item.ID })
}

// ListRoles returns one filtered Role page.
func (s *AccessManagementService) ListRoles(ctx context.Context, principal AccessPrincipal, search, status, cursor string, limit int) (AccessListPage[AccessRole], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessRole]{}, err
	}
	items := make([]AccessRole, 0, len(value.Roles))
	for _, item := range value.Roles {
		if !matchesSearch(item.Name, search) || !matchesStatus(item.Active, status) {
			continue
		}
		item.AllowedActions = []string{}
		if item.Active && item.SystemKey == nil && value.canManageRole(item) {
			item.AllowedActions = []string{"disable"}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessRole) uuid.UUID { return item.ID })
}

// ListMemberships returns one filtered membership page.
func (s *AccessManagementService) ListMemberships(ctx context.Context, principal AccessPrincipal, groupID, userID *uuid.UUID, status, cursor string, limit int) (AccessListPage[AccessMembership], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessMembership]{}, err
	}
	items := make([]AccessMembership, 0, len(value.Memberships))
	groupNames := make(map[uuid.UUID]string, len(value.Groups))
	userNames := make(map[uuid.UUID]string, len(value.Users))
	for _, group := range value.Groups {
		groupNames[group.ID] = group.Name
	}
	for _, user := range value.Users {
		userNames[user.ID] = user.Username
	}
	for _, item := range value.Memberships {
		if groupID != nil && item.GroupID != *groupID || userID != nil && item.UserID != *userID || !matchesStatus(item.Active, status) {
			continue
		}
		item.AllowedActions = []string{}
		item.GroupName, item.Username = groupNames[item.GroupID], userNames[item.UserID]
		group := findAccessGroup(value.Groups, item.GroupID)
		if item.Active && item.Source == "manual" && group != nil && value.canManageGroup(*group) {
			item.AllowedActions = []string{"revoke"}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessMembership) uuid.UUID { return item.ID })
}

// ListBindings returns one filtered Project-descendant binding page.
func (s *AccessManagementService) ListBindings(ctx context.Context, principal AccessPrincipal, status, cursor string, limit int) (AccessListPage[AccessBinding], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessBinding]{}, err
	}
	items := make([]AccessBinding, 0, len(value.Bindings))
	groupNames := make(map[uuid.UUID]string, len(value.Groups))
	roleNames := make(map[uuid.UUID]string, len(value.Roles))
	for _, group := range value.Groups {
		groupNames[group.ID] = group.Name
	}
	for _, role := range value.Roles {
		roleNames[role.ID] = role.Name
	}
	for _, item := range value.Bindings {
		if !matchesStatus(item.Active, status) {
			continue
		}
		item.AllowedActions = []string{}
		item.GroupName, item.RoleName = groupNames[item.GroupID], roleNames[item.RoleID]
		if item.Active && value.canManagePolicyAt(item.ProjectID) {
			item.AllowedActions = []string{"revoke"}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessBinding) uuid.UUID { return item.ID })
}

// ListDenies returns one filtered explicit-deny page.
func (s *AccessManagementService) ListDenies(ctx context.Context, principal AccessPrincipal, status, cursor string, limit int) (AccessListPage[AccessDeny], error) {
	value, err := s.Load(ctx, principal)
	if err != nil {
		return AccessListPage[AccessDeny]{}, err
	}
	items := make([]AccessDeny, 0, len(value.Denies))
	groupNames := make(map[uuid.UUID]string, len(value.Groups))
	for _, group := range value.Groups {
		groupNames[group.ID] = group.Name
	}
	for _, item := range value.Denies {
		if !matchesStatus(item.Active, status) {
			continue
		}
		item.AllowedActions = []string{}
		item.GroupName = groupNames[item.GroupID]
		if item.Active && value.canManagePolicyAt(item.ProjectID) {
			item.AllowedActions = []string{"revoke"}
		}
		items = append(items, item)
	}
	return pageByID(items, cursor, limit, func(item AccessDeny) uuid.UUID { return item.ID })
}

func findAccessGroup(groups []AccessGroup, groupID uuid.UUID) *AccessGroup {
	for index := range groups {
		if groups[index].ID == groupID {
			return &groups[index]
		}
	}
	return nil
}

func matchesSearch(value, search string) bool {
	return search == "" || strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(search)))
}

func matchesStatus(active bool, status string) bool {
	switch status {
	case "active":
		return active
	case "inactive":
		return !active
	default:
		return true
	}
}

func pageByID[T any](items []T, cursor string, limit int, id func(T) uuid.UUID) (AccessListPage[T], error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	start := 0
	if cursor != "" {
		cursorID, err := uuid.Parse(cursor)
		if err != nil {
			return AccessListPage[T]{}, ErrInvalidAccessRequest
		}
		found := false
		for index, item := range items {
			if id(item) == cursorID {
				start, found = index+1, true
				break
			}
		}
		if !found {
			return AccessListPage[T]{}, ErrInvalidAccessRequest
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	hasMore := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	page := append([]T(nil), items[start:end]...)
	next := ""
	if hasMore && len(page) > 0 {
		next = id(page[len(page)-1]).String()
	}
	return AccessListPage[T]{Items: page, NextCursor: next, HasMore: hasMore}, nil
}
