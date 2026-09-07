package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

const requestDetailQuery = `
SELECT request.id, request.organization_id, request.project_id, request.environment_id,
       request.classification, request.updated_at, version.id AS version_id,
       version.version_number, version.status AS version_status, version.fingerprint,
       version.workflow_version_id, version.plan_version_id, version.title,
       version.change_description, version.issue_url, version.scheduled_for,
       version.lock_version, version.source_snapshot, version.created_at,
       instance.current_state_key AS workflow_state_key,
       execution.id AS execution_id, execution.status AS execution_status
FROM deployment_requests request
JOIN LATERAL (
  SELECT * FROM deployment_request_versions
  WHERE request_id = request.id ORDER BY version_number DESC LIMIT 1
) version ON true
LEFT JOIN deployment_workflow_instances instance ON instance.request_version_id = version.id
LEFT JOIN LATERAL (
  SELECT id, status FROM deployment_executions
  WHERE request_version_id = version.id ORDER BY attempt DESC LIMIT 1
) execution ON true
WHERE request.id = ?`

type requestDetailModel struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	Classification    string
	UpdatedAt         time.Time
	VersionID         uuid.UUID `gorm:"column:version_id"`
	VersionNumber     uint64
	VersionStatus     string
	Fingerprint       string
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	Title             string
	ChangeDescription string
	IssueURL          string
	ScheduledFor      *time.Time
	LockVersion       uint64
	SourceSnapshot    datatypes.JSON
	CreatedAt         time.Time
	WorkflowStateKey  string
	ExecutionID       uuid.UUID
	ExecutionStatus   string
}

func loadDeploymentRequest(ctx context.Context, db *gorm.DB, requestID uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	var model requestDetailModel
	if err := db.WithContext(ctx).Raw(requestDetailQuery, requestID).Scan(&model).Error; err != nil {
		return deploydomain.DeploymentRequestDetail{}, fmt.Errorf("load deployment request: %w", err)
	}
	if model.VersionID == uuid.Nil {
		return deploydomain.DeploymentRequestDetail{}, deployapp.ErrRequestNotFound
	}
	applications, err := loadRequestApplications(ctx, db, model.VersionID)
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	reviews, err := loadRequestReviews(ctx, db, model.VersionID)
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	return deploymentRequestDetailFromModel(model, applications, reviews), nil
}

func deploymentRequestDetailFromModel(model requestDetailModel, applications []deploydomain.DeploymentRequestApplicationSnapshot, reviews []deploydomain.DeploymentRequestReviewSnapshot) deploydomain.DeploymentRequestDetail {
	summary := deploydomain.DeploymentRequestSummary{ID: model.ID, OrganizationID: model.OrganizationID,
		ProjectID: model.ProjectID, EnvironmentID: model.EnvironmentID, Classification: model.Classification,
		Status: deploydomain.DeploymentRequestStatus(model.VersionStatus), Title: model.Title,
		ActiveVersionNumber: model.VersionNumber, ApplicationCount: len(applications), UpdatedAt: model.UpdatedAt}
	version := deploydomain.DeploymentRequestVersionSummary{ID: model.VersionID, RequestID: model.ID,
		VersionNumber: model.VersionNumber, Status: deploydomain.DeploymentRequestStatus(model.VersionStatus),
		Fingerprint: model.Fingerprint, WorkflowVersionID: model.WorkflowVersionID, PlanVersionID: model.PlanVersionID,
		Title: model.Title, Metadata: deploydomain.DeploymentRequestMetadata{ChangeDescription: model.ChangeDescription,
			IssueURL: model.IssueURL, ScheduledFor: model.ScheduledFor}, LockVersion: model.LockVersion,
		SourceSnapshot: model.SourceSnapshot, CreatedAt: model.CreatedAt}
	return deploydomain.DeploymentRequestDetail{Summary: summary, Version: version, Applications: applications,
		Reviews: reviews, WorkflowStateKey: model.WorkflowStateKey, ExecutionID: model.ExecutionID,
		ExecutionStatus: deploydomain.ExecutionStatus(model.ExecutionStatus)}
}

func loadRequestReviews(ctx context.Context, db *gorm.DB, versionID uuid.UUID) ([]deploydomain.DeploymentRequestReviewSnapshot, error) {
	var models []reviewTaskModel
	if err := db.WithContext(ctx).Where("request_version_id = ?", versionID).Order("stage_number, created_at").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load deployment request reviews: %w", err)
	}
	values := make([]deploydomain.DeploymentRequestReviewSnapshot, 0, len(models))
	for _, model := range models {
		values = append(values, requestReviewSnapshot(model))
	}
	return values, nil
}

func requestReviewSnapshot(model reviewTaskModel) deploydomain.DeploymentRequestReviewSnapshot {
	return deploydomain.DeploymentRequestReviewSnapshot{
		ID: model.ID, StateKey: model.StateKey, StageNumber: model.StageNumber,
		PolicyType: deploydomain.ReviewPolicyType(model.PolicyType), RequiredApprovals: model.RequiredApprovals,
		AllowSelfReview: model.AllowSelfReview, Status: deploydomain.ReviewTaskStatus(model.Status),
	}
}

func loadRequestApplications(ctx context.Context, db *gorm.DB, versionID uuid.UUID) ([]deploydomain.DeploymentRequestApplicationSnapshot, error) {
	var models []deploymentRequestApplicationModel
	if err := db.WithContext(ctx).Where("request_version_id = ?", versionID).Order("execution_order, application_key").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load deployment request Applications: %w", err)
	}
	values := make([]deploydomain.DeploymentRequestApplicationSnapshot, 0, len(models))
	for _, model := range models {
		value, err := requestApplicationSnapshot(ctx, db, model)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func requestApplicationSnapshot(ctx context.Context, db *gorm.DB, model deploymentRequestApplicationModel) (deploydomain.DeploymentRequestApplicationSnapshot, error) {
	var revisions []string
	if err := json.Unmarshal(model.TargetRevisions, &revisions); err != nil {
		return deploydomain.DeploymentRequestApplicationSnapshot{}, fmt.Errorf("unmarshal request target revisions: %w", err)
	}
	images, err := loadRequestImages(ctx, db, model.ID)
	if err != nil {
		return deploydomain.DeploymentRequestApplicationSnapshot{}, err
	}
	return deploydomain.DeploymentRequestApplicationSnapshot{ID: model.ID, ApplicationID: model.ApplicationID,
		ApplicationKey: model.ApplicationKey, LiveRevision: model.LiveRevision, TargetRevision: model.TargetRevision,
		TargetRevisions: revisions, ManifestHash: model.ManifestHash, DiffHash: model.DiffHash,
		DiffSnapshot: model.DiffSnapshot, SourceSnapshot: model.SourceSnapshot, Order: model.ExecutionOrder, Images: images}, nil
}

func loadRequestImages(ctx context.Context, db *gorm.DB, applicationID uuid.UUID) ([]deploydomain.DeploymentRequestImageSnapshot, error) {
	var models []deploymentRequestImageModel
	if err := db.WithContext(ctx).Where("request_application_id = ?", applicationID).Order("image_reference").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load deployment request images: %w", err)
	}
	values := make([]deploydomain.DeploymentRequestImageSnapshot, 0, len(models))
	for _, model := range models {
		values = append(values, deploydomain.DeploymentRequestImageSnapshot{ID: model.ID,
			ImageReference: model.ImageReference, Registry: model.Registry, Repository: model.Repository,
			Tag: model.ImageTag, Digest: model.ImageDigest})
	}
	return values, nil
}
