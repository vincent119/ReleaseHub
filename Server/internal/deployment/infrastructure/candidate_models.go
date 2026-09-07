package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

type candidateTargetModel struct {
	ApplicationID         uuid.UUID
	OrganizationID        uuid.UUID
	ProjectID             uuid.UUID
	EnvironmentID         uuid.UUID
	ApplicationKey        string
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
	ArgoCDProject         string `gorm:"column:argocd_project"`
	WorkflowVersionID     uuid.UUID
	PlanVersionID         uuid.UUID
}

func (m candidateTargetModel) domain() deployapp.CandidateTarget {
	return deployapp.CandidateTarget{
		ApplicationID: m.ApplicationID, OrganizationID: m.OrganizationID,
		ProjectID: m.ProjectID, EnvironmentID: m.EnvironmentID, ApplicationKey: m.ApplicationKey,
		Argo:        argodomain.ApplicationIdentity{Namespace: m.ArgoCDNamespace, Name: m.ArgoCDApplicationName},
		ArgoProject: m.ArgoCDProject, WorkflowVersionID: m.WorkflowVersionID, PlanVersionID: m.PlanVersionID,
	}
}

type deploymentRequestModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	Classification string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (deploymentRequestModel) TableName() string { return "deployment_requests" }

type deploymentRequestVersionModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	RequestID         uuid.UUID
	VersionNumber     int
	Status            string
	Fingerprint       string
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	Title             string
	ChangeDescription string
	IssueURL          string
	ScheduledFor      *time.Time
	LockVersion       uint64
	CreatedBy         *uuid.UUID
	SourceSnapshot    datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (deploymentRequestVersionModel) TableName() string { return "deployment_request_versions" }

type deploymentRequestApplicationModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	RequestVersionID uuid.UUID
	ApplicationID    uuid.UUID
	ApplicationKey   string
	LiveRevision     string
	TargetRevision   string
	TargetRevisions  datatypes.JSON `gorm:"type:jsonb"`
	ManifestHash     string
	DiffHash         string
	DiffSnapshot     datatypes.JSON `gorm:"type:jsonb"`
	SourceSnapshot   datatypes.JSON `gorm:"type:jsonb"`
	ExecutionOrder   int
	CreatedAt        time.Time
}

func (deploymentRequestApplicationModel) TableName() string {
	return "deployment_request_applications"
}

type deploymentRequestImageModel struct {
	ID                   uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	RequestApplicationID uuid.UUID
	ImageReference       string
	Registry             string
	Repository           string
	ImageTag             string
	ImageDigest          string
	CreatedAt            time.Time
}

func (deploymentRequestImageModel) TableName() string { return "deployment_request_images" }

type candidateObservationModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	ApplicationID     uuid.UUID
	RequestVersionID  uuid.UUID
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	TargetRevision    string
	TargetRevisions   datatypes.JSON `gorm:"type:jsonb"`
	ManifestHash      string
	DiffHash          string
	DiffSnapshot      datatypes.JSON `gorm:"type:jsonb"`
	SourceSnapshot    datatypes.JSON `gorm:"type:jsonb"`
	ImageSnapshot     datatypes.JSON `gorm:"type:jsonb"`
	Fingerprint       string
	ObservedAt        time.Time
	CreatedAt         time.Time
}

func (candidateObservationModel) TableName() string {
	return "deployment_candidate_observations"
}
