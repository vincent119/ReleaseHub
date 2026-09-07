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

// ReconciliationRepository atomically persists candidate and tracked Application observations.
type ReconciliationRepository struct{ db *gorm.DB }

var _ argoapp.Store = (*ReconciliationRepository)(nil)

// NewReconciliationRepository creates the Argo CD observation store.
func NewReconciliationRepository(db *gorm.DB) (*ReconciliationRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &ReconciliationRepository{db: db}, nil
}

// Reconcile applies one complete list response as a single database transaction.
func (r *ReconciliationRepository) Reconcile(ctx context.Context, values []argodomain.Application, observedAt time.Time) (argoapp.Result, error) {
	observedAt = observedAt.UTC()
	token := uuid.New()
	result := argoapp.Result{ObservedApplications: len(values)}
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		existingCandidates, err := loadCandidateIdentities(ctx, tx)
		if err != nil {
			return err
		}
		tracked, err := loadTrackedApplications(ctx, tx)
		if err != nil {
			return err
		}

		for _, value := range values {
			identity := identityKey(value.Identity.Namespace, value.Identity.Name)
			if value.IsCandidate() {
				result.Candidates++
				candidate, err := newCandidateModel(value, observedAt, token)
				if err != nil {
					return err
				}
				if err := upsertCandidate(ctx, tx, candidate); err != nil {
					return err
				}
				if _, exists := existingCandidates[identity]; !exists {
					result.NewCandidates++
					if err := appendCandidateDiscovered(ctx, tx, value, observedAt); err != nil {
						return err
					}
				}
			}

			applicationID, exists := tracked[identity]
			if !exists {
				continue
			}
			result.TrackedApplications++
			snapshot, err := newSnapshotModel(applicationID, value, observedAt, token)
			if err != nil {
				return err
			}
			if err := upsertSnapshot(ctx, tx, snapshot); err != nil {
				return err
			}
		}

		if err := markMissingCandidates(ctx, tx, token, observedAt); err != nil {
			return err
		}
		if err := markMissingSnapshots(ctx, tx, token, observedAt); err != nil {
			return err
		}
		return detectManagedDrift(ctx, tx, observedAt)
	})
	if err != nil {
		return argoapp.Result{}, fmt.Errorf("persist Argo CD reconciliation: %w", err)
	}
	return result, nil
}

func loadCandidateIdentities(ctx context.Context, tx *gorm.DB) (map[string]struct{}, error) {
	var values []candidateIdentityModel
	if err := tx.WithContext(ctx).Table("argocd_application_candidates").Select("argocd_namespace, argocd_application_name").Find(&values).Error; err != nil {
		return nil, fmt.Errorf("list existing Argo CD candidates: %w", err)
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[identityKey(value.ArgoCDNamespace, value.ArgoCDApplicationName)] = struct{}{}
	}
	return result, nil
}

func loadTrackedApplications(ctx context.Context, tx *gorm.DB) (map[string]uuid.UUID, error) {
	var values []trackedApplicationModel
	if err := tx.WithContext(ctx).Table("applications").Select("id, argocd_namespace, argocd_application_name").Where("active").Find(&values).Error; err != nil {
		return nil, fmt.Errorf("list tracked Applications: %w", err)
	}
	result := make(map[string]uuid.UUID, len(values))
	for _, value := range values {
		result[identityKey(value.ArgoCDNamespace, value.ArgoCDApplicationName)] = value.ID
	}
	return result, nil
}

func newCandidateModel(value argodomain.Application, observedAt time.Time, token uuid.UUID) (candidateModel, error) {
	labels, sources, revisions, err := marshalObservation(value)
	if err != nil {
		return candidateModel{}, err
	}
	return candidateModel{
		ArgoCDNamespace: value.Identity.Namespace, ArgoCDApplicationName: value.Identity.Name, ArgoCDProject: value.Project,
		Labels: labels, Sources: sources, DestinationServer: value.Destination.Server, DestinationName: value.Destination.Name,
		DestinationNamespace: value.Destination.Namespace, AutomatedSync: value.AutomatedSync, SyncStatus: value.SyncStatus,
		HealthStatus: value.HealthStatus, OperationPhase: value.OperationPhase, ResolvedRevision: value.ResolvedRevision,
		ResolvedRevisions: revisions, ResourceVersion: value.ResourceVersion, DiscoveryStatus: "present",
		FirstSeenAt: observedAt, LastSeenAt: observedAt, ReconciliationToken: token, Version: 1,
	}, nil
}

