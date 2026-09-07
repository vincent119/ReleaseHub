package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// DeploymentRequestRepository persists request projections and immutable metadata versions.
type DeploymentRequestRepository struct{ db *gorm.DB }

var _ deployapp.DeploymentRequestRepository = (*DeploymentRequestRepository)(nil)

// NewDeploymentRequestRepository creates the PostgreSQL request repository.
func NewDeploymentRequestRepository(db *gorm.DB) (*DeploymentRequestRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentRequestRepository{db: db}, nil
}

// List returns current request summaries in one persisted Environment scope.
func (r *DeploymentRequestRepository) List(ctx context.Context, scope authz.Scope) ([]deploydomain.DeploymentRequestSummary, error) {
	var models []requestSummaryModel
	err := r.db.WithContext(ctx).Raw(requestSummaryQuery, scope.OrganizationID, scope.ProjectID, scope.EnvironmentID).Scan(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list deployment requests: %w", err)
	}
	return requestSummaries(models), nil
}

// Load returns the active immutable version for a request.
func (r *DeploymentRequestRepository) Load(ctx context.Context, requestID uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	return loadDeploymentRequest(ctx, r.db, requestID)
}

// SupersedeMetadata atomically preserves the old version and appends a replacement version.
func (r *DeploymentRequestRepository) SupersedeMetadata(ctx context.Context, mutation deployapp.RequestMutation, change deployapp.MetadataVersionChange) (deploydomain.DeploymentRequestDetail, error) {
	var requestID uuid.UUID
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		detail, err := loadDeploymentRequest(ctx, tx, change.RequestID)
		if err != nil {
			return err
		}
		if err := validateMetadataSupersede(detail, change); err != nil {
			return err
		}
		_, err = appendMetadataVersion(ctx, tx, metadataAppend{mutation: mutation, detail: detail, metadata: change.Metadata})
		requestID = detail.Summary.ID
		return err
	})
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	return r.Load(ctx, requestID)
}

func validateMetadataSupersede(detail deploydomain.DeploymentRequestDetail, change deployapp.MetadataVersionChange) error {
	if detail.Version.ID != change.RequestVersion || detail.Version.LockVersion != change.ExpectedVersion {
		return deployapp.ErrRequestConflict
	}
	if !detail.Version.CanBeSuperseded() {
		return deployapp.ErrRequestConflict
	}
	return nil
}

type metadataAppend struct {
	mutation deployapp.RequestMutation
	detail   deploydomain.DeploymentRequestDetail
	metadata deploydomain.DeploymentRequestMetadata
}

func appendMetadataVersion(ctx context.Context, tx *gorm.DB, value metadataAppend) (uuid.UUID, error) {
	now := value.mutation.OccurredAt.UTC()
	if err := markCandidateVersionSuperseded(ctx, tx, value.detail.Version.ID, now); err != nil {
		return uuid.Nil, requestWriteError(err)
	}
	versionID := uuid.New()
	build := metadataVersionBuild{id: versionID, prior: value.detail.Version, metadata: value.metadata, actorID: value.mutation.ActorID, now: now}
	if err := persistMetadataVersion(ctx, tx, build); err != nil {
		return uuid.Nil, requestWriteError(err)
	}
	if err := cloneRequestApplications(ctx, tx, applicationClone{values: value.detail.Applications, versionID: versionID, now: now}); err != nil {
		return uuid.Nil, err
	}
	if err := finishMetadataVersion(ctx, tx, metadataFinish{value: value, versionID: versionID, now: now}); err != nil {
		return uuid.Nil, err
	}
	return versionID, nil
}

type metadataFinish struct {
	value     metadataAppend
	versionID uuid.UUID
	now       time.Time
}

func finishMetadataVersion(ctx context.Context, tx *gorm.DB, finish metadataFinish) error {
	if err := touchDeploymentRequest(ctx, tx, finish.value.detail.Summary.ID, finish.now); err != nil {
		return err
	}
	return appendMetadataEffects(ctx, tx, metadataEffects{mutation: finish.value.mutation, detail: finish.value.detail, versionID: finish.versionID})
}

func persistMetadataVersion(ctx context.Context, tx *gorm.DB, value metadataVersionBuild) error {
	version := metadataVersionModel(value)
	return tx.WithContext(ctx).Create(&version).Error
}

type metadataVersionBuild struct {
	id       uuid.UUID
	prior    deploydomain.DeploymentRequestVersionSummary
	metadata deploydomain.DeploymentRequestMetadata
	actorID  uuid.UUID
	now      time.Time
}

func metadataVersionModel(value metadataVersionBuild) deploymentRequestVersionModel {
	return deploymentRequestVersionModel{
		ID: value.id, RequestID: value.prior.RequestID, VersionNumber: int(value.prior.VersionNumber + 1),
		Status: string(deploydomain.DeploymentRequestCandidate), Fingerprint: metadataFingerprint(value.prior.Fingerprint, value.metadata),
		WorkflowVersionID: value.prior.WorkflowVersionID, PlanVersionID: value.prior.PlanVersionID, Title: value.prior.Title,
		ChangeDescription: value.metadata.ChangeDescription, IssueURL: value.metadata.IssueURL, ScheduledFor: value.metadata.ScheduledFor,
		LockVersion: 1, CreatedBy: &value.actorID, SourceSnapshot: value.prior.SourceSnapshot, CreatedAt: value.now, UpdatedAt: value.now,
	}
}

