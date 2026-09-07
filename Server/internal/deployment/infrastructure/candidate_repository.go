// Package infrastructure implements deployment persistence and external adapters.
package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
)

// CandidateRepository persists automatic candidate observations and Requests atomically.
type CandidateRepository struct{ db *gorm.DB }

type candidatePersistenceEvidence struct {
	sources         datatypes.JSON
	diffs           datatypes.JSON
	images          datatypes.JSON
	targetRevisions datatypes.JSON
	requestSource   datatypes.JSON
}

type candidateInsert struct {
	observation          deploydomain.CandidateObservation
	requestID            uuid.UUID
	requestVersionID     uuid.UUID
	requestApplicationID uuid.UUID
	createRequest        bool
	versionNumber        int
	executionOrder       int
	metadata             candidateVersionMetadata
	evidence             candidatePersistenceEvidence
}

const candidateTargetsQuery = `
SELECT application.id AS application_id,
       application.organization_id,
       application.project_id,
       application.environment_id,
       application.name AS application_key,
       application.argocd_namespace,
       application.argocd_application_name,
       application.argocd_project,
       binding.workflow_version_id,
       binding.plan_version_id
FROM applications application
JOIN environments environment
  ON environment.id = application.environment_id
 AND environment.project_id = application.project_id
 AND environment.organization_id = application.organization_id
JOIN application_onboardings onboarding ON onboarding.application_id = application.id
JOIN deployment_bindings binding
  ON binding.environment_id = application.environment_id
 AND binding.project_id = application.project_id
 AND binding.organization_id = application.organization_id
JOIN release_workflow_versions workflow_version ON workflow_version.id = binding.workflow_version_id
JOIN deployment_plan_versions plan_version ON plan_version.id = binding.plan_version_id
WHERE application.active
  AND environment.active
  AND environment.environment_type = 'Production'
  AND onboarding.status = 'Managed'
  AND binding.active
  AND workflow_version.lifecycle = 'Published'
  AND plan_version.lifecycle = 'Published'
ORDER BY application.id`

var _ deployapp.CandidateRepository = (*CandidateRepository)(nil)

// NewCandidateRepository creates the PostgreSQL candidate repository.
func NewCandidateRepository(db *gorm.DB) (*CandidateRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &CandidateRepository{db: db}, nil
}

// ListCandidateTargets returns active managed production Applications with one active published binding.
func (r *CandidateRepository) ListCandidateTargets(ctx context.Context) ([]deployapp.CandidateTarget, error) {
	var models []candidateTargetModel
	err := r.db.WithContext(ctx).Raw(candidateTargetsQuery).Scan(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list deployment candidate targets: %w", err)
	}
	result := make([]deployapp.CandidateTarget, 0, len(models))
	for _, model := range models {
		result = append(result, model.domain())
	}
	return result, nil
}

// CreateDeploymentRequest accumulates one Application Candidate into its bound Request group.
func (r *CandidateRepository) CreateDeploymentRequest(ctx context.Context, observation deploydomain.CandidateObservation) (bool, error) {
	if observation.ID == uuid.Nil || observation.Fingerprint == "" {
		return false, errors.New("invalid candidate observation")
	}
	created := false
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var transactionErr error
		created, transactionErr = createDeploymentRequestIfNew(ctx, tx, observation)
		return transactionErr
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

func createDeploymentRequestIfNew(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) (bool, error) {
	if err := lockCandidateGroup(ctx, tx, observation); err != nil {
		return false, err
	}
	exists, err := candidateExists(ctx, tx, observation)
	if err != nil || exists {
		return false, err
	}
	return true, upsertCandidateRequest(ctx, tx, observation)
}
func lockCandidateGroup(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) error {
	key := fmt.Sprintf("%s:%s:%s:%s:%s", observation.OrganizationID, observation.ProjectID,
		observation.EnvironmentID, observation.WorkflowVersionID, observation.PlanVersionID)
	err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, key).Error
	if err != nil {
		return fmt.Errorf("lock deployment candidate group: %w", err)
	}
	return nil
}
func candidateExists(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Table("deployment_candidate_observations").
		Where("application_id = ? AND fingerprint = ?", observation.ApplicationID, observation.Fingerprint).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check deployment candidate fingerprint: %w", err)
	}
	return count > 0, nil
}
func insertInitialDeploymentRequest(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) error {
	insert, err := newCandidateInsert(ctx, tx, observation)
	if err != nil {
		return err
	}
	if err := persistCandidateInsert(ctx, tx, insert); err != nil {
		return err
	}
	classification, err := classifyCandidateRequest(candidateClassification{
		ctx: ctx, tx: tx, requestID: insert.requestID,
		requestVersionID: insert.requestVersionID, observedAt: observation.ObservedAt,
	})
	if err != nil {
		return err
	}
	return appendDeploymentRequestCreated(ctx, tx, deploymentRequestEvent{
		observation: observation, requestID: insert.requestID,
		requestVersionID: insert.requestVersionID, classification: classification,
	})
}
func newCandidateInsert(ctx context.Context, tx *gorm.DB, observation deploydomain.CandidateObservation) (candidateInsert, error) {
	evidence, err := newCandidatePersistenceEvidence(observation)
	if err != nil {
		return candidateInsert{}, err
	}
	executionOrder, err := candidateExecutionOrder(ctx, tx, observation)
	if err != nil {
		return candidateInsert{}, err
	}
	return candidateInsert{
		observation: observation, requestID: uuid.New(), requestVersionID: uuid.New(), requestApplicationID: uuid.New(),
		createRequest: true, versionNumber: 1, executionOrder: executionOrder,
		evidence: evidence,
	}, nil
}

