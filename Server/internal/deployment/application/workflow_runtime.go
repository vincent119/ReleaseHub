package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	// ErrWorkflowRuntimeNotFound hides unavailable request workflow state.
	ErrWorkflowRuntimeNotFound = errors.New("workflow runtime was not found")
	// ErrWorkflowRuntimeConflict identifies stale or duplicate commands.
	ErrWorkflowRuntimeConflict = errors.New("workflow runtime changed concurrently")
	// ErrWorkflowRuntimeInvalid identifies commands rejected by the pinned graph.
	ErrWorkflowRuntimeInvalid = errors.New("workflow runtime command is invalid")
)

// WorkflowRuntimeSnapshot contains the immutable graph and current request workflow state.
type WorkflowRuntimeSnapshot struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	RequestCreatorID uuid.UUID
	Scope            authz.Scope
	Version          deploydomain.ReleaseWorkflowVersion
	Instance         *deploydomain.WorkflowInstance
	CurrentReview    *deploydomain.ReviewTask
	NextReviewStage  int
}

// WorkflowRuntimeMutation carries command identity and audit correlation.
type WorkflowRuntimeMutation struct {
	ActorID        uuid.UUID
	RequestID      string
	IdempotencyKey string
	OccurredAt     time.Time
}

// WorkflowStartChange persists one newly started instance.
type WorkflowStartChange struct {
	Mutation WorkflowRuntimeMutation
	Result   deploydomain.WorkflowResult
	Review   *deploydomain.ReviewTask
}

// WorkflowTransitionChange persists one optimistic state transition.
type WorkflowTransitionChange struct {
	Mutation     WorkflowRuntimeMutation
	ExpectedLock uint64
	Result       deploydomain.WorkflowResult
	Review       *deploydomain.ReviewTask
}

// WorkflowReviewChange persists one append-only review decision.
type WorkflowReviewChange struct {
	Mutation     WorkflowRuntimeMutation
	ExpectedLock uint64
	Task         deploydomain.ReviewTask
	Decision     deploydomain.ReviewDecision
}

// WorkflowReviewReassignmentChange persists one audited assignment replacement.
type WorkflowReviewReassignmentChange struct {
	Mutation           WorkflowRuntimeMutation
	ExpectedLock       uint64
	Task               deploydomain.ReviewTask
	PreviousAssignment deploydomain.ReviewAssignment
	Reassignment       deploydomain.ReviewReassignment
}

// WorkflowRuntimeRepository persists runtime changes atomically with audit and intent.
type WorkflowRuntimeRepository interface {
	Load(context.Context, uuid.UUID) (WorkflowRuntimeSnapshot, error)
	Start(context.Context, WorkflowStartChange) error
	ApplyTransition(context.Context, WorkflowTransitionChange) error
	ApplyReview(context.Context, WorkflowReviewChange) error
	ApplyReviewReassignment(context.Context, WorkflowReviewReassignmentChange) error
}

// ReviewAssignmentResolver captures eligible users and current role memberships.
type ReviewAssignmentResolver interface {
	Resolve(context.Context, deploydomain.ReviewPolicy, authz.Scope) (deploydomain.ReviewAssignment, error)
}

// WorkflowRuntimeServiceOptions groups runtime engine dependencies.
type WorkflowRuntimeServiceOptions struct {
	Repository  WorkflowRuntimeRepository
	Authorizer  WorkflowAuthorizer
	Assignments ReviewAssignmentResolver
	Clock       WorkflowClock
}

// WorkflowStartInput starts the pinned workflow for one request version.
type WorkflowStartInput struct {
	RequestVersionID uuid.UUID
	IdempotencyKey   string
	RequestID        string
}

// WorkflowTransitionInput requests one typed graph edge.
type WorkflowTransitionInput struct {
	RequestVersionID uuid.UUID
	ExpectedLock     uint64
	TransitionKey    string
	Trigger          deploydomain.WorkflowTrigger
	Reason           string
	Facts            map[deploydomain.WorkflowFact]string
	IdempotencyKey   string
	RequestID        string
}

// WorkflowReviewInput records one reviewer decision.
type WorkflowReviewInput struct {
	RequestVersionID uuid.UUID
	ReviewTaskID     uuid.UUID
	ExpectedLock     uint64
	Decision         deploydomain.ReviewDecisionType
	Reason           string
	IdempotencyKey   string
	RequestID        string
}

// WorkflowReviewReassignmentInput replaces the eligible reviewers of one pending task.
type WorkflowReviewReassignmentInput struct {
	RequestVersionID uuid.UUID
	ReviewTaskID     uuid.UUID
	ExpectedLock     uint64
	UserIDs          []uuid.UUID
	RoleIDs          []uuid.UUID
	Reason           string
	IdempotencyKey   string
	RequestID        string
}