func newSnapshotModel(applicationID uuid.UUID, value argodomain.Application, observedAt time.Time, token uuid.UUID) (snapshotModel, error) {
	labels, sources, revisions, err := marshalObservation(value)
	if err != nil {
		return snapshotModel{}, err
	}
	return snapshotModel{
		ApplicationID: applicationID, ArgoCDProject: value.Project, Labels: labels, Sources: sources,
		DestinationServer: value.Destination.Server, DestinationName: value.Destination.Name, DestinationNamespace: value.Destination.Namespace,
		AutomatedSync: value.AutomatedSync, ManagedLabelPresent: value.IsCandidate(), SyncStatus: value.SyncStatus,
		HealthStatus: value.HealthStatus, OperationPhase: value.OperationPhase, ResolvedRevision: value.ResolvedRevision,
		ResolvedRevisions: revisions, ResourceVersion: value.ResourceVersion, ReconciledAt: value.ReconciledAt,
		LastSeenAt: observedAt, ReconciliationToken: token, Version: 1,
	}, nil
}

func marshalObservation(value argodomain.Application) (datatypes.JSON, datatypes.JSON, datatypes.JSON, error) {
	labelsValue := value.Labels
	if labelsValue == nil {
		labelsValue = map[string]string{}
	}
	sourcesValue := value.Sources
	if sourcesValue == nil {
		sourcesValue = []argodomain.Source{}
	}
	revisionsValue := value.ResolvedRevisions
	if revisionsValue == nil {
		revisionsValue = []string{}
	}
	labels, err := json.Marshal(labelsValue)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal Argo CD labels: %w", err)
	}
	sources, err := json.Marshal(sourcesValue)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal Argo CD sources: %w", err)
	}
	revisions, err := json.Marshal(revisionsValue)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal Argo CD revisions: %w", err)
	}
	return labels, sources, revisions, nil
}

func upsertCandidate(ctx context.Context, tx *gorm.DB, value candidateModel) error {
	updates := candidateUpdateColumns()
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "argocd_namespace"}, {Name: "argocd_application_name"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&value).Error; err != nil {
		return fmt.Errorf("upsert Argo CD candidate: %w", err)
	}
	return nil
}

func candidateUpdateColumns() map[string]any {
	columns := []string{
		"argocd_project", "labels", "sources", "destination_server", "destination_name", "destination_namespace",
		"automated_sync", "sync_status", "health_status", "operation_phase", "resolved_revision", "resolved_revisions",
		"resource_version", "last_seen_at", "reconciliation_token",
	}
	result := make(map[string]any, len(columns)+4)
	for _, column := range columns {
		result[column] = gorm.Expr("EXCLUDED." + column)
	}
	result["discovery_status"] = "present"
	result["missing_since"] = nil
	result["version"] = gorm.Expr("argocd_application_candidates.version + 1")
	result["updated_at"] = gorm.Expr("CURRENT_TIMESTAMP")
	return result
}

func upsertSnapshot(ctx context.Context, tx *gorm.DB, value snapshotModel) error {
	columns := []string{
		"argocd_project", "labels", "sources", "destination_server", "destination_name", "destination_namespace", "automated_sync",
		"managed_label_present", "sync_status", "health_status", "operation_phase", "resolved_revision", "resolved_revisions",
		"resource_version", "reconciled_at", "last_seen_at", "reconciliation_token",
	}
	updates := make(map[string]any, len(columns)+3)
	for _, column := range columns {
		updates[column] = gorm.Expr("EXCLUDED." + column)
	}
	updates["missing_since"] = nil
	updates["version"] = gorm.Expr("argocd_application_snapshots.version + 1")
	updates["updated_at"] = gorm.Expr("CURRENT_TIMESTAMP")
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "application_id"}}, DoUpdates: clause.Assignments(updates),
	}).Create(&value).Error; err != nil {
		return fmt.Errorf("upsert tracked Argo CD Application snapshot: %w", err)
	}
	return nil
}

