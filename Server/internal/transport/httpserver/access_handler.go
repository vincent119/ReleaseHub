package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type accessManagementService interface {
	Load(context.Context, authzapp.AccessPrincipal) (authzapp.AccessSnapshot, error)
	CreateGroup(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateGroupInput) (authzapp.AccessGroup, error)
	DisableGroup(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
	CreateRole(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateRoleInput) (authzapp.AccessRole, error)
	AddMembership(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID, uuid.UUID) (authzapp.AccessMembership, error)
	CreateBinding(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateBindingInput) (authzapp.AccessBinding, error)
	CreateDeny(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateDenyInput) (authzapp.AccessDeny, error)
	DisableUser(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
}

type accessHandler struct {
	authn   *authHandler
	service accessManagementService
}

// GetAccessManagementSnapshot returns the access records the current administrator may manage.
func (h *accessHandler) GetAccessManagementSnapshot(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "ACCESS_MANAGEMENT_UNAVAILABLE", "Access management is unavailable")
		return
	}
	value, err := h.service.Load(c.Request.Context(), authzapp.AccessPrincipal{UserID: user.ID, Disabled: user.Disabled})
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessManagementSnapshotResponse{Data: accessSnapshotResponse(value), Meta: responseMeta(c)})
}

func respondAccessReadError(c *gin.Context, err error) {
	if errors.Is(err, authzapp.ErrAccessManagementNotFound) {
		respondError(c, http.StatusNotFound, "ACCESS_MANAGEMENT_NOT_FOUND", "Access management was not found")
		return
	}
	respondError(c, http.StatusInternalServerError, "ACCESS_MANAGEMENT_READ_FAILED", "Unable to read access management")
}

func accessSnapshotResponse(value authzapp.AccessSnapshot) contract.AccessManagementSnapshot {
	return contract.AccessManagementSnapshot{
		CanManagePlatform: value.CanManagePlatform, ManagedProjectIds: value.ManagedProjects,
		Users: accessUsersResponse(value.Users), Groups: accessGroupsResponse(value.Groups),
		Roles: accessRolesResponse(value.Roles), Permissions: accessPermissionsResponse(value.Permissions),
		Memberships: accessMembershipsResponse(value.Memberships), Bindings: accessBindingsResponse(value.Bindings),
		Denies: accessDeniesResponse(value.Denies),
	}
}

func accessUsersResponse(values []authzapp.AccessUser) []contract.AccessUser {
	result := make([]contract.AccessUser, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessUser{Id: value.ID, Username: value.Username, Disabled: value.DisabledAt != nil})
	}
	return result
}

func accessGroupsResponse(values []authzapp.AccessGroup) []contract.AccessGroup {
	result := make([]contract.AccessGroup, 0, len(values))
	for _, value := range values {
		result = append(result, accessGroupResponse(value))
	}
	return result
}

func accessRolesResponse(values []authzapp.AccessRole) []contract.AccessRole {
	result := make([]contract.AccessRole, 0, len(values))
	for _, value := range values {
		result = append(result, accessRoleResponse(value))
	}
	return result
}

func accessPermissionsResponse(values []authzapp.AccessPermission) []contract.AccessPermission {
	result := make([]contract.AccessPermission, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessPermission{Key: value.Key, PlatformOnly: value.PlatformOnly, ProjectRoleDelegable: value.ProjectRoleDelegable})
	}
	return result
}

func accessMembershipsResponse(values []authzapp.AccessMembership) []contract.AccessMembership {
	result := make([]contract.AccessMembership, 0, len(values))
	for _, value := range values {
		result = append(result, accessMembershipResponse(value))
	}
	return result
}

func accessBindingsResponse(values []authzapp.AccessBinding) []contract.AccessBinding {
	result := make([]contract.AccessBinding, 0, len(values))
	for _, value := range values {
		result = append(result, accessBindingResponse(value))
	}
	return result
}

func accessDeniesResponse(values []authzapp.AccessDeny) []contract.AccessDeny {
	result := make([]contract.AccessDeny, 0, len(values))
	for _, value := range values {
		result = append(result, accessDenyResponse(value))
	}
	return result
}

// CreateAccessGroup creates a Group after mutation and authorization validation.
func (h *accessHandler) CreateAccessGroup(c *gin.Context, params contract.CreateAccessGroupParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessGroupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Group request is invalid")
		return
	}
	value, err := h.service.CreateGroup(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateGroupInput{OwnerKind: string(body.OwnerKind), OwnerID: body.OwnerId, Name: body.Name, OIDCViewerOnly: body.OidcViewerOnly})
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessGroupResponse{Data: accessGroupResponse(value), Meta: responseMeta(c)})
}

