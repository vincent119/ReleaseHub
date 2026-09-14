package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
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
	Capabilities(context.Context, authzapp.AccessPrincipal) (authzapp.AccessCapabilities, error)
	ListUsers(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessUser], error)
	ListGroups(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessGroup], error)
	ListRoles(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessRole], error)
	ListMemberships(context.Context, authzapp.AccessPrincipal, *uuid.UUID, *uuid.UUID, string, string, int) (authzapp.AccessListPage[authzapp.AccessMembership], error)
	ListBindings(context.Context, authzapp.AccessPrincipal, string, string, int) (authzapp.AccessListPage[authzapp.AccessBinding], error)
	ListDenies(context.Context, authzapp.AccessPrincipal, string, string, int) (authzapp.AccessListPage[authzapp.AccessDeny], error)
	MembershipCandidates(context.Context, authzapp.AccessPrincipal, uuid.UUID, string, string, int) (authzapp.AccessListPage[authzapp.AccessUser], error)
	ScopeOptions(context.Context, authzapp.AccessPrincipal, string, string) (authzapp.AccessScopeOptions, error)
	RevokeMembership(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
	DisableRole(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
	RevokeBinding(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
	RevokeDeny(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error
}

type localUserCreator interface {
	CreateUser(context.Context, uuid.UUID, string, string, string) (identity.User, error)
}

type accessHandler struct {
	authn      *authHandler
	service    accessManagementService
	localUsers localUserCreator
}

// CreateAccessUser creates one local identity and first-login credential after fresh platform authorization.
func (h *accessHandler) CreateAccessUser(c *gin.Context, params contract.CreateAccessUserParams) {
	_, actor, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if h.localUsers == nil {
		respondError(c, http.StatusServiceUnavailable, "LOCAL_USER_CREATION_UNAVAILABLE", "Local user creation is unavailable")
		return
	}
	var body contract.CreateAccessUserRequest
	if err := c.ShouldBindJSON(&body); err != nil || len(body.InitialPassword) < 8 || len(body.InitialPassword) > 72 {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Local user request is invalid")
		return
	}
	user, ok := h.createLocalUser(c, actor, body)
	if !ok {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessUserResponse{Data: contract.AccessUser{Id: user.ID, Username: user.Username, Disabled: false, AllowedActions: []contract.AccessAction{contract.AccessAction("disable")}}, Meta: responseMeta(c)})
}

func (h *accessHandler) createLocalUser(c *gin.Context, actor identity.User, body contract.CreateAccessUserRequest) (identity.User, bool) {
	capabilities, err := h.service.Capabilities(c.Request.Context(), accessPrincipal(actor))
	if err != nil || !collectionCanCreate(capabilities, "users") {
		respondError(c, http.StatusNotFound, "ACCESS_MANAGEMENT_NOT_FOUND", "Access management resource was not found")
		return identity.User{}, false
	}
	user, err := h.localUsers.CreateUser(c.Request.Context(), actor.ID, c.GetHeader(requestIDHeader), body.Username, body.InitialPassword)
	if err != nil {
		respondLocalUserCreateError(c, err)
		return identity.User{}, false
	}
	return user, true
}

func respondLocalUserCreateError(c *gin.Context, err error) {
	if errors.Is(err, identityapp.ErrInvalidLocalUser) {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Local user request is invalid")
		return
	}
	if errors.Is(err, identityapp.ErrLocalUsernameConflict) {
		respondError(c, http.StatusConflict, "ACCESS_USERNAME_CONFLICT", "The username is unavailable")
		return
	}
	respondError(c, http.StatusInternalServerError, "ACCESS_USER_CREATE_FAILED", "Unable to create local user")
}

func collectionCanCreate(value authzapp.AccessCapabilities, key string) bool {
	for _, collection := range value.Collections {
		if collection.Key == key {
			return collection.Visible && collection.CanCreate
		}
	}
	return false
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
	value, err := h.service.CreateBinding(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateBindingInput{GroupID: body.GroupId, RoleID: body.RoleId, OrganizationID: accessUUIDValue(body.OrganizationId), ScopeKind: string(body.ScopeKind), ProjectID: accessUUIDValue(body.ProjectId), EnvironmentID: body.EnvironmentId, ApplicationID: body.ApplicationId})
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
	value, err := h.service.CreateDeny(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateDenyInput{GroupID: body.GroupId, Permission: body.Permission, OrganizationID: accessUUIDValue(body.OrganizationId), ScopeKind: string(body.ScopeKind), ProjectID: accessUUIDValue(body.ProjectId), EnvironmentID: body.EnvironmentId, ApplicationID: body.ApplicationId})
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

// RevokeAccessMembership revokes one active manual Group membership.
func (h *accessHandler) RevokeAccessMembership(c *gin.Context, membershipID uuid.UUID, params contract.RevokeAccessMembershipParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.RevokeMembership(c.Request.Context(), accessPrincipal(user), accessMutation(c), membershipID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

// DisableAccessRole disables one active custom Role.
func (h *accessHandler) DisableAccessRole(c *gin.Context, roleID uuid.UUID, params contract.DisableAccessRoleParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.DisableRole(c.Request.Context(), accessPrincipal(user), accessMutation(c), roleID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

// RevokeAccessBinding revokes one active Role binding.
func (h *accessHandler) RevokeAccessBinding(c *gin.Context, bindingID uuid.UUID, params contract.RevokeAccessBindingParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.RevokeBinding(c.Request.Context(), accessPrincipal(user), accessMutation(c), bindingID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

// RevokeAccessDeny revokes one active explicit deny policy.
func (h *accessHandler) RevokeAccessDeny(c *gin.Context, denyID uuid.UUID, params contract.RevokeAccessDenyParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	if !h.respondAccessMutationError(c, h.service.RevokeDeny(c.Request.Context(), accessPrincipal(user), accessMutation(c), denyID)) {
		return
	}
	c.Status(http.StatusNoContent)
}

type accessErrorMapping struct {
	target        error
	status        int
	code, message string
}

var accessMutationErrors = []accessErrorMapping{
	{authzapp.ErrAccessManagementNotFound, http.StatusNotFound, "ACCESS_MANAGEMENT_NOT_FOUND", "Access management resource was not found"},
	{authzapp.ErrInvalidAccessRequest, http.StatusBadRequest, "INVALID_REQUEST", "Access management request is invalid"},
	{authzapp.ErrSelfDisable, http.StatusConflict, "ACCESS_SELF_DISABLE_FORBIDDEN", "Administrators cannot disable their own account"},
	{authzapp.ErrLastPlatformManager, http.StatusConflict, "ACCESS_LAST_PLATFORM_MANAGER", "The last platform-management path cannot be removed"},
	{authzapp.ErrProtectedAccessResource, http.StatusConflict, "ACCESS_RESOURCE_PROTECTED", "The access-management resource is system managed"},
	{authzapp.ErrAccessManagementConflict, http.StatusConflict, "ACCESS_MANAGEMENT_CONFLICT", "The access-management resource is no longer active"},
}

func (h *accessHandler) respondAccessMutationError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	for _, mapping := range accessMutationErrors {
		if errors.Is(err, mapping.target) {
			respondError(c, mapping.status, mapping.code, mapping.message)
			return false
		}
	}
	respondError(c, http.StatusInternalServerError, "ACCESS_MANAGEMENT_MUTATION_FAILED", "Unable to apply the access management change")
	return false
}
