package application

import (
	"context"
	"fmt"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// ReassignReview replaces a pending task assignment after current management authorization.
func (s *WorkflowRuntimeService) ReassignReview(ctx context.Context, principal WorkflowPrincipal, input WorkflowReviewReassignmentInput) (deploydomain.ReviewTask, error) {
	if err := validateWorkflowReviewReassignmentInput(input); err != nil {
		return deploydomain.ReviewTask{}, err
	}
	snapshot, err := s.repository.Load(ctx, input.RequestVersionID)
	if err != nil {
		return deploydomain.ReviewTask{}, err
	}
	if err := validateReviewReassignmentSnapshot(snapshot, input); err != nil {
		return deploydomain.ReviewTask{}, err
	}
	if err := s.authorizeReviewManagement(ctx, principal, snapshot.Scope); err != nil {
		return deploydomain.ReviewTask{}, err
	}
	return s.applyReviewReassignment(ctx, principal, snapshot, input)
}

func (s *WorkflowRuntimeService) applyReviewReassignment(ctx context.Context, principal WorkflowPrincipal, snapshot WorkflowRuntimeSnapshot, input WorkflowReviewReassignmentInput) (deploydomain.ReviewTask, error) {
	policy := reassignedReviewPolicy(snapshot.CurrentReview.Policy, input)
	assignment, err := s.assignments.Resolve(ctx, policy, snapshot.Scope)
	if err != nil {
		return deploydomain.ReviewTask{}, fmt.Errorf("resolve review reassignment: %w", err)
	}
	updated, reassignment, err := snapshot.CurrentReview.Reassign(deploydomain.ReviewReassignment{
		ActorID: principal.UserID, Policy: policy, Assignment: assignment,
		Reason: input.Reason, ReassignedAt: s.clock.Now(),
	})
	if err != nil {
		return deploydomain.ReviewTask{}, workflowRuntimeInvalid(err)
	}
	prepared := preparedReviewReassignment{
		task: updated, previous: snapshot.CurrentReview.Assignment, value: reassignment,
	}
	return updated, s.persistReviewReassignment(ctx, principal, input, prepared)
}

type preparedReviewReassignment struct {
	task     deploydomain.ReviewTask
	previous deploydomain.ReviewAssignment
	value    deploydomain.ReviewReassignment
}

func (s *WorkflowRuntimeService) persistReviewReassignment(ctx context.Context, principal WorkflowPrincipal, input WorkflowReviewReassignmentInput, prepared preparedReviewReassignment) error {
	mutation := userRuntimeMutation(principal.UserID, input.RequestID, input.IdempotencyKey, s.clock.Now())
	return s.repository.ApplyReviewReassignment(ctx, WorkflowReviewReassignmentChange{
		Mutation: mutation, ExpectedLock: input.ExpectedLock, Task: prepared.task,
		PreviousAssignment: prepared.previous, Reassignment: prepared.value,
	})
}

func reassignedReviewPolicy(current deploydomain.ReviewPolicy, input WorkflowReviewReassignmentInput) deploydomain.ReviewPolicy {
	return deploydomain.ReviewPolicy{
		Type: current.Type, RequiredApprovals: current.RequiredApprovals,
		AllowSelfReview: current.AllowSelfReview, UserIDs: input.UserIDs, RoleIDs: input.RoleIDs,
	}
}

func (s *WorkflowRuntimeService) authorizeReviewManagement(ctx context.Context, principal WorkflowPrincipal, scope authz.Scope) error {
	permission, _ := authz.NewPermission("deployment_request.reassign")
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize workflow review reassignment: %w", err)
	}
	if !allowed {
		return ErrWorkflowForbidden
	}
	return nil
}

func validateReviewReassignmentSnapshot(snapshot WorkflowRuntimeSnapshot, input WorkflowReviewReassignmentInput) error {
	if err := validateRuntimeInstance(snapshot, input.ExpectedLock); err != nil {
		return err
	}
	if snapshot.CurrentReview == nil || snapshot.CurrentReview.ID != input.ReviewTaskID {
		return ErrWorkflowRuntimeNotFound
	}
	if snapshot.CurrentReview.Status != deploydomain.ReviewTaskPending && snapshot.CurrentReview.Status != deploydomain.ReviewTaskReassignmentRequired {
		return ErrWorkflowRuntimeConflict
	}
	return nil
}
