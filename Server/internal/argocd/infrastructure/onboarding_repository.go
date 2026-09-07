package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// OnboardingRepository persists the recoverable PostgreSQL side of Application onboarding.
type OnboardingRepository struct{ db *gorm.DB }

var _ argoapp.OnboardingRepository = (*OnboardingRepository)(nil)

// NewOnboardingRepository creates the Application onboarding adapter.
func NewOnboardingRepository(db *gorm.DB) (*OnboardingRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &OnboardingRepository{db: db}, nil
}

// LoadTarget reads the catalog mapping and exact discovery state without accepting repository topology as identity.
func (r *OnboardingRepository) LoadTarget(ctx context.Context, applicationID uuid.UUID) (argodomain.OnboardingTarget, error) {
	var model onboardingTargetModel
	query := r.db.WithContext(ctx).Raw(`
SELECT application.id AS application_id,
       application.organization_id,
       application.project_id,
       application.environment_id,
       environment.environment_type = 'Production' AS production,
       application.argocd_namespace,
       application.argocd_application_name,
       application.argocd_project,
       application.destination_server,
       application.destination_namespace,
       application.source_repo_url,
       application.source_target_revision,
       application.source_path,
       EXISTS (
           SELECT 1
           FROM argocd_application_candidates candidate
           WHERE candidate.argocd_namespace = application.argocd_namespace
             AND candidate.argocd_application_name = application.argocd_application_name
             AND candidate.discovery_status = 'present'
       ) AS candidate_present
FROM applications application
JOIN environments environment
  ON environment.id = application.environment_id
 AND environment.organization_id = application.organization_id
 AND environment.project_id = application.project_id
WHERE application.id = ?
  AND application.active
  AND environment.active`, applicationID).Scan(&model)
	if query.Error != nil {
		return argodomain.OnboardingTarget{}, fmt.Errorf("load onboarding target: %w", query.Error)
	}
	if query.RowsAffected != 1 {
		return argodomain.OnboardingTarget{}, gorm.ErrRecordNotFound
	}
	return model.domain(), nil
}

// SaveValidation persists a dry-run result without crossing the Argo CD mutation boundary.
func (r *OnboardingRepository) SaveValidation(ctx context.Context, mutation argoapp.OnboardingMutation, target argodomain.OnboardingTarget, issues []argodomain.ValidationIssueCode, resourceVersion string, now time.Time) (argodomain.Onboarding, error) {
	status := argodomain.OnboardingAwaitingConfirmation
	if len(issues) > 0 {
		status = argodomain.OnboardingValidationFailed
	}
	issuesJSON, err := json.Marshal(issues)
	if err != nil {
		return argodomain.Onboarding{}, fmt.Errorf("marshal onboarding validation issues: %w", err)
	}
	var result argodomain.Onboarding
	err = database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var model onboardingModel
		query := tx.WithContext(ctx).Raw(`
INSERT INTO application_onboardings (
    application_id, status, validation_issues, validated_resource_version, validated_at,
    drift_reasons, version, created_at, updated_at
) VALUES (?, ?, ?::jsonb, ?, ?, '[]'::jsonb, 1, ?, ?)
ON CONFLICT (application_id) DO UPDATE SET
    status = EXCLUDED.status,
    validation_issues = EXCLUDED.validation_issues,
    validated_resource_version = EXCLUDED.validated_resource_version,
    validated_at = EXCLUDED.validated_at,
    confirmed_by = NULL,
    confirmed_at = NULL,
    managed_at = NULL,
    drift_detected_at = NULL,
    drift_reasons = '[]'::jsonb,
    last_error_code = '',
    version = application_onboardings.version + 1,
    updated_at = EXCLUDED.updated_at
WHERE application_onboardings.status NOT IN ('Applying', 'Managed')
RETURNING *`, target.ApplicationID, status, string(issuesJSON), resourceVersion, now.UTC(), now.UTC(), now.UTC()).Scan(&model)
		if query.Error != nil {
			return fmt.Errorf("save onboarding validation: %w", query.Error)
		}
		if query.RowsAffected != 1 {
			return argoapp.ErrOnboardingConflict
		}
		if err := appendOnboardingChange(ctx, tx, &mutation.ActorID, mutation.RequestID, target.OrganizationID, target.ApplicationID, string(status), now); err != nil {
			return err
		}
		result = model.domain()
		return nil
	})
	return result, err
}

