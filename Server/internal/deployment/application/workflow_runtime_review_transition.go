package application

import (
	"context"
	"errors"
	"fmt"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (s *WorkflowRuntimeService) attachApprovedReviewTransition(ctx context.Context, snapshot WorkflowRuntimeSnapshot, change *WorkflowReviewChange) error {
	result, review, found, err := s.approvedReviewTransition(ctx, snapshot, change.Task)
	if err != nil || !found {
		return err
	}
	change.Result = &result
	change.NextReview = review
	return nil
}

func (s *WorkflowRuntimeService) approvedReviewTransition(ctx context.Context, snapshot WorkflowRuntimeSnapshot, task deploydomain.ReviewTask) (deploydomain.WorkflowResult, *deploydomain.ReviewTask, bool, error) {
	if snapshot.Instance == nil || task.Status != deploydomain.ReviewTaskApproved || task.StateKey != snapshot.Instance.CurrentStateKey {
		return deploydomain.WorkflowResult{}, nil, false, nil
	}
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Version.Document)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, false, workflowRuntimeInvalid(err)
	}
	result, found, err := engine.ApplyReviewResult(*snapshot.Instance, task.Status, s.clock.Now())
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, false, workflowRuntimeInvalid(err)
	}
	if !found {
		return deploydomain.WorkflowResult{}, nil, false, nil
	}
	review, err := s.reviewForResult(ctx, snapshot, result)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, false, err
	}
	return result, review, true, nil
}

// RecoverApprovedReviews advances persisted approvals left behind by older runtime versions.
func (s *WorkflowRuntimeService) RecoverApprovedReviews(ctx context.Context) error {
	values, err := s.repository.ListApprovedReviewRecoveries(ctx)
	if err != nil {
		return fmt.Errorf("list approved review recoveries: %w", err)
	}
	var recoveryErr error
	for _, value := range values {
		recoveryErr = errors.Join(recoveryErr, s.recoverApprovedReview(ctx, value))
	}
	return recoveryErr
}

func (s *WorkflowRuntimeService) recoverApprovedReview(ctx context.Context, value ApprovedReviewRecovery) error {
	snapshot, err := s.repository.Load(ctx, value.RequestVersionID)
	if err != nil {
		return err
	}
	if snapshot.CurrentReview == nil || snapshot.Instance == nil {
		return nil
	}
	result, review, found, err := s.approvedReviewTransition(ctx, snapshot, *snapshot.CurrentReview)
	if err != nil || !found {
		return err
	}
	change := WorkflowTransitionChange{
		Mutation: systemRuntimeMutation(WorkflowStartInput{
			RequestVersionID: value.RequestVersionID,
			IdempotencyKey:   "workflow-review-satisfied:" + snapshot.CurrentReview.ID.String(),
			RequestID:        "worker-workflow-review-recovery",
		}, s.clock.Now()),
		ExpectedLock: snapshot.Instance.LockVersion, Result: result, Review: review,
	}
	err = s.repository.ApplyTransition(ctx, change)
	if errors.Is(err, ErrWorkflowRuntimeConflict) {
		return nil
	}
	return err
}
