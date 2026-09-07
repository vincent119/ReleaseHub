package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type workflowRuntimeSourceModel struct {
	RequestID         uuid.UUID
	RequestVersionID  uuid.UUID
	RequestCreatorID  *uuid.UUID
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	WorkflowID        uuid.UUID
	WorkflowVersionID uuid.UUID
	VersionNumber     uint64
	Lifecycle         string
	Document          datatypes.JSON
	VersionLock       uint64
	VersionCreatedBy  uuid.UUID
	VersionCreatedAt  time.Time
	VersionUpdatedAt  time.Time
	PublishedAt       *time.Time
	DisabledAt        *time.Time
	InstanceID        *uuid.UUID
	CurrentStateKey   *string
	InstanceStatus    *string
	InstanceLock      *uint64
	StartedAt         *time.Time
	CompletedAt       *time.Time
}

type workflowInstanceModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	RequestVersionID  uuid.UUID
	WorkflowVersionID uuid.UUID
	CurrentStateKey   string
	Status            string
	LockVersion       uint64
	StartedAt         time.Time
	CompletedAt       *time.Time
	UpdatedAt         time.Time
}

func (workflowInstanceModel) TableName() string { return "deployment_workflow_instances" }

type workflowTransitionRecordModel struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkflowInstanceID uuid.UUID
	RequestVersionID   uuid.UUID
	FromStateKey       string
	ToStateKey         string
	TransitionKey      string
	ActorID            *uuid.UUID
	Reason             string
	IdempotencyKey     string
	OccurredAt         time.Time
}

func (workflowTransitionRecordModel) TableName() string {
	return "deployment_workflow_transitions"
}

type reviewTaskModel struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkflowInstanceID uuid.UUID
	RequestVersionID   uuid.UUID
	StateKey           string
	StageNumber        int
	PolicyType         string
	RequiredApprovals  int
	AllowSelfReview    bool
	AssigneeSnapshot   datatypes.JSON `gorm:"type:jsonb"`
	Status             string
	CreatedAt          time.Time
	ClosedAt           *time.Time
}

func (reviewTaskModel) TableName() string { return "deployment_review_tasks" }

type reviewDecisionModel struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	ReviewTaskID       uuid.UUID
	ReviewerID         uuid.UUID
	Decision           string
	Reason             string
	PermissionSnapshot datatypes.JSON `gorm:"type:jsonb"`
	IdempotencyKey     string
	DecidedAt          time.Time
}

func (reviewDecisionModel) TableName() string { return "deployment_review_decisions" }

type reviewReassignmentModel struct {
	ID                       uuid.UUID `gorm:"type:uuid;primaryKey"`
	ReviewTaskID             uuid.UUID
	ActorID                  uuid.UUID
	PreviousAssigneeSnapshot datatypes.JSON `gorm:"type:jsonb"`
	AssigneeSnapshot         datatypes.JSON `gorm:"type:jsonb"`
	Reason                   string
	PermissionSnapshot       datatypes.JSON `gorm:"type:jsonb"`
	IdempotencyKey           string
	ReassignedAt             time.Time
}

func (reviewReassignmentModel) TableName() string {
	return "deployment_review_reassignments"
}

type deploymentJobModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	JobType          string
	AggregateType    string
	AggregateID      uuid.UUID
	Payload          datatypes.JSON `gorm:"type:jsonb"`
	Status           string
	IdempotencyKey   string
	AvailableAt      time.Time
	LeaseOwner       *string
	LeaseExpiresAt   *time.Time
	FencingToken     uint64
	Attempts         int
	MaxAttempts      int
	LastErrorCode    string
	LastErrorMessage string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (deploymentJobModel) TableName() string { return "deployment_jobs" }
