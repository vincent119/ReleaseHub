package application

import (
	"strings"

	"github.com/google/uuid"
)

func validateWorkflowStartInput(input WorkflowStartInput) error {
	if input.RequestVersionID == uuid.Nil || !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrWorkflowRuntimeInvalid
	}
	return nil
}

func validateWorkflowTransitionInput(input WorkflowTransitionInput) error {
	if input.RequestVersionID == uuid.Nil || input.ExpectedLock == 0 || strings.TrimSpace(input.TransitionKey) == "" {
		return ErrWorkflowRuntimeInvalid
	}
	if !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrWorkflowRuntimeInvalid
	}
	return nil
}

func validateWorkflowReviewInput(input WorkflowReviewInput) error {
	if input.RequestVersionID == uuid.Nil || input.ReviewTaskID == uuid.Nil || input.ExpectedLock == 0 {
		return ErrWorkflowRuntimeInvalid
	}
	if !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrWorkflowRuntimeInvalid
	}
	return nil
}

func validateWorkflowReviewReassignmentInput(input WorkflowReviewReassignmentInput) error {
	if input.RequestVersionID == uuid.Nil || input.ReviewTaskID == uuid.Nil || input.ExpectedLock == 0 {
		return ErrWorkflowRuntimeInvalid
	}
	if len(input.UserIDs) == 0 && len(input.RoleIDs) == 0 {
		return ErrWorkflowRuntimeInvalid
	}
	if strings.TrimSpace(input.Reason) == "" || len(strings.TrimSpace(input.Reason)) > 10000 {
		return ErrWorkflowRuntimeInvalid
	}
	if !validRuntimeIdempotencyKey(input.IdempotencyKey) {
		return ErrWorkflowRuntimeInvalid
	}
	return nil
}

func validRuntimeIdempotencyKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) >= 1 && len(trimmed) <= 255
}
