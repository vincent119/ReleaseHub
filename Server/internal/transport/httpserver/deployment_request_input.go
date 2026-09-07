package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func deploymentRequestMetadataInput(c *gin.Context, requestID, versionID uuid.UUID) (deployapp.MetadataVersionChange, error) {
	var body contract.UpdateDeploymentRequestVersionRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.MetadataVersionChange{}, errors.New("deployment request metadata is invalid")
	}
	return deployapp.MetadataVersionChange{RequestID: requestID, RequestVersion: versionID,
		ExpectedVersion: uint64(body.ExpectedVersion), Metadata: deploydomain.DeploymentRequestMetadata{
			ChangeDescription: optionalString(body.ChangeDescription), IssueURL: optionalString(body.IssueUrl), ScheduledFor: body.ScheduledFor,
		}}, nil
}