// BeginApplying records confirmation before the external automated-sync mutation.
func (r *OnboardingRepository) BeginApplying(ctx context.Context, mutation argoapp.OnboardingMutation, applicationID uuid.UUID, expectedVersion uint64, resourceVersion string, now time.Time) (argodomain.Onboarding, error) {
	return r.transition(ctx, transitionInput{
		ApplicationID: applicationID, ExpectedVersion: expectedVersion, FromStatus: argodomain.OnboardingAwaitingConfirmation,
		ToStatus: argodomain.OnboardingApplying, ActorID: &mutation.ActorID, RequestID: mutation.RequestID,
		ResourceVersion: resourceVersion, Now: now,
	})
}

// BeginApplyingIdempotent atomically reserves a confirmation command and persists the recoverable Applying state.
func (r *OnboardingRepository) BeginApplyingIdempotent(ctx context.Context, mutation argoapp.OnboardingMutation, applicationID uuid.UUID, expectedVersion uint64, resourceVersion string, command argoapp.OnboardingCommand, now time.Time) (argodomain.Onboarding, argoapp.OnboardingCommand, bool, error) {
	var onboarding argodomain.Onboarding
	var persisted argoapp.OnboardingCommand
	var replayed bool
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		model := (commandModel{}).fromDomain(command)
		result := tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "actor_id"}, {Name: "operation"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&model)
		if result.Error != nil {
			return fmt.Errorf("reserve onboarding command: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			model = commandModel{}
			if err := tx.WithContext(ctx).Where("actor_id = ? AND operation = ? AND idempotency_key = ?", command.ActorID, string(command.Operation), command.IdempotencyKey).Take(&model).Error; err != nil {
				return fmt.Errorf("load onboarding command: %w", err)
			}
			persisted = model.domain()
			if persisted.RequestFingerprint != command.RequestFingerprint || persisted.ApplicationID != applicationID {
				return argoapp.ErrIdempotencyKeyReuse
			}
			replayed = true
			return nil
		}
		organizationID, err := organizationForApplication(ctx, tx, applicationID)
		if err != nil {
			return err
		}
		var state onboardingModel
		query := tx.WithContext(ctx).Raw(`UPDATE application_onboardings SET status = 'Applying', version = version + 1, updated_at = ?, last_error_code = '', confirmed_by = ?, confirmed_at = ?, validated_resource_version = ? WHERE application_id = ? AND status = 'AwaitingConfirmation' AND version = ? RETURNING *`, now.UTC(), mutation.ActorID, now.UTC(), resourceVersion, applicationID, expectedVersion).Scan(&state)
		if query.Error != nil {
			return fmt.Errorf("begin idempotent onboarding: %w", query.Error)
		}
		if query.RowsAffected != 1 {
			return argoapp.ErrOnboardingConflict
		}
		if err := appendOnboardingChange(ctx, tx, &mutation.ActorID, mutation.RequestID, organizationID, applicationID, string(argodomain.OnboardingApplying), now); err != nil {
			return err
		}
		onboarding, persisted = state.domain(), command
		return nil
	})
	return onboarding, persisted, replayed, err
}

// MarkManaged finalizes onboarding only after a successful Argo CD readback.
func (r *OnboardingRepository) MarkManaged(ctx context.Context, actorID *uuid.UUID, requestID string, applicationID uuid.UUID, expectedVersion uint64, now time.Time) (argodomain.Onboarding, error) {
	return r.transition(ctx, transitionInput{
		ApplicationID: applicationID, ExpectedVersion: expectedVersion, FromStatus: argodomain.OnboardingApplying,
		ToStatus: argodomain.OnboardingManaged, ActorID: actorID, RequestID: requestID, Now: now,
	})
}

