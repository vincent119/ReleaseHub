package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func deploymentReviewInput(c *gin.Context, versionID, taskID uuid.UUID, idempotencyKey string) (deployapp.WorkflowReviewInput, error) {
	var body contract.DeploymentReviewDecisionRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 || !body.Decision.Valid() {
		return deployapp.WorkflowReviewInput{}, errors.New("deployment review request is invalid")
	}
	return deployapp.WorkflowReviewInput{
		RequestVersionID: versionID, ReviewTaskID: taskID, ExpectedLock: uint64(body.ExpectedVersion),
		Decision: deploydomain.ReviewDecisionType(body.Decision), Reason: optionalString(body.Reason),
		IdempotencyKey: idempotencyKey, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func deploymentReviewReassignmentInput(c *gin.Context, versionID, taskID uuid.UUID, idempotencyKey string) (deployapp.WorkflowReviewReassignmentInput, error) {
	var body contract.DeploymentReviewReassignmentRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.WorkflowReviewReassignmentInput{}, errors.New("deployment review reassignment request is invalid")
	}
	return deployapp.WorkflowReviewReassignmentInput{
		RequestVersionID: versionID, ReviewTaskID: taskID, ExpectedLock: uint64(body.ExpectedVersion),
		UserIDs: body.UserIds, RoleIDs: body.RoleIds, Reason: body.Reason,
		IdempotencyKey: idempotencyKey, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func deploymentTransitionInput(c *gin.Context, versionID uuid.UUID, idempotencyKey string) (deployapp.WorkflowTransitionInput, error) {
	var body contract.DeploymentTransitionRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		return deployapp.WorkflowTransitionInput{}, errors.New("deployment transition request is invalid")
	}
	return deployapp.WorkflowTransitionInput{
		RequestVersionID: versionID, ExpectedLock: uint64(body.ExpectedVersion),
		TransitionKey: body.TransitionKey, Trigger: deploydomain.WorkflowTriggerManual,
		Reason: optionalString(body.Reason), IdempotencyKey: idempotencyKey,
		RequestID: c.GetHeader(requestIDHeader),
	}, nil
}
