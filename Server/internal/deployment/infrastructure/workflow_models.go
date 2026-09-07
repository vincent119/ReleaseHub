package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type releaseWorkflowModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name        string
	Description string
	Active      bool
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (releaseWorkflowModel) TableName() string { return "release_workflows" }

type releaseWorkflowVersionModel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkflowID    uuid.UUID
	VersionNumber uint64
	Lifecycle     string
	Document      datatypes.JSON `gorm:"type:jsonb"`
	LockVersion   uint64
	CreatedBy     uuid.UUID
	PublishedAt   *time.Time
	DisabledAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (releaseWorkflowVersionModel) TableName() string { return "release_workflow_versions" }

func workflowToModel(value deploydomain.ReleaseWorkflow) releaseWorkflowModel {
	return releaseWorkflowModel{
		ID: value.ID, Name: value.Name, Description: value.Description,
		Active: value.Active, CreatedBy: value.CreatedBy,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func workflowVersionToModel(value deploydomain.ReleaseWorkflowVersion) (releaseWorkflowVersionModel, error) {
	document, err := marshalWorkflowDocument(value.Document)
	if err != nil {
		return releaseWorkflowVersionModel{}, err
	}
	return releaseWorkflowVersionModel{
		ID: value.ID, WorkflowID: value.WorkflowID, VersionNumber: value.VersionNumber,
		Lifecycle: string(value.Lifecycle), Document: document, LockVersion: value.LockVersion,
		CreatedBy: value.CreatedBy, PublishedAt: value.PublishedAt, DisabledAt: value.DisabledAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func workflowFromModel(model releaseWorkflowModel, versions []deploydomain.ReleaseWorkflowVersion) deploydomain.ReleaseWorkflow {
	return deploydomain.ReleaseWorkflow{
		ID: model.ID, Name: model.Name, Description: model.Description,
		Active: model.Active, CreatedBy: model.CreatedBy,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, Versions: versions,
	}
}

func workflowVersionFromModel(model releaseWorkflowVersionModel) (deploydomain.ReleaseWorkflowVersion, error) {
	document, err := unmarshalWorkflowDocument(model.Document)
	if err != nil {
		return deploydomain.ReleaseWorkflowVersion{}, err
	}
	return deploydomain.ReleaseWorkflowVersion{
		ID: model.ID, WorkflowID: model.WorkflowID, VersionNumber: model.VersionNumber,
		Lifecycle: deploydomain.DefinitionLifecycle(model.Lifecycle), Document: document,
		LockVersion: model.LockVersion, CreatedBy: model.CreatedBy,
		PublishedAt: model.PublishedAt, DisabledAt: model.DisabledAt,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}, nil
}