// RecordOperationError leaves the command recoverable and stores only a stable error code.
func (r *OnboardingRepository) RecordOperationError(ctx context.Context, actorID *uuid.UUID, requestID string, applicationID uuid.UUID, expectedVersion uint64, code string, now time.Time) (argodomain.Onboarding, error) {
	var result argodomain.Onboarding
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var model onboardingModel
		query := tx.WithContext(ctx).Raw(`
UPDATE application_onboardings
SET last_error_code = ?, version = version + 1, updated_at = ?
WHERE application_id = ? AND status = 'Applying' AND version = ?
RETURNING *`, code, now.UTC(), applicationID, expectedVersion).Scan(&model)
		if query.Error != nil {
			return fmt.Errorf("record onboarding operation error: %w", query.Error)
		}
		if query.RowsAffected != 1 {
			return argoapp.ErrOnboardingConflict
		}
		organizationID, err := organizationForApplication(ctx, tx, applicationID)
		if err != nil {
			return err
		}
		if err := appendOnboardingChange(ctx, tx, actorID, requestID, organizationID, applicationID, code, now); err != nil {
			return err
		}
		result = model.domain()
		return nil
	})
	return result, err
}

// ListApplying returns only explicitly confirmed operations eligible for restart recovery.
func (r *OnboardingRepository) ListApplying(ctx context.Context) ([]argoapp.ApplyingOnboarding, error) {
	var models []onboardingModel
	if err := r.db.WithContext(ctx).
		Where("status = ?", argodomain.OnboardingApplying).
		Order("updated_at, application_id").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list applying onboarding states: %w", err)
	}
	result := make([]argoapp.ApplyingOnboarding, 0, len(models))
	for _, model := range models {
		target, err := r.LoadTarget(ctx, model.ApplicationID)
		if err != nil {
			return nil, fmt.Errorf("load applying onboarding target: %w", err)
		}
		result = append(result, argoapp.ApplyingOnboarding{Target: target, Onboarding: model.domain()})
	}
	return result, nil
}

func (r *OnboardingRepository) transition(ctx context.Context, input transitionInput) (argodomain.Onboarding, error) {
	var result argodomain.Onboarding
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		organizationID, err := organizationForApplication(ctx, tx, input.ApplicationID)
		if err != nil {
			return err
		}
		assignments := map[string]any{
			"status": input.ToStatus, "version": gorm.Expr("version + 1"), "updated_at": input.Now.UTC(), "last_error_code": "",
		}
		if input.ToStatus == argodomain.OnboardingApplying {
			assignments["confirmed_by"] = input.ActorID
			assignments["confirmed_at"] = input.Now.UTC()
			assignments["validated_resource_version"] = input.ResourceVersion
		}
		if input.ToStatus == argodomain.OnboardingManaged {
			assignments["managed_at"] = input.Now.UTC()
			assignments["drift_detected_at"] = nil
			assignments["drift_reasons"] = datatypes.JSON([]byte("[]"))
		}
		query := tx.WithContext(ctx).Model(&onboardingModel{}).
			Where("application_id = ? AND status = ? AND version = ?", input.ApplicationID, input.FromStatus, input.ExpectedVersion).
			Updates(assignments)
		if query.Error != nil {
			return fmt.Errorf("transition onboarding state: %w", query.Error)
		}
		if query.RowsAffected != 1 {
			return argoapp.ErrOnboardingConflict
		}
		var model onboardingModel
		if err := tx.WithContext(ctx).Where("application_id = ?", input.ApplicationID).Take(&model).Error; err != nil {
			return fmt.Errorf("read transitioned onboarding state: %w", err)
		}
		if err := appendOnboardingChange(ctx, tx, input.ActorID, input.RequestID, organizationID, input.ApplicationID, string(input.ToStatus), input.Now); err != nil {
			return err
		}
		if input.ToStatus == argodomain.OnboardingManaged {
			if err := tx.WithContext(ctx).Model(&commandModel{}).Where("application_id = ? AND operation = ? AND state = ?", input.ApplicationID, argoapp.CommandConfirm, argoapp.CommandAccepted).Updates(map[string]any{"state": argoapp.CommandCompleted, "onboarding_status": string(argodomain.OnboardingManaged), "onboarding_version": model.Version, "response_code": "managed", "completed_at": input.Now.UTC(), "updated_at": input.Now.UTC()}).Error; err != nil {
				return fmt.Errorf("complete onboarding command: %w", err)
			}
		}
		result = model.domain()
		return nil
	})
	return result, err
}

func organizationForApplication(ctx context.Context, tx *gorm.DB, applicationID uuid.UUID) (uuid.UUID, error) {
	var result struct {
		OrganizationID uuid.UUID
	}
	query := tx.WithContext(ctx).Table("applications").Select("organization_id").Where("id = ?", applicationID).Scan(&result)
	if query.Error != nil {
		return uuid.Nil, fmt.Errorf("load Application organization: %w", query.Error)
	}
	if query.RowsAffected != 1 || result.OrganizationID == uuid.Nil {
		return uuid.Nil, gorm.ErrRecordNotFound
	}
	return result.OrganizationID, nil
}

