package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type deploymentHistoryService interface {
	List(context.Context, deployapp.RequestPrincipal, deployapp.DeploymentHistoryQuery) (deployapp.DeploymentHistoryPage, error)
}

// ListDeploymentHistory returns successful deployment snapshots in one authorized scope.
func (h *deploymentHandler) ListDeploymentHistory(c *gin.Context, params contract.ListDeploymentHistoryParams) {
	principal, ok := h.historyPrincipal(c)
	if !ok {
		return
	}
	page, err := h.history.List(c.Request.Context(), principal, deploymentHistoryQuery(params))
	if !respondDeploymentHistoryError(c, err) {
		return
	}
	meta := responseMeta(c)
	c.JSON(http.StatusOK, contract.DeploymentHistoryListResponse{
		Data: deploymentHistoryItems(page.Items),
		Meta: contract.CursorPageMeta{RequestId: meta.RequestId, Timestamp: meta.Timestamp,
			HasMore: page.HasMore, NextCursor: optionalCursor(page.NextCursor)},
	})
}

func (h *deploymentHandler) historyPrincipal(c *gin.Context) (deployapp.RequestPrincipal, bool) {
	if h.history == nil {
		h.requireDeploymentRead(c)
		return deployapp.RequestPrincipal{}, false
	}
	_, user, ok := h.authn.authenticate(c)
	return deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
}

func deploymentHistoryQuery(params contract.ListDeploymentHistoryParams) deployapp.DeploymentHistoryQuery {
	query := deployapp.DeploymentHistoryQuery{ProjectID: params.ProjectId, EnvironmentID: params.EnvironmentId, Limit: 20}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
	}
	if params.Limit != nil {
		query.Limit = int(*params.Limit)
	}
	return query
}

func deploymentHistoryItems(values []deploydomain.DeploymentHistoryItem) []contract.DeploymentHistoryItem {
	result := make([]contract.DeploymentHistoryItem, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentHistoryItem{
			RequestId: value.RequestID, RequestVersionId: value.RequestVersionID,
			Classification: contract.DeploymentHistoryItemClassification(value.Classification),
			CompletedAt:    value.CompletedAt, Applications: deploymentRequestApplications(value.Applications),
		})
	}
	return result
}

func optionalCursor(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func respondDeploymentHistoryError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrHistoryNotFound) || errors.Is(err, deployapp.ErrHistoryForbidden) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_HISTORY_NOT_FOUND", "Deployment history was not found")
	} else if errors.Is(err, deployapp.ErrHistoryInvalid) {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Deployment history query is invalid")
	} else {
		respondError(c, http.StatusInternalServerError, "DEPLOYMENT_HISTORY_FAILED", "Unable to read deployment history")
	}
	return false
}
