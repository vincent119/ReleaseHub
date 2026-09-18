package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

// CreateAccessBindingsBatch grants multiple Roles to a Group atomically.
func (h *accessHandler) CreateAccessBindingsBatch(c *gin.Context, params contract.CreateAccessBindingsBatchParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateAccessBindingsBatchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Role binding request is invalid")
		return
	}
	value, err := h.service.CreateBindings(c.Request.Context(), accessPrincipal(user), accessMutation(c), authzapp.CreateBindingsInput{GroupID: body.GroupId, RoleIDs: body.RoleIds, OrganizationID: accessUUIDValue(body.OrganizationId), ScopeKind: string(body.ScopeKind), ProjectID: accessUUIDValue(body.ProjectId), EnvironmentID: body.EnvironmentId, ApplicationID: body.ApplicationId})
	if !h.respondAccessMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.AccessBindingsResponse{Data: accessBindingsResponse(value), Meta: responseMeta(c)})
}
