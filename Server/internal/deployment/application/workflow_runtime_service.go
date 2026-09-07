package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// WorkflowRuntimeService coordinates the pure engine and atomic persistence ports.
type WorkflowRuntimeService struct {
	repository  WorkflowRuntimeRepository
	authorizer  WorkflowAuthorizer
	assignments ReviewAssignmentResolver
	clock       WorkflowClock
}

// NewWorkflowRuntimeService creates the request workflow use case.
func NewWorkflowRuntimeService(options WorkflowRuntimeServiceOptions) (*WorkflowRuntimeService, error) {
	if options.Repository == nil || options.Authorizer == nil || options.Assignments == nil || options.Clock == nil {
		return nil, errors.New("workflow runtime dependencies are required")
	}
	return &WorkflowRuntimeService{
		repository: options.Repository, authorizer: options.Authorizer,
		assignments: options.Assignments, clock: options.Clock,
	}, nil
}

// Start creates an instance from the request's pinned published workflow version.
func (s *WorkflowRuntimeService) Start(ctx context.Context, input WorkflowStartInput) (deploydomain.WorkflowResult, error) {
	if err := validateWorkflowStartInput(input); err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	result, review, now, err := s.prepareStart(ctx, input.RequestVersionID)
	if err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	err = s.repository.Start(ctx, WorkflowStartChange{Mutation: systemRuntimeMutation(input, now), Result: result, Review: review})
	return result, err
}

func (s *WorkflowRuntimeService) prepareStart(ctx context.Context, requestVersionID uuid.UUID) (deploydomain.WorkflowResult, *deploydomain.ReviewTask, time.Time, error) {
	snapshot, err := s.repository.Load(ctx, requestVersionID)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, time.Time{}, err
	}
	if snapshot.Instance != nil || snapshot.Version.Lifecycle != deploydomain.DefinitionPublished {
		return deploydomain.WorkflowResult{}, nil, time.Time{}, ErrWorkflowRuntimeConflict
	}
	now := s.clock.Now()
	result, err := startWorkflowEngine(snapshot, now)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, time.Time{}, workflowRuntimeInvalid(err)
	}
	review, err := s.reviewForResult(ctx, snapshot, result)
	return result, review, now, err
}

// Transition applies one currently authorized edge from the pinned graph.
func (s *WorkflowRuntimeService) Transition(ctx context.Context, principal WorkflowPrincipal, input WorkflowTransitionInput) (deploydomain.WorkflowResult, error) {
	if err := validateWorkflowTransitionInput(input); err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	result, review, err := s.executeTransition(ctx, principal, input)
	if err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	mutation := userRuntimeMutation(principal.UserID, input.RequestID, input.IdempotencyKey, s.clock.Now())
	change := workflowTransitionChange(input, result, review, mutation)
	return result, s.repository.ApplyTransition(ctx, change)
}

func (s *WorkflowRuntimeService) executeTransition(ctx context.Context, principal WorkflowPrincipal, input WorkflowTransitionInput) (deploydomain.WorkflowResult, *deploydomain.ReviewTask, error) {
	snapshot, err := s.repository.Load(ctx, input.RequestVersionID)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, err
	}
	if err := validateRuntimeInstance(snapshot, input.ExpectedLock); err != nil {
		return deploydomain.WorkflowResult{}, nil, err
	}
	command, err := s.authorizedTransitionCommand(ctx, principal, snapshot, input)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, err
	}
	result, err := transitionWorkflow(snapshot, command)
	if err != nil {
		return deploydomain.WorkflowResult{}, nil, workflowRuntimeInvalid(err)
	}
	review, err := s.reviewForResult(ctx, snapshot, result)
	return result, review, err
}

func (s *WorkflowRuntimeService) authorizedTransitionCommand(ctx context.Context, principal WorkflowPrincipal, snapshot WorkflowRuntimeSnapshot, input WorkflowTransitionInput) (deploydomain.WorkflowTransitionCommand, error) {
	allowed, err := s.authorizeTransition(ctx, principal, snapshot, input)
	if err != nil {
		return deploydomain.WorkflowTransitionCommand{}, err
	}
	if !allowed {
		return deploydomain.WorkflowTransitionCommand{}, ErrWorkflowForbidden
	}
	command := workflowTransitionCommand(input, principal.UserID, true, s.clock.Now())
	command.Facts = runtimeTransitionFacts(snapshot, input.Facts)
	return command, nil
}

