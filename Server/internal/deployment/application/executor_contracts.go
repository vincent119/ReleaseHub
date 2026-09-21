package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// ExecutionTarget binds one immutable Request Application to one Plan node.
type ExecutionTarget struct {
	Node        deploydomain.DeploymentPlanNode
	Preflight   PreflightTarget
	Status      string
	OperationID string
}

// ExecutionSnapshot is the durable input for one deployment attempt.
type ExecutionSnapshot struct {
	ID               uuid.UUID
	RequestVersionID uuid.UUID
	Status           string
	Plan             deploydomain.DeploymentPlanDocument
	Targets          []ExecutionTarget
}

// ExecutionNodeUpdate records one externally observable node transition.
type ExecutionNodeUpdate struct {
	ExecutionID    uuid.UUID
	ApplicationID  uuid.UUID
	Status         string
	OperationID    string
	SyncStatus     string
	HealthStatus   string
	ActualRevision string
	ActualImages   []deploydomain.DeploymentRequestImageSnapshot
	ErrorCode      string
	ErrorMessage   string
	UpdatedAt      time.Time
}

// ExecutionBlock records one immutable preflight failure.
type ExecutionBlock struct {
	ExecutionID          uuid.UUID
	RequestApplicationID uuid.UUID
	Code                 string
	OccurredAt           time.Time
}

type nodeExecution struct {
	executionID uuid.UUID
	target      ExecutionTarget
	operationID string
}

type nodeResult struct {
	key string
	err error
}

type conditionTiming struct {
	startedAt        time.Time
	stableSince      time.Time
	now              time.Time
	verifiedRevision string
}

// ExecutionRepository persists snapshots and node progress.
type ExecutionRepository interface {
	Prepare(context.Context, uuid.UUID, time.Time) (ExecutionSnapshot, error)
	Load(context.Context, uuid.UUID) (ExecutionSnapshot, error)
	MarkRunning(context.Context, uuid.UUID, time.Time) error
	UpdateNode(context.Context, ExecutionNodeUpdate) error
	Complete(context.Context, uuid.UUID, time.Time) (deploydomain.ExecutionStatus, error)
	Block(context.Context, ExecutionBlock) error
}

// DeploymentArgoClient exposes explicit mutation and observation operations.
type DeploymentArgoClient interface {
	GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error)
	SyncApplication(context.Context, argodomain.SyncRequest) (argodomain.Application, error)
	WatchApplication(context.Context, argodomain.ApplicationIdentity, string, string) (argodomain.ApplicationWatch, error)
}

// DeploymentExecutorOptions groups explicit worker dependencies.
type DeploymentExecutorOptions struct {
	Repository   ExecutionRepository
	Preflight    *PreflightService
	Argo         DeploymentArgoClient
	Locks        ApplicationLocks
	MaxParallel  int
	LockTTL      time.Duration
	WatchTimeout time.Duration
}
