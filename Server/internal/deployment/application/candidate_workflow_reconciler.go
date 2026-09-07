package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// PendingWorkflowStart identifies one Request Version that has no Workflow instance yet.
type PendingWorkflowStart struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
}

// CandidateWorkflowStarter starts the pinned Workflow for a Request Version.
type CandidateWorkflowStarter interface {
	Start(context.Context, WorkflowStartInput) (deploydomain.WorkflowResult, error)
}

func (r *CandidateReconciler) startPendingWorkflows(ctx context.Context) error {
	values, err := r.repository.ListPendingWorkflowStarts(ctx)
	if err != nil {
		return fmt.Errorf("list pending Workflow starts: %w", err)
	}
	var startErr error
	for _, value := range values {
		_, err := r.workflows.Start(ctx, workflowStartInput(value))
		startErr = errors.Join(startErr, err)
	}
	return startErr
}

func workflowStartInput(value PendingWorkflowStart) WorkflowStartInput {
	return WorkflowStartInput{
		RequestVersionID: value.RequestVersionID,
		IdempotencyKey:   "candidate-workflow-start:" + value.RequestVersionID.String(),
		RequestID:        "worker-candidate-reconciliation",
	}
}