// DecideReview records a decision after current permission and policy evaluation.
func (s *WorkflowRuntimeService) DecideReview(ctx context.Context, principal WorkflowPrincipal, input WorkflowReviewInput) (deploydomain.ReviewTask, error) {
	if err := validateWorkflowReviewInput(input); err != nil {
		return deploydomain.ReviewTask{}, err
	}
	task, decision, err := s.decideReview(ctx, principal, input)
	if err != nil {
		return deploydomain.ReviewTask{}, err
	}
	mutation := userRuntimeMutation(principal.UserID, input.RequestID, input.IdempotencyKey, s.clock.Now())
	change := workflowReviewChange(input, task, decision, mutation)
	return task, s.repository.ApplyReview(ctx, change)
}

func (s *WorkflowRuntimeService) decideReview(ctx context.Context, principal WorkflowPrincipal, input WorkflowReviewInput) (deploydomain.ReviewTask, deploydomain.ReviewDecision, error) {
	snapshot, err := s.repository.Load(ctx, input.RequestVersionID)
	if err != nil {
		return deploydomain.ReviewTask{}, deploydomain.ReviewDecision{}, err
	}
	if err := validateReviewSnapshot(snapshot, input); err != nil {
		return deploydomain.ReviewTask{}, deploydomain.ReviewDecision{}, err
	}
	if err := s.authorizeReview(ctx, principal, snapshot.Scope); err != nil {
		return deploydomain.ReviewTask{}, deploydomain.ReviewDecision{}, err
	}
	decision := newReviewDecision(principal.UserID, input, s.clock.Now())
	task, err := snapshot.CurrentReview.Decide(decision)
	if err != nil {
		return deploydomain.ReviewTask{}, deploydomain.ReviewDecision{}, workflowRuntimeInvalid(err)
	}
	return task, decision, nil
}

func newReviewDecision(reviewerID uuid.UUID, input WorkflowReviewInput, now time.Time) deploydomain.ReviewDecision {
	return deploydomain.ReviewDecision{
		ReviewerID: reviewerID, Decision: input.Decision, Reason: input.Reason, DecidedAt: now,
	}
}

func startWorkflowEngine(snapshot WorkflowRuntimeSnapshot, now time.Time) (deploydomain.WorkflowResult, error) {
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Version.Document)
	if err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	return engine.Start(snapshot.RequestVersionID, snapshot.Version.ID, now)
}

func transitionWorkflow(snapshot WorkflowRuntimeSnapshot, command deploydomain.WorkflowTransitionCommand) (deploydomain.WorkflowResult, error) {
	engine, err := deploydomain.NewWorkflowEngine(snapshot.Version.Document)
	if err != nil {
		return deploydomain.WorkflowResult{}, err
	}
	return engine.Transition(*snapshot.Instance, command)
}

func workflowTransitionCommand(input WorkflowTransitionInput, actorID uuid.UUID, allowed bool, now time.Time) deploydomain.WorkflowTransitionCommand {
	return deploydomain.WorkflowTransitionCommand{
		TransitionKey: input.TransitionKey, Trigger: input.Trigger, ActorID: actorID,
		PermissionGranted: allowed, Reason: input.Reason, Facts: input.Facts, OccurredAt: now,
	}
}

func (s *WorkflowRuntimeService) reviewForResult(ctx context.Context, snapshot WorkflowRuntimeSnapshot, result deploydomain.WorkflowResult) (*deploydomain.ReviewTask, error) {
	if result.ReviewPolicy == nil {
		return nil, nil
	}
	assignment, err := s.assignments.Resolve(ctx, *result.ReviewPolicy, snapshot.Scope)
	if err != nil {
		return nil, fmt.Errorf("resolve review assignment: %w", err)
	}
	task, err := deploydomain.NewReviewTask(deploydomain.ReviewTaskDraft{
		RequestVersionID: snapshot.RequestVersionID, StateKey: result.Instance.CurrentStateKey,
		StageNumber: snapshot.NextReviewStage, RequestCreatorID: snapshot.RequestCreatorID,
		Policy: *result.ReviewPolicy, Assignment: assignment,
	})
	return &task, err
}