func appendOnboardingChange(ctx context.Context, tx *gorm.DB, actorID *uuid.UUID, requestID string, organizationID, applicationID uuid.UUID, state string, occurredAt time.Time) error {
	payload, err := json.Marshal(map[string]string{"applicationId": applicationID.String(), "state": state})
	if err != nil {
		return fmt.Errorf("marshal onboarding event: %w", err)
	}
	event, err := platform.NewEvent("argocd.application.onboarding_state_changed", "application_onboarding", applicationID.String(), payload, occurredAt)
	if err != nil {
		return fmt.Errorf("create onboarding event: %w", err)
	}
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: occurredAt, ActorID: actorID, OrganizationID: &organizationID,
		Action: "application_onboarding.state_change", ResourceType: "application_onboarding",
		ResourceID: applicationID.String(), RequestID: requestID, Metadata: map[string]any{"state": state},
	}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

type transitionInput struct {
	ApplicationID   uuid.UUID
	ExpectedVersion uint64
	FromStatus      argodomain.OnboardingStatus
	ToStatus        argodomain.OnboardingStatus
	ActorID         *uuid.UUID
	RequestID       string
	ResourceVersion string
	Now             time.Time
}

type onboardingTargetModel struct {
	ApplicationID         uuid.UUID
	OrganizationID        uuid.UUID
	ProjectID             uuid.UUID
	EnvironmentID         uuid.UUID
	Production            bool
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
	ArgoCDProject         string `gorm:"column:argocd_project"`
	DestinationServer     string
	DestinationNamespace  string
	SourceRepositoryURL   string `gorm:"column:source_repo_url"`
	SourceTargetRevision  string
	SourcePath            string
	CandidatePresent      bool
}

func (m onboardingTargetModel) domain() argodomain.OnboardingTarget {
	return argodomain.OnboardingTarget{
		ApplicationID: m.ApplicationID, OrganizationID: m.OrganizationID, ProjectID: m.ProjectID, EnvironmentID: m.EnvironmentID,
		Production: m.Production, Argo: argodomain.ApplicationIdentity{Namespace: m.ArgoCDNamespace, Name: m.ArgoCDApplicationName},
		ArgoProject: m.ArgoCDProject, DestinationServer: m.DestinationServer, DestinationNamespace: m.DestinationNamespace,
		Source:           argodomain.CatalogSource{RepositoryURL: m.SourceRepositoryURL, TargetRevision: m.SourceTargetRevision, Path: m.SourcePath},
		CandidatePresent: m.CandidatePresent,
	}
}

type onboardingModel struct {
	ApplicationID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Status                   string
	ValidationIssues         datatypes.JSON `gorm:"type:jsonb"`
	ValidatedResourceVersion string
	ValidatedAt              *time.Time
	ConfirmedBy              *uuid.UUID `gorm:"type:uuid"`
	ConfirmedAt              *time.Time
	ManagedAt                *time.Time
	DriftDetectedAt          *time.Time
	DriftReasons             datatypes.JSON `gorm:"type:jsonb"`
	LastErrorCode            string
	Version                  uint64
}

func (onboardingModel) TableName() string { return "application_onboardings" }

func (m onboardingModel) domain() argodomain.Onboarding {
	var issues []argodomain.ValidationIssueCode
	var reasons []argodomain.DriftReason
	_ = json.Unmarshal(m.ValidationIssues, &issues)
	_ = json.Unmarshal(m.DriftReasons, &reasons)
	return argodomain.Onboarding{
		ApplicationID: m.ApplicationID, Status: argodomain.OnboardingStatus(m.Status), ValidationIssues: issues,
		ValidatedResourceVersion: m.ValidatedResourceVersion, ValidatedAt: m.ValidatedAt, ConfirmedBy: m.ConfirmedBy,
		ConfirmedAt: m.ConfirmedAt, ManagedAt: m.ManagedAt, DriftDetectedAt: m.DriftDetectedAt,
		DriftReasons: reasons, LastErrorCode: m.LastErrorCode, Version: m.Version,
	}
}