func metadataFingerprint(prior string, metadata deploydomain.DeploymentRequestMetadata) string {
	payload, _ := json.Marshal(struct {
		Fingerprint string
		Metadata    deploydomain.DeploymentRequestMetadata
	}{prior, metadata})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

type applicationClone struct {
	values    []deploydomain.DeploymentRequestApplicationSnapshot
	versionID uuid.UUID
	now       time.Time
}

func cloneRequestApplications(ctx context.Context, tx *gorm.DB, clone applicationClone) error {
	for _, value := range clone.values {
		applicationID := uuid.New()
		model := deploymentRequestApplicationModel{ID: applicationID, RequestVersionID: clone.versionID,
			ApplicationID: value.ApplicationID, ApplicationKey: value.ApplicationKey, LiveRevision: value.LiveRevision,
			TargetRevision: value.TargetRevision, TargetRevisions: marshalStringSlice(value.TargetRevisions),
			ManifestHash: value.ManifestHash, DiffHash: value.DiffHash, DiffSnapshot: value.DiffSnapshot,
			SourceSnapshot: value.SourceSnapshot, ExecutionOrder: value.Order, CreatedAt: clone.now}
		if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
			return requestWriteError(err)
		}
		if err := cloneRequestImages(ctx, tx, imageClone{applicationID: applicationID, values: value.Images, now: clone.now}); err != nil {
			return err
		}
	}
	return nil
}

type imageClone struct {
	applicationID uuid.UUID
	values        []deploydomain.DeploymentRequestImageSnapshot
	now           time.Time
}

func cloneRequestImages(ctx context.Context, tx *gorm.DB, clone imageClone) error {
	for _, value := range clone.values {
		model := deploymentRequestImageModel{ID: uuid.New(), RequestApplicationID: clone.applicationID,
			ImageReference: value.ImageReference, Registry: value.Registry, Repository: value.Repository,
			ImageTag: value.Tag, ImageDigest: value.Digest, CreatedAt: clone.now}
		if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
			return requestWriteError(err)
		}
	}
	return nil
}

type metadataEffects struct {
	mutation  deployapp.RequestMutation
	detail    deploydomain.DeploymentRequestDetail
	versionID uuid.UUID
}

func appendMetadataEffects(ctx context.Context, tx *gorm.DB, value metadataEffects) error {
	payload, err := json.Marshal(map[string]any{"requestId": value.detail.Summary.ID, "requestVersionId": value.versionID, "supersededVersionId": value.detail.Version.ID})
	if err != nil {
		return fmt.Errorf("marshal deployment request metadata event: %w", err)
	}
	event, err := platform.NewEvent("deployment.request.version.created", "deployment_request", value.detail.Summary.ID.String(), payload, value.mutation.OccurredAt)
	if err != nil {
		return fmt.Errorf("create deployment request metadata event: %w", err)
	}
	actorID := value.mutation.ActorID
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{OccurredAt: value.mutation.OccurredAt, ActorID: &actorID,
		Action: "deployment_request.version.supersede", ResourceType: "deployment_request_version", ResourceID: value.versionID.String(), RequestID: value.mutation.RequestID}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func requestWriteError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.ErrRequestNotFound
	}
	return fmt.Errorf("write deployment request: %w", err)
}

func marshalStringSlice(values []string) datatypes.JSON {
	encoded, _ := json.Marshal(values)
	return encoded
}

const requestSummaryQuery = `
SELECT request.id, request.organization_id, request.project_id, request.environment_id,
       request.classification, request.status, request.updated_at, version.version_number,
       version.status AS version_status, version.title, COUNT(application.id) AS application_count
FROM deployment_requests request
JOIN LATERAL (
  SELECT * FROM deployment_request_versions
  WHERE request_id = request.id ORDER BY version_number DESC LIMIT 1
) version ON true
LEFT JOIN deployment_request_applications application ON application.request_version_id = version.id
WHERE request.organization_id = ? AND request.project_id = ? AND request.environment_id = ?
GROUP BY request.id, version.id
ORDER BY request.updated_at DESC`

type requestSummaryModel struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	ProjectID        uuid.UUID
	EnvironmentID    uuid.UUID
	Classification   string
	Status           string
	UpdatedAt        time.Time
	VersionNumber    uint64
	VersionStatus    string
	Title            string
	ApplicationCount int
}

func requestSummaries(values []requestSummaryModel) []deploydomain.DeploymentRequestSummary {
	return slices.Collect(func(yield func(deploydomain.DeploymentRequestSummary) bool) {
		for _, value := range values {
			if !yield(requestSummary(value)) {
				return
			}
		}
	})
}

func requestSummary(value requestSummaryModel) deploydomain.DeploymentRequestSummary {
	return deploydomain.DeploymentRequestSummary{ID: value.ID, OrganizationID: value.OrganizationID,
		ProjectID: value.ProjectID, EnvironmentID: value.EnvironmentID, Classification: value.Classification,
		Status: deploydomain.DeploymentRequestStatus(value.VersionStatus), Title: value.Title,
		ActiveVersionNumber: value.VersionNumber, ApplicationCount: value.ApplicationCount, UpdatedAt: value.UpdatedAt}
}
