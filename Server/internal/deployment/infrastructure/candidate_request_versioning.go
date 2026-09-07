package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type candidateVersionMetadata struct {
	ChangeDescription string
	IssueURL          string
	ScheduledFor      *time.Time
	CreatedBy         *uuid.UUID
}

type activeCandidateVersion struct {
	RequestID         uuid.UUID
	RequestVersionID  uuid.UUID
	VersionNumber     int
	Fingerprint       string
	ChangeDescription string
	IssueURL          string
	ScheduledFor      *time.Time
	CreatedBy         *uuid.UUID
}

const activeCandidateVersionQuery = `
SELECT request.id AS request_id,
       version.id AS request_version_id,
       version.version_number,
       version.fingerprint,
       version.change_description,
       version.issue_url,
       version.scheduled_for,
       version.created_by
FROM deployment_request_applications application
JOIN deployment_request_versions version ON version.id = application.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE request.organization_id = ?
  AND request.project_id = ?
  AND request.environment_id = ?
  AND version.workflow_version_id = ?
  AND version.plan_version_id = ?
  AND version.status NOT IN ('Succeeded', 'Failed', 'Superseded', 'Terminated', 'Deploying')
ORDER BY version.created_at DESC
LIMIT 1`

func upsertCandidateRequest(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) error {
	previous, err := findActiveCandidateVersion(ctx, tx, observation)
	if err != nil {
		return err
	}
	if previous == nil {
		return insertInitialDeploymentRequest(ctx, tx, observation)
	}
	return supersedeCandidateVersion(ctx, tx, observation, *previous)
}

func findActiveCandidateVersion(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) (*activeCandidateVersion, error) {
	var value activeCandidateVersion
	err := tx.WithContext(ctx).Raw(activeCandidateVersionQuery,
		observation.OrganizationID, observation.ProjectID,
		observation.EnvironmentID, observation.WorkflowVersionID, observation.PlanVersionID,
	).Scan(&value).Error
	if err != nil {
		return nil, fmt.Errorf("find active deployment request version: %w", err)
	}
	if value.RequestVersionID == uuid.Nil {
		return nil, nil
	}
	return &value, nil
}

func supersedeCandidateVersion(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation, previous activeCandidateVersion) error {
	insert, err := newCandidateInsert(ctx, tx, observation)
	if err != nil {
		return err
	}
	return persistSupersedingCandidate(ctx, tx, supersedingCandidate{observation: observation, previous: previous, insert: insert})
}

type supersedingCandidate struct {
	observation deploydomain.CandidateObservation
	previous    activeCandidateVersion
	insert      candidateInsert
}

func persistSupersedingCandidate(ctx context.Context, tx *gorm.DB, value supersedingCandidate) error {
	value.insert = configureSupersedingInsert(value)
	if err := replaceCandidateVersion(ctx, tx, value); err != nil {
		return err
	}
	return finishSupersedingCandidate(ctx, tx, value)
}

func configureSupersedingInsert(value supersedingCandidate) candidateInsert {
	value.insert.requestID = value.previous.RequestID
	value.insert.createRequest = false
	value.insert.versionNumber = value.previous.VersionNumber + 1
	value.insert.metadata = value.previous.metadata()
	return value.insert
}

func replaceCandidateVersion(ctx context.Context, tx *gorm.DB, value supersedingCandidate) error {
	if err := markCandidateVersionSuperseded(ctx, tx, value.previous.RequestVersionID, value.observation.ObservedAt); err != nil {
		return err
	}
	if err := persistCandidateInsert(ctx, tx, value.insert); err != nil {
		return err
	}
	if err := cloneCandidateApplications(ctx, tx, candidateApplicationClone{
		fromVersionID: value.previous.RequestVersionID, toVersionID: value.insert.requestVersionID,
		excludedApplicationID: value.observation.ApplicationID, createdAt: value.observation.ObservedAt,
	}); err != nil {
		return err
	}
	return nil
}

func finishSupersedingCandidate(ctx context.Context, tx *gorm.DB, value supersedingCandidate) error {
	classification, err := classifyCandidateRequest(candidateClassification{
		ctx: ctx, tx: tx, requestID: value.previous.RequestID,
		requestVersionID: value.insert.requestVersionID, observedAt: value.observation.ObservedAt,
	})
	if err != nil {
		return err
	}
	return appendDeploymentRequestCreated(ctx, tx, deploymentRequestEvent{
		observation: value.observation, requestID: value.insert.requestID,
		requestVersionID: value.insert.requestVersionID, classification: classification,
	})
}

type candidateApplicationClone struct {
	fromVersionID         uuid.UUID
	toVersionID           uuid.UUID
	excludedApplicationID uuid.UUID
	createdAt             time.Time
}

type candidateImageClone struct {
	previousApplicationID uuid.UUID
	applicationID         uuid.UUID
	createdAt             time.Time
}

