package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type deploymentExecutionModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	RequestVersionID uuid.UUID
	PlanVersionID    uuid.UUID
	Attempt          int
	Status           string
	TriggerKind      string
	TriggeredBy      *uuid.UUID
	PlanSnapshot     datatypes.JSON `gorm:"type:jsonb"`
	LockVersion      uint64
	StartedAt        *time.Time
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (deploymentExecutionModel) TableName() string { return "deployment_executions" }

type deploymentExecutionNodeModel struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExecutionID          uuid.UUID
	RequestApplicationID uuid.UUID
	ApplicationID        uuid.UUID
	NodeKey              string
	Status               string
	OperationID          string
	SyncStatus           string
	HealthStatus         string
	ActualRevision       string
	ActualImages         datatypes.JSON `gorm:"type:jsonb"`
	ErrorCode            string
	ErrorMessage         string
	StartedAt            *time.Time
	CompletedAt          *time.Time
	UpdatedAt            time.Time
}

func (deploymentExecutionNodeModel) TableName() string { return "deployment_execution_nodes" }

type executionApplicationModel struct {
	ID                    uuid.UUID
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
	ArgoCDProject         string `gorm:"column:argocd_project"`
}
