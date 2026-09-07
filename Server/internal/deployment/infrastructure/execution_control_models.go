package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type executionControlSourceModel struct {
	RequestID                 uuid.UUID
	OrganizationID            uuid.UUID
	ProjectID                 uuid.UUID
	EnvironmentID             uuid.UUID
	Classification            string
	WorkflowDocument          datatypes.JSON
	WorkflowInstanceID        uuid.UUID
	WorkflowInstanceVersionID uuid.UUID
	WorkflowState             string
	WorkflowStatus            string
	WorkflowLockVersion       uint64
	WorkflowStartedAt         time.Time
	WorkflowCompletedAt       *time.Time
	ExecutionID               uuid.UUID
	RequestVersionID          uuid.UUID
	PlanVersionID             uuid.UUID
	Attempt                   int
	ExecutionStatus           string
	TriggerKind               string
	LockVersion               uint64
	CreatedAt                 time.Time
	StartedAt                 *time.Time
	CompletedAt               *time.Time
}

type executionCommandModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	RequestVersionID  uuid.UUID
	ExecutionID       uuid.UUID
	ResultExecutionID *uuid.UUID
	CommandType       string
	ActorID           uuid.UUID
	IdempotencyKey    string
	RequestHash       string
	Reason            string
	ActualStates      datatypes.JSON `gorm:"type:jsonb"`
	OccurredAt        time.Time
}

func (executionCommandModel) TableName() string { return "deployment_execution_commands" }
