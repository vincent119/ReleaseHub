package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func retryExecutionInput(c *gin.Context, requestID, versionID uuid.UUID, key string) (deployapp.RetryExecutionInput, error) {
	var body contract.DeploymentRetryRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.RetryExecutionInput{}, errors.New("retry deployment input is invalid")
	}
	return deployapp.RetryExecutionInput{
		RequestID: requestID, RequestVersionID: versionID, ApplicationIDs: body.ApplicationIds,
		ExpectedVersion: uint64(body.ExpectedVersion), IdempotencyKey: key, RequestTraceID: c.GetHeader(requestIDHeader),
	}, nil
}

func terminateExecutionInput(c *gin.Context, requestID, versionID uuid.UUID, key string) (deployapp.TerminateExecutionInput, error) {
	var body contract.DeploymentTerminateRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.TerminateExecutionInput{}, errors.New("terminate deployment input is invalid")
	}
	return deployapp.TerminateExecutionInput{
		RequestID: requestID, RequestVersionID: versionID, Reason: body.Reason,
		ExpectedVersion: uint64(body.ExpectedVersion), IdempotencyKey: key, RequestTraceID: c.GetHeader(requestIDHeader),
	}, nil
}

func unlockExecutionInput(c *gin.Context, requestID, versionID uuid.UUID, key string) (deployapp.UnlockExecutionInput, error) {
	var body contract.DeploymentUnlockRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.UnlockExecutionInput{}, errors.New("unlock deployment input is invalid")
	}
	states := make([]deployapp.ActualState, 0, len(body.ActualStates))
	for _, state := range body.ActualStates {
		states = append(states, deployapp.ActualState{ApplicationID: state.ApplicationId,
			Revision: state.Revision, Images: executionImagesFromContract(state.Images)})
	}
	return deployapp.UnlockExecutionInput{
		RequestID: requestID, RequestVersionID: versionID, Reason: body.Reason,
		ExpectedVersion: uint64(body.ExpectedVersion), ActualStates: states,
		IdempotencyKey: key, RequestTraceID: c.GetHeader(requestIDHeader),
	}, nil
}

func executionImagesFromContract(values []contract.DeploymentImageSnapshot) []deploydomain.DeploymentRequestImageSnapshot {
	result := make([]deploydomain.DeploymentRequestImageSnapshot, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.DeploymentRequestImageSnapshot{
			ImageReference: value.ImageReference, Registry: value.Registry,
			Repository: value.Repository, Tag: value.Tag, Digest: value.Digest,
		})
	}
	return result
}