func (s *WorkflowRuntimeService) authorizeTransition(ctx context.Context, principal WorkflowPrincipal, snapshot WorkflowRuntimeSnapshot, input WorkflowTransitionInput) (bool, error) {
	permission, err := transitionPermission(snapshot, input)
	if err != nil {
		return false, err
	}
	return s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled,
		Permission: permission, Scope: snapshot.Scope,
	})
}

func (s *WorkflowRuntimeService) authorizeReview(ctx context.Context, principal WorkflowPrincipal, scope authz.Scope) error {
	permission, _ := authz.NewPermission("deployment_request.review")
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize workflow review: %w", err)
	}
	if !allowed {
		return ErrWorkflowForbidden
	}
	return nil
}

func transitionPermission(snapshot WorkflowRuntimeSnapshot, input WorkflowTransitionInput) (authz.Permission, error) {
	for _, transition := range snapshot.Version.Document.Transitions {
		if transition.From == snapshot.Instance.CurrentStateKey && transition.Key == strings.TrimSpace(input.TransitionKey) {
			permission, err := authz.NewPermission(transition.Permission)
			return permission, err
		}
	}
	return "", ErrWorkflowRuntimeInvalid
}

func validateRuntimeInstance(snapshot WorkflowRuntimeSnapshot, expected uint64) error {
	if snapshot.Instance == nil {
		return ErrWorkflowRuntimeNotFound
	}
	if snapshot.Instance.LockVersion != expected {
		return ErrWorkflowRuntimeConflict
	}
	return nil
}

func validateReviewSnapshot(snapshot WorkflowRuntimeSnapshot, input WorkflowReviewInput) error {
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

func runtimeTransitionFacts(snapshot WorkflowRuntimeSnapshot, provided map[deploydomain.WorkflowFact]string) map[deploydomain.WorkflowFact]string {
	result := make(map[deploydomain.WorkflowFact]string, len(provided)+1)
	for fact, value := range provided {
		result[fact] = value
	}
	if snapshot.CurrentReview != nil && snapshot.Instance != nil && snapshot.CurrentReview.StateKey == snapshot.Instance.CurrentStateKey {
		result[deploydomain.WorkflowFactReviewStatus] = string(snapshot.CurrentReview.Status)
	} else {
		delete(result, deploydomain.WorkflowFactReviewStatus)
	}
	return result
}

func systemRuntimeMutation(input WorkflowStartInput, now time.Time) WorkflowRuntimeMutation {
	return WorkflowRuntimeMutation{
		RequestID: input.RequestID, IdempotencyKey: strings.TrimSpace(input.IdempotencyKey), OccurredAt: now.UTC(),
	}
}

func workflowTransitionChange(input WorkflowTransitionInput, result deploydomain.WorkflowResult, review *deploydomain.ReviewTask, mutation WorkflowRuntimeMutation) WorkflowTransitionChange {
	return WorkflowTransitionChange{
		Mutation: mutation, ExpectedLock: input.ExpectedLock, Result: result, Review: review,
	}
}

func workflowReviewChange(input WorkflowReviewInput, task deploydomain.ReviewTask, decision deploydomain.ReviewDecision, mutation WorkflowRuntimeMutation) WorkflowReviewChange {
	return WorkflowReviewChange{
		Mutation: mutation, ExpectedLock: input.ExpectedLock, Task: task, Decision: decision,
	}
}

func userRuntimeMutation(actorID uuid.UUID, requestID, idempotencyKey string, now time.Time) WorkflowRuntimeMutation {
	return WorkflowRuntimeMutation{
		ActorID: actorID, RequestID: requestID,
		IdempotencyKey: strings.TrimSpace(idempotencyKey), OccurredAt: now.UTC(),
	}
}

func workflowRuntimeInvalid(err error) error {
	return fmt.Errorf("%w: %v", ErrWorkflowRuntimeInvalid, err)
}