func markMissingCandidates(ctx context.Context, tx *gorm.DB, token uuid.UUID, observedAt time.Time) error {
	if err := tx.WithContext(ctx).Model(&candidateModel{}).
		Where("discovery_status = ? AND reconciliation_token <> ?", "present", token).
		Updates(map[string]any{"discovery_status": "missing", "missing_since": observedAt, "version": gorm.Expr("version + 1"), "updated_at": observedAt}).Error; err != nil {
		return fmt.Errorf("mark missing Argo CD candidates: %w", err)
	}
	return nil
}

func markMissingSnapshots(ctx context.Context, tx *gorm.DB, token uuid.UUID, observedAt time.Time) error {
	if err := tx.WithContext(ctx).Model(&snapshotModel{}).
		Where("missing_since IS NULL AND reconciliation_token <> ?", token).
		Updates(map[string]any{"missing_since": observedAt, "version": gorm.Expr("version + 1"), "updated_at": observedAt}).Error; err != nil {
		return fmt.Errorf("mark missing tracked Argo CD Application snapshots: %w", err)
	}
	return nil
}

func detectManagedDrift(ctx context.Context, tx *gorm.DB, observedAt time.Time) error {
	var values []managedDriftModel
	if err := tx.WithContext(ctx).Raw(`
SELECT onboarding.application_id,
       application.organization_id,
       application.project_id,
       application.environment_id,
       application.argocd_namespace,
       application.argocd_application_name,
       application.argocd_project AS expected_argocd_project,
       application.destination_server AS expected_destination_server,
       application.destination_namespace AS expected_destination_namespace,
       application.source_repo_url,
       application.source_target_revision,
       application.source_path,
       snapshot.application_id IS NOT NULL AND snapshot.missing_since IS NULL AS present,
       snapshot.argocd_project AS observed_argocd_project,
       snapshot.labels,
       snapshot.sources,
       snapshot.destination_server AS observed_destination_server,
       snapshot.destination_namespace AS observed_destination_namespace,
       snapshot.automated_sync
FROM application_onboardings onboarding
JOIN applications application ON application.id = onboarding.application_id
LEFT JOIN argocd_application_snapshots snapshot ON snapshot.application_id = onboarding.application_id
WHERE onboarding.status = 'Managed'`).Scan(&values).Error; err != nil {
		return fmt.Errorf("load managed Application drift observations: %w", err)
	}
	for _, value := range values {
		reasons, err := value.reasons()
		if err != nil {
			return err
		}
		if len(reasons) == 0 {
			continue
		}
		encoded, err := json.Marshal(reasons)
		if err != nil {
			return fmt.Errorf("marshal Application drift reasons: %w", err)
		}
		query := tx.WithContext(ctx).Model(&onboardingModel{}).
			Where("application_id = ? AND status = ?", value.ApplicationID, argodomain.OnboardingManaged).
			Updates(map[string]any{
				"status":            argodomain.OnboardingConfigurationDrift,
				"drift_detected_at": observedAt,
				"drift_reasons":     datatypes.JSON(encoded),
				"version":           gorm.Expr("version + 1"),
				"updated_at":        observedAt,
			})
		if query.Error != nil {
			return fmt.Errorf("mark Application configuration drift: %w", query.Error)
		}
		if query.RowsAffected == 1 {
			if err := appendOnboardingChange(ctx, tx, nil, "worker-reconciliation", value.OrganizationID, value.ApplicationID, string(argodomain.OnboardingConfigurationDrift), observedAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendCandidateDiscovered(ctx context.Context, tx *gorm.DB, value argodomain.Application, occurredAt time.Time) error {
	resourceID := identityKey(value.Identity.Namespace, value.Identity.Name)
	payload, err := json.Marshal(map[string]string{"namespace": value.Identity.Namespace, "name": value.Identity.Name})
	if err != nil {
		return fmt.Errorf("marshal Argo CD candidate event: %w", err)
	}
	event, err := platform.NewEvent("argocd.application.candidate_discovered", "argocd_application_candidate", resourceID, payload, occurredAt)
	if err != nil {
		return fmt.Errorf("create Argo CD candidate event: %w", err)
	}
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: occurredAt, Action: "argocd_application_candidate.discover", ResourceType: "argocd_application_candidate",
		ResourceID: resourceID, Metadata: map[string]any{},
	}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func identityKey(namespace, name string) string { return namespace + "/" + name }

type candidateIdentityModel struct {
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
}

type trackedApplicationModel struct {
	ID                    uuid.UUID
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
}

type candidateModel struct {
	ID                    uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ArgoCDNamespace       string         `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string         `gorm:"column:argocd_application_name"`
	ArgoCDProject         string         `gorm:"column:argocd_project"`
	Labels                datatypes.JSON `gorm:"type:jsonb"`
	Sources               datatypes.JSON `gorm:"type:jsonb"`
	DestinationServer     string
	DestinationName       string
	DestinationNamespace  string
	AutomatedSync         bool
	SyncStatus            string
	HealthStatus          string
	OperationPhase        string
	ResolvedRevision      string
	ResolvedRevisions     datatypes.JSON `gorm:"type:jsonb"`
	ResourceVersion       string
	DiscoveryStatus       string
	FirstSeenAt           time.Time
	LastSeenAt            time.Time
	MissingSince          *time.Time
	ReconciliationToken   uuid.UUID `gorm:"type:uuid"`
	Version               uint64
}

func (candidateModel) TableName() string { return "argocd_application_candidates" }

type snapshotModel struct {
	ApplicationID        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ArgoCDProject        string         `gorm:"column:argocd_project"`
	Labels               datatypes.JSON `gorm:"type:jsonb"`
	Sources              datatypes.JSON `gorm:"type:jsonb"`
	DestinationServer    string
	DestinationName      string
	DestinationNamespace string
	AutomatedSync        bool
	ManagedLabelPresent  bool
	SyncStatus           string
	HealthStatus         string
	OperationPhase       string
	ResolvedRevision     string
	ResolvedRevisions    datatypes.JSON `gorm:"type:jsonb"`
	ResourceVersion      string
	ReconciledAt         *time.Time
	LastSeenAt           time.Time
	MissingSince         *time.Time
	ReconciliationToken  uuid.UUID `gorm:"type:uuid"`
	Version              uint64
}

func (snapshotModel) TableName() string { return "argocd_application_snapshots" }

type managedDriftModel struct {
	ApplicationID                uuid.UUID
	OrganizationID               uuid.UUID
	ProjectID                    uuid.UUID
	EnvironmentID                uuid.UUID
	ArgoCDNamespace              string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName        string `gorm:"column:argocd_application_name"`
	ExpectedArgoCDProject        string
	ExpectedDestinationServer    string
	ExpectedDestinationNamespace string
	SourceRepositoryURL          string `gorm:"column:source_repo_url"`
	SourceTargetRevision         string
	SourcePath                   string
	Present                      bool
	ObservedArgoCDProject        string
	Labels                       datatypes.JSON `gorm:"type:jsonb"`
	Sources                      datatypes.JSON `gorm:"type:jsonb"`
	ObservedDestinationServer    string
	ObservedDestinationNamespace string
	AutomatedSync                bool
}

func (m managedDriftModel) reasons() ([]argodomain.DriftReason, error) {
	if !m.Present {
		return []argodomain.DriftReason{argodomain.DriftApplicationMissing}, nil
	}
	var labels map[string]string
	if err := json.Unmarshal(m.Labels, &labels); err != nil {
		return nil, fmt.Errorf("decode tracked Application labels: %w", err)
	}
	var sources []argodomain.Source
	if err := json.Unmarshal(m.Sources, &sources); err != nil {
		return nil, fmt.Errorf("decode tracked Application sources: %w", err)
	}
	target := argodomain.OnboardingTarget{
		ApplicationID: m.ApplicationID, OrganizationID: m.OrganizationID, ProjectID: m.ProjectID, EnvironmentID: m.EnvironmentID,
		Argo:        argodomain.ApplicationIdentity{Namespace: m.ArgoCDNamespace, Name: m.ArgoCDApplicationName},
		ArgoProject: m.ExpectedArgoCDProject, DestinationServer: m.ExpectedDestinationServer,
		DestinationNamespace: m.ExpectedDestinationNamespace,
		Source:               argodomain.CatalogSource{RepositoryURL: m.SourceRepositoryURL, TargetRevision: m.SourceTargetRevision, Path: m.SourcePath},
	}
	observation := argodomain.Application{
		Identity: target.Argo, Project: m.ObservedArgoCDProject, Labels: labels, Sources: sources,
		Destination:   argodomain.Destination{Server: m.ObservedDestinationServer, Namespace: m.ObservedDestinationNamespace},
		AutomatedSync: m.AutomatedSync,
	}
	return target.DetectDrift(observation, true), nil
}
