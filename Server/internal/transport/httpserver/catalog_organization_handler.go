package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

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
	value, err := h.service.RenameOrganization(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}, organizationID, body.Name, uint64(body.Version))
	if h.respondCatalogMutationError(c, err) {
		respondCatalogOrganization(c, http.StatusOK, value)
	}
}

func respondCatalogOrganization(c *gin.Context, status int, value catalog.Organization) {
	c.JSON(status, contract.CatalogOrganizationResponse{
		Data: contract.CatalogOrganizationResource{Id: value.ID, Name: value.Name, Active: value.Active, Version: int64(value.Version)},
		Meta: responseMeta(c),
	})
}