func candidateExecutionOrder(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) (int, error) {
	var order *int
	err := tx.WithContext(ctx).Raw(`
SELECT (node ->> 'order')::integer
FROM deployment_plan_versions version,
     jsonb_array_elements(version.document -> 'nodes') node
WHERE version.id = ? AND node ->> 'applicationKey' = ?
LIMIT 1`, observation.PlanVersionID, observation.ApplicationKey).Scan(&order).Error
	if err != nil {
		return 0, fmt.Errorf("resolve candidate deployment Plan node: %w", err)
	}
	if order == nil {
		return 0, errors.New("candidate Application is not defined by deployment Plan")
	}
	return *order, nil
}

func cloneCandidateApplications(ctx context.Context, tx *gorm.DB, value candidateApplicationClone) error {
	var applications []deploymentRequestApplicationModel
	err := tx.WithContext(ctx).Where("request_version_id = ? AND application_id <> ?",
		value.fromVersionID, value.excludedApplicationID).Find(&applications).Error
	if err != nil {
		return fmt.Errorf("load accumulated deployment request Applications: %w", err)
	}
	for _, application := range applications {
		if err := cloneCandidateApplication(ctx, tx, value, application); err != nil {
			return err
		}
	}
	return nil
}

func cloneCandidateApplication(ctx context.Context, tx *gorm.DB, value candidateApplicationClone, application deploymentRequestApplicationModel) error {
	previousID := application.ID
	application.ID, application.RequestVersionID = uuid.New(), value.toVersionID
	application.CreatedAt = value.createdAt.UTC()
	if err := tx.WithContext(ctx).Create(&application).Error; err != nil {
		return fmt.Errorf("clone accumulated deployment request Application: %w", err)
	}
	return cloneCandidateImages(ctx, tx, candidateImageClone{
		previousApplicationID: previousID, applicationID: application.ID, createdAt: value.createdAt,
	})
}

func cloneCandidateImages(ctx context.Context, tx *gorm.DB, value candidateImageClone) error {
	var images []deploymentRequestImageModel
	if err := tx.WithContext(ctx).Where("request_application_id = ?", value.previousApplicationID).Find(&images).Error; err != nil {
		return fmt.Errorf("load accumulated deployment request images: %w", err)
	}
	for _, image := range images {
		image.ID, image.RequestApplicationID, image.CreatedAt = uuid.New(), value.applicationID, value.createdAt.UTC()
		if err := tx.WithContext(ctx).Create(&image).Error; err != nil {
			return fmt.Errorf("clone accumulated deployment request image: %w", err)
		}
	}
	return nil
}

func (v activeCandidateVersion) metadata() candidateVersionMetadata {
	return candidateVersionMetadata{
		ChangeDescription: v.ChangeDescription, IssueURL: v.IssueURL,
		ScheduledFor: v.ScheduledFor, CreatedBy: v.CreatedBy,
	}
}

func markCandidateVersionSuperseded(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) error {
	result := tx.WithContext(ctx).Model(&deploymentRequestVersionModel{}).
		Where("id = ? AND status NOT IN ?", versionID, []string{"Succeeded", "Failed", "Superseded", "Terminated", "Deploying"}).
		Updates(map[string]any{"status": "Superseded", "updated_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("supersede deployment request version: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("deployment request version cannot be superseded")
	}
	return supersedeWorkflowRuntime(ctx, tx, versionID, now)
}

func supersedeWorkflowRuntime(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) error {
	if err := tx.WithContext(ctx).Model(&workflowInstanceModel{}).
		Where("request_version_id = ?", versionID).
		Updates(map[string]any{"status": "Superseded", "completed_at": now.UTC(), "updated_at": now.UTC()}).Error; err != nil {
		return fmt.Errorf("supersede deployment workflow instance: %w", err)
	}
	if err := tx.WithContext(ctx).Model(&reviewTaskModel{}).
		Where("request_version_id = ? AND status IN ?", versionID, []string{"Pending", "ReassignmentRequired"}).
		Updates(map[string]any{"status": "Closed", "closed_at": now.UTC()}).Error; err != nil {
		return fmt.Errorf("close superseded deployment reviews: %w", err)
	}
	return nil
}

func touchDeploymentRequest(ctx context.Context, tx *gorm.DB, requestID uuid.UUID, now time.Time) error {
	if err := tx.WithContext(ctx).Model(&deploymentRequestModel{}).
		Where("id = ?", requestID).Update("updated_at", now.UTC()).Error; err != nil {
		return fmt.Errorf("touch deployment request: %w", err)
	}
	return nil
}

func candidateVersionFingerprint(insert candidateInsert) string {
	if insert.versionNumber == 1 {
		return insert.observation.Fingerprint
	}
	sum := sha256.Sum256([]byte(insert.observation.Fingerprint + ":" + insert.requestID.String()))
	return hex.EncodeToString(sum[:])
}
