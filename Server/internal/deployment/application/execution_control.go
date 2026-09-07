package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	ErrExecutionNotFound  = errors.New("deployment execution was not found")
	ErrExecutionForbidden = errors.New("deployment execution operation is not allowed")
	ErrExecutionConflict  = errors.New("deployment execution changed concurrently")
	ErrExecutionInvalid   = errors.New("deployment execution command is invalid")
	ErrExecutionStopped   = errors.New("deployment execution was stopped")
)

// DeploymentExecution is the application read model for one attempt.
type DeploymentExecution struct {
	ID               uuid.UUID
	RequestVersionID uuid.UUID
	PlanVersionID    uuid.UUID
	Attempt          int
	Status           string
	TriggerKind      string
	LockVersion      uint64
	Nodes            []DeploymentExecutionNode
	CreatedAt        time.Time
	StartedAt        *time.Time
	CompletedAt      *time.Time
}

// DeploymentExecutionNode is one Application result within an attempt.
type DeploymentExecutionNode struct {
	ID                   uuid.UUID
	RequestApplicationID uuid.UUID
	ApplicationID        uuid.UUID
	NodeKey              string
	Status               string
	OperationID          string
	SyncStatus           string
	HealthStatus         string
	ActualRevision       string
	ActualImages         []deploydomain.DeploymentRequestImageSnapshot
	ErrorCode            string
	ErrorMessage         string
	Identity             argodomain.ApplicationIdentity
	ArgoProject          string
}

// ExecutionControlSnapshot contains authorization and pinned workflow facts.
type ExecutionControlSnapshot struct {
	RequestID        uuid.UUID
	Scope            authz.Scope
	Workflow         deploydomain.WorkflowDocument
	WorkflowInstance deploydomain.WorkflowInstance
	Classification   string
	Execution        DeploymentExecution
}

// ActualState is a fresh Application observation used before forced unlock.
type ActualState struct {
	ApplicationID uuid.UUID
	Revision      string
	Images        []deploydomain.DeploymentRequestImageSnapshot
}

// ExecutionMutation carries idempotency and audit identity.
type ExecutionMutation struct {
	ActorID        uuid.UUID
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	OccurredAt     time.Time
}

// ExecutionCommandReplay identifies one immutable command result lookup.
type ExecutionCommandReplay struct {
	RequestVersionID uuid.UUID
	Command          string
	IdempotencyKey   string
	RequestHash      string
}

// ExecutionRetryChange creates one business attempt from failed nodes.
type ExecutionRetryChange struct {
	Mutation         ExecutionMutation
	WorkflowResult   deploydomain.WorkflowResult
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	ExecutionID      uuid.UUID
	ExpectedVersion  uint64
	ApplicationIDs   []uuid.UUID
}

// ExecutionTerminateChange terminates one unconverged attempt without unlocking it.
type ExecutionTerminateChange struct {
	Mutation        ExecutionMutation
	WorkflowResult  deploydomain.WorkflowResult
	ExecutionID     uuid.UUID
	ExpectedVersion uint64
	Reason          string
}

// ExecutionUnlockChange saves confirmed actual state before releasing locks.
type ExecutionUnlockChange struct {
	Mutation        ExecutionMutation
	ExecutionID     uuid.UUID
	ExpectedVersion uint64
	Reason          string
	ActualStates    []ActualState
}

type ExecutionControlRepository interface {
	LoadByID(context.Context, uuid.UUID) (ExecutionControlSnapshot, error)
	LoadLatest(context.Context, uuid.UUID, uuid.UUID) (ExecutionControlSnapshot, error)
	Retry(context.Context, ExecutionRetryChange) (DeploymentExecution, error)
	Terminate(context.Context, ExecutionTerminateChange) (DeploymentExecution, error)
	Unlock(context.Context, ExecutionUnlockChange) (DeploymentExecution, error)
	Replay(context.Context, ExecutionCommandReplay) (DeploymentExecution, bool, error)
}

type ExecutionActualStateReader interface {
	ReadActualState(context.Context, DeploymentExecutionNode) (ActualState, error)
}

type ExecutionOperationTerminator interface {
	TerminateApplication(context.Context, argodomain.ApplicationIdentity, string) error
}

// ExecutionControlServiceOptions groups explicit D10 dependencies.
type ExecutionControlServiceOptions struct {
	Repository   ExecutionControlRepository
	Authorizer   WorkflowAuthorizer
	ActualStates ExecutionActualStateReader
	Terminator   ExecutionOperationTerminator
	Clock        WorkflowClock
}

// RetryExecutionInput requests a failed-only business retry.
type RetryExecutionInput struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	ApplicationIDs   []uuid.UUID
	ExpectedVersion  uint64
	IdempotencyKey   string
	RequestTraceID   string
}

// TerminateExecutionInput requests an explicit workflow-governed stop.
type TerminateExecutionInput struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	Reason           string
	ExpectedVersion  uint64
	IdempotencyKey   string
	RequestTraceID   string
}

// UnlockExecutionInput confirms every actual state before releasing locks.
type UnlockExecutionInput struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	Reason           string
	ExpectedVersion  uint64
	ActualStates     []ActualState
	IdempotencyKey   string
	RequestTraceID   string
}
