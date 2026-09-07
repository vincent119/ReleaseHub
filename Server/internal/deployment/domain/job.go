package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidJob     = errors.New("invalid deployment job")
	ErrNoJobAvailable = errors.New("no deployment job available")
	ErrJobConflict    = errors.New("deployment job idempotency conflict")
	ErrStaleJobLease  = errors.New("stale deployment job lease")
)

type JobType string

const (
	JobObserveApplication   JobType = "ObserveApplication"
	JobCreateRequestVersion JobType = "CreateRequestVersion"
	JobAdvanceWorkflow      JobType = "AdvanceWorkflow"
	JobExecuteDeployment    JobType = "ExecuteDeployment"
	JobReconcileExecution   JobType = "ReconcileExecution"
	JobProjectNotification  JobType = "ProjectNotification"
	JobExpireNotification   JobType = "ExpireNotification"
)

type DeploymentJob struct {
	ID             uuid.UUID
	Type           JobType
	AggregateType  string
	AggregateID    uuid.UUID
	Payload        json.RawMessage
	Status         string
	IdempotencyKey string
	AvailableAt    time.Time
	Attempts       int
	MaxAttempts    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DeploymentJobDraft struct {
	Type           JobType
	AggregateType  string
	AggregateID    uuid.UUID
	Payload        json.RawMessage
	IdempotencyKey string
	AvailableAt    time.Time
	MaxAttempts    int
}

type JobLease struct {
	Job          DeploymentJob
	Owner        string
	FencingToken uint64
	ExpiresAt    time.Time
}

func NewDeploymentJob(draft DeploymentJobDraft, now time.Time) (DeploymentJob, error) {
	if !validJobDraft(draft) {
		return DeploymentJob{}, ErrInvalidJob
	}
	availableAt := draft.AvailableAt.UTC()
	if availableAt.IsZero() {
		availableAt = now.UTC()
	}
	return DeploymentJob{
		ID: uuid.New(), Type: draft.Type, AggregateType: strings.TrimSpace(draft.AggregateType),
		AggregateID: draft.AggregateID, Payload: bytes.Clone(draft.Payload), Status: "Pending",
		IdempotencyKey: strings.TrimSpace(draft.IdempotencyKey), AvailableAt: availableAt,
		MaxAttempts: draft.MaxAttempts, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

func validJobDraft(draft DeploymentJobDraft) bool {
	return validJobType(draft.Type) && validJobText(draft.AggregateType, 128) &&
		draft.AggregateID != uuid.Nil && validJSONObject(draft.Payload) &&
		validJobText(draft.IdempotencyKey, 255) && draft.MaxAttempts > 0
}

func validJobType(value JobType) bool {
	switch value {
	case JobObserveApplication, JobCreateRequestVersion, JobAdvanceWorkflow,
		JobExecuteDeployment, JobReconcileExecution, JobProjectNotification, JobExpireNotification:
		return true
	default:
		return false
	}
}

func validJobText(value string, maximum int) bool {
	length := len(strings.TrimSpace(value))
	return length > 0 && length <= maximum
}

func validJSONObject(value json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}
