package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	ecrapp "github.com/vincent119/ReleaseHub/Server/internal/ecr/application"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// DigestRepository persists immutable ECR digest resolution records for managed production Applications.
type DigestRepository struct{ db *gorm.DB }

var _ ecrapp.DigestRepository = (*DigestRepository)(nil)

// NewDigestRepository creates the PostgreSQL adapter for ECR digest resolution.
func NewDigestRepository(db *gorm.DB) (*DigestRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DigestRepository{db: db}, nil
}

// LoadDigestTarget reads only a managed production Application and its Argo CD identity.
func (r *DigestRepository) LoadDigestTarget(ctx context.Context, applicationID uuid.UUID) (ecrapp.DigestTarget, error) {
	var model digestTargetModel
	query := r.db.WithContext(ctx).Raw(`
SELECT application.id AS application_id,
       application.organization_id,
       application.project_id,
       application.environment_id,
       application.argocd_namespace,
       application.argocd_application_name,
       application.argocd_project
FROM applications application
JOIN environments environment
  ON environment.id = application.environment_id
 AND environment.organization_id = application.organization_id
 AND environment.project_id = application.project_id
JOIN application_onboardings onboarding
  ON onboarding.application_id = application.id
WHERE application.id = ?
  AND application.active
  AND environment.active
  AND environment.environment_type = 'Production'
  AND onboarding.status = 'Managed'`, applicationID).Scan(&model)
	if query.Error != nil {
		return ecrapp.DigestTarget{}, fmt.Errorf("load digest target: %w", query.Error)
	}
	if query.RowsAffected != 1 {
		return ecrapp.DigestTarget{}, gorm.ErrRecordNotFound
	}
	return model.domain(), nil
}

// AppendDigestSnapshots writes all outcomes, audit data, and one outbox event atomically.
func (r *DigestRepository) AppendDigestSnapshots(ctx context.Context, mutation ecrapp.DigestMutation, target ecrapp.DigestTarget, snapshots []ecrdomain.DigestSnapshot) error {
	if mutation.ActorID == uuid.Nil || target.ApplicationID == uuid.Nil || len(snapshots) == 0 {
		return errors.New("invalid digest snapshot persistence input")
	}
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		available, unavailable := 0, 0
		for _, snapshot := range snapshots {
			if snapshot.ApplicationID != target.ApplicationID {
				return errors.New("digest snapshot belongs to a different application")
			}
			if snapshot.Status == ecrdomain.DigestAvailable {
				available++
			} else {
				unavailable++
			}
			model := digestSnapshotModel{
				ApplicationID: snapshot.ApplicationID, ManifestRevision: snapshot.ManifestRevision, ImageReference: snapshot.ImageReference,
				Registry: snapshot.Registry, Repository: snapshot.Repository, ImageTag: snapshot.Tag,
				RequestedDigest: snapshot.RequestedDigest, ResolvedDigest: snapshot.ResolvedDigest,
				Status: string(snapshot.Status), ErrorCode: snapshot.ErrorCode, ResolvedAt: snapshot.ResolvedAt.UTC(),
			}
			if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
				return fmt.Errorf("insert image digest snapshot: %w", err)
			}
		}
		return appendDigestResolution(ctx, tx, mutation, target, snapshots[0].ManifestRevision, available, unavailable, snapshots[0].ResolvedAt)
	})
}

func appendDigestResolution(ctx context.Context, tx *gorm.DB, mutation ecrapp.DigestMutation, target ecrapp.DigestTarget, revision string, available, unavailable int, occurredAt time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"applicationId": target.ApplicationID.String(), "manifestRevision": revision,
		"availableCount": available, "unavailableCount": unavailable,
	})
	if err != nil {
		return fmt.Errorf("marshal image digest event: %w", err)
	}
	event, err := platform.NewEvent("ecr.image_digest_snapshots_recorded", "application", target.ApplicationID.String(), payload, occurredAt.UTC())
	if err != nil {
		return fmt.Errorf("create image digest event: %w", err)
	}
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: occurredAt.UTC(), ActorID: &mutation.ActorID, OrganizationID: &target.OrganizationID,
		Action: "application.image_digest.resolve", ResourceType: "application", ResourceID: target.ApplicationID.String(),
		RequestID: mutation.RequestID, Metadata: map[string]any{"manifestRevision": revision, "availableCount": available, "unavailableCount": unavailable},
	}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

type digestTargetModel struct {
	ApplicationID         uuid.UUID
	OrganizationID        uuid.UUID
	ProjectID             uuid.UUID
	EnvironmentID         uuid.UUID
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
	ArgoCDProject         string `gorm:"column:argocd_project"`
}

func (m digestTargetModel) domain() ecrapp.DigestTarget {
	return ecrapp.DigestTarget{
		ApplicationID: m.ApplicationID, OrganizationID: m.OrganizationID, ProjectID: m.ProjectID, EnvironmentID: m.EnvironmentID,
		Argo: argodomain.ApplicationIdentity{Namespace: m.ArgoCDNamespace, Name: m.ArgoCDApplicationName}, ArgoProject: m.ArgoCDProject,
	}
}

type digestSnapshotModel struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ApplicationID    uuid.UUID `gorm:"type:uuid;not null"`
	ManifestRevision string
	ImageReference   string
	Registry         string
	Repository       string
	ImageTag         string `gorm:"column:image_tag"`
	RequestedDigest  string
	ResolvedDigest   string
	Status           string
	ErrorCode        string
	ResolvedAt       time.Time
}

func (digestSnapshotModel) TableName() string { return "application_image_digest_snapshots" }
