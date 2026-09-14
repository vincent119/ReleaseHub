package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

// GetAccessCapabilities returns server-owned collection capabilities.
func (h *accessHandler) GetAccessCapabilities(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.Capabilities(c.Request.Context(), accessPrincipal(user))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	collections := make([]contract.AccessCollectionCapability, 0, len(value.Collections))
	for _, item := range value.Collections {
		collections = append(collections, contract.AccessCollectionCapability{Key: contract.AccessCollectionCapabilityKey(item.Key), Visible: item.Visible, CanCreate: item.CanCreate})
	}
	c.JSON(http.StatusOK, contract.AccessCapabilitiesResponse{Data: contract.AccessCapabilities{Collections: collections, Permissions: accessPermissionsResponse(value.Permissions)}, Meta: responseMeta(c)})
}

// ListAccessUsers returns one filtered user page.
func (h *accessHandler) ListAccessUsers(c *gin.Context, params contract.ListAccessUsersParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListUsers(c.Request.Context(), accessPrincipal(user), stringValue(params.Search), stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessUserListResponse{Data: accessUsersResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessGroups returns one filtered Group page.
func (h *accessHandler) ListAccessGroups(c *gin.Context, params contract.ListAccessGroupsParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListGroups(c.Request.Context(), accessPrincipal(user), stringValue(params.Search), stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessGroupListResponse{Data: accessGroupsResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessRoles returns one filtered Role page.
func (h *accessHandler) ListAccessRoles(c *gin.Context, params contract.ListAccessRolesParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListRoles(c.Request.Context(), accessPrincipal(user), stringValue(params.Search), stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessRoleListResponse{Data: accessRolesResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessMemberships returns one filtered membership page.
func (h *accessHandler) ListAccessMemberships(c *gin.Context, params contract.ListAccessMembershipsParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListMemberships(c.Request.Context(), accessPrincipal(user), params.GroupId, params.UserId, stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessMembershipListResponse{Data: accessMembershipsResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessBindings returns one filtered Role-binding page.
func (h *accessHandler) ListAccessBindings(c *gin.Context, params contract.ListAccessBindingsParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListBindings(c.Request.Context(), accessPrincipal(user), stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessBindingListResponse{Data: accessBindingsResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessDenies returns one filtered explicit-deny page.
func (h *accessHandler) ListAccessDenies(c *gin.Context, params contract.ListAccessDeniesParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ListDenies(c.Request.Context(), accessPrincipal(user), stringEnum(params.Status), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessDenyListResponse{Data: accessDeniesResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// ListAccessMembershipCandidates returns minimal eligible user identities for one Group.
func (h *accessHandler) ListAccessMembershipCandidates(c *gin.Context, groupID uuid.UUID, params contract.ListAccessMembershipCandidatesParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.MembershipCandidates(c.Request.Context(), accessPrincipal(user), groupID, stringValue(params.Query), cursorValue(params.Cursor), limitValue(params.Limit))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessUserCandidatesResponse{Data: accessUserCandidatesResponse(value.Items), Meta: accessPageMeta(c, value.NextCursor, value.HasMore)})
}

// GetAccessScopeOptions returns legal downstream options after scope selection.
func (h *accessHandler) GetAccessScopeOptions(c *gin.Context, scopeKind contract.GetAccessScopeOptionsParamsScopeKind, scopeID string) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	value, err := h.service.ScopeOptions(c.Request.Context(), accessPrincipal(user), string(scopeKind), scopeID)
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessScopeOptionsResponse{Data: contract.AccessScopeOptions{Groups: accessGroupsResponse(value.Groups), Roles: accessRolesResponse(value.Roles), Permissions: accessPermissionsResponse(value.Permissions)}, Meta: responseMeta(c)})
}

// GetAccessManagementSnapshot retains the legacy read contract during client migration.
func (h *accessHandler) GetAccessManagementSnapshot(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "ACCESS_MANAGEMENT_UNAVAILABLE", "Access management is unavailable")
		return
	}
	value, err := h.service.Load(c.Request.Context(), accessPrincipal(user))
	if err != nil {
		respondAccessReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.AccessManagementSnapshotResponse{Data: accessSnapshotResponse(value), Meta: responseMeta(c)})
}

func respondAccessReadError(c *gin.Context, err error) {
	if errors.Is(err, authzapp.ErrInvalidAccessRequest) {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Access management query is invalid")
		return
	}
	if errors.Is(err, authzapp.ErrAccessManagementNotFound) {
		respondError(c, http.StatusNotFound, "ACCESS_MANAGEMENT_NOT_FOUND", "Access management was not found")
		return
	}
	respondError(c, http.StatusInternalServerError, "ACCESS_MANAGEMENT_READ_FAILED", "Unable to read access management")
}