func newCandidatePersistenceEvidence(observation deploydomain.CandidateObservation) (candidatePersistenceEvidence, error) {
	sources, diffs, images, targetRevisions, err := marshalCandidateEvidence(observation)
	if err != nil {
		return candidatePersistenceEvidence{}, err
	}
	requestSource, err := marshalCandidateRequestSource(observation)
	return candidatePersistenceEvidence{
		sources: sources, diffs: diffs, images: images,
		targetRevisions: targetRevisions, requestSource: requestSource,
	}, err
}

func marshalCandidateRequestSource(observation deploydomain.CandidateObservation) (datatypes.JSON, error) {
	value, err := json.Marshal(map[string]any{
		"applicationId": observation.ApplicationID, "targetRevision": observation.TargetRevision, "sources": observation.Sources,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal deployment request source snapshot: %w", err)
	}
	return value, nil
}
func persistCandidateInsert(ctx context.Context, tx *gorm.DB, insert candidateInsert) error {
	models := []any{candidateVersionModel(insert), candidateApplicationModel(insert)}
	if insert.createRequest {
		models = append([]any{candidateRequestModel(insert)}, models...)
	}
	for _, model := range models {
		if err := tx.WithContext(ctx).Create(model).Error; err != nil {
			return fmt.Errorf("create deployment request snapshot: %w", err)
		}
	}
	if err := persistCandidateImages(ctx, tx, insert); err != nil {
		return err
	}
	model := candidateObservationPersistenceModel(insert)
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create deployment candidate observation: %w", err)
	}
	return nil
}
func candidateRequestModel(insert candidateInsert) *deploymentRequestModel {
	value := insert.observation
	return &deploymentRequestModel{
		ID: insert.requestID, OrganizationID: value.OrganizationID, ProjectID: value.ProjectID,
		EnvironmentID: value.EnvironmentID, Classification: "Standard", Status: "Open",
		CreatedAt: value.ObservedAt, UpdatedAt: value.ObservedAt,
	}
}
func candidateVersionModel(insert candidateInsert) *deploymentRequestVersionModel {
	value := insert.observation
	return &deploymentRequestVersionModel{
		ID: insert.requestVersionID, RequestID: insert.requestID, VersionNumber: insert.versionNumber,
		Status: "Candidate", Fingerprint: candidateVersionFingerprint(insert),
		WorkflowVersionID: value.WorkflowVersionID, PlanVersionID: value.PlanVersionID, Title: value.ApplicationKey,
		ChangeDescription: insert.metadata.ChangeDescription, IssueURL: insert.metadata.IssueURL,
		ScheduledFor: insert.metadata.ScheduledFor, LockVersion: 1, CreatedBy: insert.metadata.CreatedBy,
		SourceSnapshot: insert.evidence.requestSource, CreatedAt: value.ObservedAt, UpdatedAt: value.ObservedAt,
	}
}
func candidateApplicationModel(insert candidateInsert) *deploymentRequestApplicationModel {
	value := insert.observation
	return &deploymentRequestApplicationModel{
		ID: insert.requestApplicationID, RequestVersionID: insert.requestVersionID, ApplicationID: value.ApplicationID,
		ApplicationKey: value.ApplicationKey, TargetRevision: value.TargetRevision,
		TargetRevisions: insert.evidence.targetRevisions, ManifestHash: value.ManifestHash, DiffHash: value.DiffHash,
		DiffSnapshot: insert.evidence.diffs, SourceSnapshot: insert.evidence.requestSource, CreatedAt: value.ObservedAt,
		ExecutionOrder: insert.executionOrder,
	}
}

func persistCandidateImages(ctx context.Context, tx *gorm.DB, insert candidateInsert) error {
	for _, image := range insert.observation.Images {
		model := deploymentRequestImageModel{
			RequestApplicationID: insert.requestApplicationID, ImageReference: image.ImageReference,
			Registry: image.Registry, Repository: image.Repository, ImageTag: image.Tag,
			ImageDigest: image.Digest, CreatedAt: insert.observation.ObservedAt,
		}
		if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
			return fmt.Errorf("create deployment request image snapshot: %w", err)
		}
	}
	return nil
}

func candidateObservationPersistenceModel(insert candidateInsert) candidateObservationModel {
	value := insert.observation
	return candidateObservationModel{
		ID: value.ID, ApplicationID: value.ApplicationID, RequestVersionID: insert.requestVersionID,
		WorkflowVersionID: value.WorkflowVersionID, PlanVersionID: value.PlanVersionID,
		TargetRevision: value.TargetRevision, TargetRevisions: insert.evidence.targetRevisions,
		ManifestHash: value.ManifestHash, DiffHash: value.DiffHash, DiffSnapshot: insert.evidence.diffs,
		SourceSnapshot: insert.evidence.sources, ImageSnapshot: insert.evidence.images,
		Fingerprint: value.Fingerprint, ObservedAt: value.ObservedAt, CreatedAt: value.ObservedAt,
	}
}

func marshalCandidateEvidence(observation deploydomain.CandidateObservation) (datatypes.JSON, datatypes.JSON, datatypes.JSON, datatypes.JSON, error) {
	sources, err := json.Marshal(observation.Sources)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal candidate sources: %w", err)
	}
	diffs, err := json.Marshal(observation.Diffs)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal candidate differences: %w", err)
	}
	images, err := json.Marshal(observation.Images)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal candidate images: %w", err)
	}
	targetRevisions, err := json.Marshal(observation.TargetRevisions)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal candidate target revisions: %w", err)
	}
	return sources, diffs, images, targetRevisions, nil
}
