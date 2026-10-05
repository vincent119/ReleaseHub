package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func deploymentRequestListQuery(params contract.ListDeploymentRequestsParams) deployapp.DeploymentRequestListQuery {
	query := deployapp.DeploymentRequestListQuery{Limit: params.Limit, Cursor: params.Cursor, Search: params.Search}
	if params.Status != nil {
		status := string(*params.Status)
		query.Status = &status
	}
	return query
}

func respondDeploymentRequestList(c *gin.Context, page deployapp.DeploymentRequestListPage) {
	meta := responseMeta(c)
	c.JSON(http.StatusOK, contract.DeploymentRequestListResponse{
		Data: deploymentRequestSummaries(page.Items),
		Meta: contract.CursorPageMeta{RequestId: meta.RequestId, Timestamp: meta.Timestamp,
			HasMore: page.HasMore, NextCursor: optionalCursor(page.NextCursor)},
	})
}
