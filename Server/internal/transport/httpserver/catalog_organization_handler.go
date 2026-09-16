package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

// CreateCatalogOrganization creates a tenant boundary after CSRF and platform authorization checks.
func (h *catalogHandler) CreateCatalogOrganization(c *gin.Context, params contract.CreateCatalogOrganizationParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateNamedResourceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Organization request is invalid")
		return
	}
	value, err := h.service.CreateOrganization(c.Request.Context(), catalogPrincipal(user), catalogMutation(c), body.Name)
	if h.respondCatalogMutationError(c, err) {
		respondCatalogOrganization(c, http.StatusCreated, value)
	}
}

// UpdateCatalogOrganization renames a tenant boundary after CSRF and platform authorization checks.
func (h *catalogHandler) UpdateCatalogOrganization(c *gin.Context, organizationID uuid.UUID, params contract.UpdateCatalogOrganizationParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.UpdateCatalogOrganizationRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Version < 1 {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Organization request is invalid")
		return
	}
	value, err := h.service.RenameOrganization(c.Request.Context(), catalogPrincipal(user), catalogMutation(c), organizationID, body.Name, uint64(body.Version))
	if h.respondCatalogMutationError(c, err) {
		respondCatalogOrganization(c, http.StatusOK, value)
	}
}

// DeleteCatalogOrganization deactivates an empty non-default tenant boundary.
func (h *catalogHandler) DeleteCatalogOrganization(c *gin.Context, organizationID uuid.UUID, params contract.DeleteCatalogOrganizationParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok || !validOrganizationDeletion(c, params.ExpectedVersion) {
		return
	}
	err := h.service.DeleteOrganization(c.Request.Context(), catalogPrincipal(user), catalogMutation(c), organizationID, uint64(params.ExpectedVersion))
	if h.respondCatalogMutationError(c, err) {
		c.Status(http.StatusNoContent)
	}
}

func validOrganizationDeletion(c *gin.Context, version int64) bool {
	if version > 0 {
		return true
	}
	respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Organization deletion request is invalid")
	return false
}

func catalogPrincipal(user identity.User) catalogapp.Principal {
	return catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}
}

func catalogMutation(c *gin.Context) catalogapp.Mutation {
	return catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}
}

func respondCatalogOrganization(c *gin.Context, status int, value catalog.Organization) {
	c.JSON(status, contract.CatalogOrganizationResponse{
		Data: contract.CatalogOrganizationResource{Id: value.ID, Name: value.Name, Active: value.Active, Version: int64(value.Version)},
		Meta: responseMeta(c),
	})
}