// DisableAccessGroup disables a Group and its policies.
func (h *accessHandler) DisableAccessGroup(c *gin.Context, groupID uuid.UUID, params contract.DisableAccessGroupParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.DisableGroup(c.Request.Context(), accessPrincipal(user), accessMutation(c), groupID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

// CreateAccessMembership adds one user to a Group.
func (h *accessHandler) CreateAccessMembership(c *gin.Context, groupID uuid.UUID, params contract.CreateAccessMembershipParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessMembershipRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.UserId == uuid.Nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Membership request is invalid")
		return
	}
	value, err := h.service.AddMembership(c.Request.Context(), accessPrincipal(user), accessMutation(c), groupID, body.UserId)
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessMembershipResponse{Data: accessMembershipResponse(value), Meta: responseMeta(c)})
}

// CreateAccessRole creates a custom Role with an initial permission set.
func (h *accessHandler) CreateAccessRole(c *gin.Context, params contract.CreateAccessRoleParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessRoleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Role request is invalid")
		return
	}
	value, err := h.service.CreateRole(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateRoleInput{OwnerKind: string(body.OwnerKind), OwnerID: body.OwnerId, Name: body.Name, Permissions: body.Permissions})
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessRoleResponse{Data: accessRoleResponse(value), Meta: responseMeta(c)})
}

// CreateAccessBinding grants one Role to a Group at an exact scope.
func (h *accessHandler) CreateAccessBinding(c *gin.Context, params contract.CreateAccessBindingParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessBindingRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Role binding request is invalid")
		return
	}
	value, err := h.service.CreateBinding(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateBindingInput{GroupID: body.GroupId, RoleID: body.RoleId, OrganizationID: body.OrganizationId, ScopeKind: string(body.ScopeKind), ProjectID: body.ProjectId, EnvironmentID: body.EnvironmentId, ApplicationID: body.ApplicationId})
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessBindingResponse{Data: accessBindingResponse(value), Meta: responseMeta(c)})
}

// CreateAccessDeny creates one explicit deny policy.
func (h *accessHandler) CreateAccessDeny(c *gin.Context, params contract.CreateAccessDenyParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessDenyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Deny policy request is invalid")
		return
	}
	value, err := h.service.CreateDeny(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateDenyInput{GroupID: body.GroupId, Permission: body.Permission, OrganizationID: body.OrganizationId, ScopeKind: string(body.ScopeKind), ProjectID: body.ProjectId, EnvironmentID: body.EnvironmentId, ApplicationID: body.ApplicationId})
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessDenyResponse{Data: accessDenyResponse(value), Meta: responseMeta(c)})
}

// DisableAccessUser locally disables an account and revokes its sessions.
func (h *accessHandler) DisableAccessUser(c *gin.Context, userID uuid.UUID, params contract.DisableAccessUserParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.DisableUser(c.Request.Context(), accessPrincipal(user), accessMutation(c), userID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *accessHandler) respondAccessMutationError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, authzapp.ErrAccessManagementNotFound) {
		respondError(c, http.StatusNotFound, "ACCESS_MANAGEMENT_NOT_FOUND", "Access management resource was not found")
		return false
	}
	respondError(c, http.StatusConflict, "ACCESS_MANAGEMENT_MUTATION_REJECTED", "Access management change was rejected")
	return false
}

func accessPrincipal(user identity.User) authzapp.AccessPrincipal {
	return authzapp.AccessPrincipal{UserID: user.ID, Disabled: user.Disabled}
}

func accessMutation(c *gin.Context) authzapp.AccessMutation {
	return authzapp.AccessMutation{RequestID: c.GetHeader(requestIDHeader)}
}

func accessGroupResponse(value authzapp.AccessGroup) contract.AccessGroup {
	return contract.AccessGroup{Id: value.ID, OwnerKind: contract.AccessGroupOwnerKind(value.OwnerKind), OwnerId: value.OwnerID, Name: value.Name, OidcViewerOnly: value.OIDCViewerOnly, Disabled: value.DisabledAt != nil}
}

func accessRoleResponse(value authzapp.AccessRole) contract.AccessRole {
	return contract.AccessRole{Id: value.ID, OwnerKind: contract.AccessRoleOwnerKind(value.OwnerKind), OwnerId: value.OwnerID, Name: value.Name, SystemKey: value.SystemKey, Active: value.Active, Permissions: value.Permissions}
}

func accessMembershipResponse(value authzapp.AccessMembership) contract.AccessMembership {
	return contract.AccessMembership{Id: value.ID, GroupId: value.GroupID, UserId: value.UserID, Source: contract.AccessMembershipSource(value.Source), Active: value.Active}
}

func accessBindingResponse(value authzapp.AccessBinding) contract.AccessBinding {
	return contract.AccessBinding{Id: value.ID, GroupId: value.GroupID, RoleId: value.RoleID, OrganizationId: value.OrganizationID, ScopeKind: contract.AccessBindingScopeKind(value.ScopeKind), ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, ApplicationId: value.ApplicationID, Active: value.Active}
}

func accessDenyResponse(value authzapp.AccessDeny) contract.AccessDeny {
	return contract.AccessDeny{Id: value.ID, GroupId: value.GroupID, Permission: value.Permission, OrganizationId: value.OrganizationID, ScopeKind: contract.AccessDenyScopeKind(value.ScopeKind), ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, ApplicationId: value.ApplicationID, Active: value.Active}
}
