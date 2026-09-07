package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type deploymentPlanModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OwnerKind      string
	OwnerProjectID *uuid.UUID
	Name           string
	Description    string
	Active         bool
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (deploymentPlanModel) TableName() string { return "deployment_plans" }

type deploymentPlanVersionModel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	PlanID        uuid.UUID
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

func (deploymentPlanVersionModel) TableName() string { return "deployment_plan_versions" }

func deploymentPlanToModel(value deploydomain.DeploymentPlan) deploymentPlanModel {
	return deploymentPlanModel{
		ID: value.ID, OwnerKind: string(value.OwnerKind), OwnerProjectID: value.OwnerProjectID,
		Name: value.Name, Description: value.Description, Active: value.Active,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func deploymentPlanVersionToModel(value deploydomain.DeploymentPlanVersion) (deploymentPlanVersionModel, error) {
	document, err := marshalDeploymentPlanDocument(value.Document)
	if err != nil {
		return deploymentPlanVersionModel{}, err
	}
	return deploymentPlanVersionModel{
		ID: value.ID, PlanID: value.PlanID, VersionNumber: value.VersionNumber,
		Lifecycle: string(value.Lifecycle), Document: document, LockVersion: value.LockVersion,
		CreatedBy: value.CreatedBy, PublishedAt: value.PublishedAt, DisabledAt: value.DisabledAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func deploymentPlanFromModel(model deploymentPlanModel, versions []deploydomain.DeploymentPlanVersion) deploydomain.DeploymentPlan {
	return deploydomain.DeploymentPlan{
		ID: model.ID, OwnerKind: deploydomain.DeploymentPlanOwnerKind(model.OwnerKind),
		OwnerProjectID: model.OwnerProjectID, Name: model.Name, Description: model.Description,
		Active: model.Active, CreatedBy: model.CreatedBy, CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt, Versions: versions,
	}
}

func deploymentPlanVersionFromModel(model deploymentPlanVersionModel) (deploydomain.DeploymentPlanVersion, error) {
	document, err := unmarshalDeploymentPlanDocument(model.Document)
	if err != nil {
		return deploydomain.DeploymentPlanVersion{}, err
	}
	return deploydomain.DeploymentPlanVersion{
		ID: model.ID, PlanID: model.PlanID, VersionNumber: model.VersionNumber,
		Lifecycle: deploydomain.DefinitionLifecycle(model.Lifecycle), Document: document,
		LockVersion: model.LockVersion, CreatedBy: model.CreatedBy,
		PublishedAt: model.PublishedAt, DisabledAt: model.DisabledAt,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}, nil
}
