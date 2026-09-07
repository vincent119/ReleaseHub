package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// CandidateRepository reads persisted Argo CD discovery state.
type CandidateRepository struct{ db *gorm.DB }

// NewCandidateRepository creates the PostgreSQL candidate queue adapter.
func NewCandidateRepository(db *gorm.DB) (*CandidateRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &CandidateRepository{db: db}, nil
}

// ListPresent returns only candidates present in the latest successful reconciliation.
func (r *CandidateRepository) ListPresent(ctx context.Context) ([]argoapp.Candidate, error) {
	var models []candidateModel
	if err := r.db.WithContext(ctx).Where(`discovery_status = ? AND NOT EXISTS (
SELECT 1 FROM applications
WHERE applications.active
  AND (applications.candidate_id = argocd_application_candidates.id
       OR (applications.argocd_namespace = argocd_application_candidates.argocd_namespace
           AND applications.argocd_application_name = argocd_application_candidates.argocd_application_name))
)`, "present").Order("last_seen_at DESC, argocd_namespace, argocd_application_name").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list Argo CD candidates: %w", err)
	}
	result := make([]argoapp.Candidate, 0, len(models))
	for _, model := range models {
		candidate, err := candidateDomain(model)
		if err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, nil
}

// ListAssignmentScopes returns active production catalog ancestors for the assignment form.
func (r *CandidateRepository) ListAssignmentScopes(ctx context.Context) ([]argoapp.AssignmentScope, error) {
	var values []argoapp.AssignmentScope
	err := r.db.WithContext(ctx).Raw(`
SELECT organization.id AS organization_id, organization.name AS organization_name,
       project.id AS project_id, project.name AS project_name,
       environment.id AS environment_id, environment.name AS environment_name
FROM environments environment
JOIN projects project ON project.id = environment.project_id AND project.organization_id = environment.organization_id
JOIN organizations organization ON organization.id = environment.organization_id
	WHERE organization.active AND project.active AND environment.active AND environment.environment_type = 'Production'
	ORDER BY lower(organization.name), lower(project.name), lower(environment.name), environment.id`).Scan(&values).Error
	if err != nil {
		return nil, fmt.Errorf("list candidate assignment scopes: %w", err)
	}
	return values, nil
}

// LoadPresent reads one unassigned candidate from the latest successful reconciliation.
func (r *CandidateRepository) LoadPresent(ctx context.Context, candidateID uuid.UUID) (argoapp.Candidate, error) {
	var model candidateModel
	err := r.db.WithContext(ctx).Where(`id = ? AND discovery_status = ? AND NOT EXISTS (
SELECT 1 FROM applications
WHERE applications.active
  AND (applications.candidate_id = argocd_application_candidates.id
       OR (applications.argocd_namespace = argocd_application_candidates.argocd_namespace
           AND applications.argocd_application_name = argocd_application_candidates.argocd_application_name))
)`, candidateID, "present").Take(&model).Error
	if err != nil {
		return argoapp.Candidate{}, fmt.Errorf("load Argo CD candidate: %w", err)
	}
	return candidateDomain(model)
}

// Assign atomically creates a production catalog Application linked to one fresh candidate version.
func (r *CandidateRepository) Assign(ctx context.Context, mutation catalogapp.Mutation, candidateID uuid.UUID, expectedVersion uint64, value catalog.Application) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var candidate candidateModel
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND discovery_status = ? AND version = ?", candidateID, "present", expectedVersion).Take(&candidate).Error; err != nil {
			return fmt.Errorf("lock candidate assignment: %w", err)
		}
		var environmentCount int64
		if err := tx.WithContext(ctx).Table("environments").Where("id = ? AND organization_id = ? AND project_id = ? AND environment_type = 'Production' AND active", value.EnvironmentID, value.OrganizationID, value.ProjectID).Count(&environmentCount).Error; err != nil || environmentCount != 1 {
			return fmt.Errorf("candidate assignment requires an active production environment")
		}
		if err := tx.WithContext(ctx).Exec(`INSERT INTO applications (id, organization_id, project_id, environment_id, candidate_id, name, argocd_namespace, argocd_application_name, argocd_project, destination_server, destination_namespace, source_repo_url, source_target_revision, source_path, active, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, true, 1)`, value.ID, value.OrganizationID, value.ProjectID, value.EnvironmentID, candidateID, value.Name, value.Argo.Namespace, value.Argo.Name, value.ArgoProject, value.DestinationServer, value.DestinationNamespace, value.Source.RepositoryURL, value.Source.TargetRevision, value.Source.Path).Error; err != nil {
			return fmt.Errorf("create candidate Application mapping: %w", err)
		}
		now := time.Now().UTC()
		payload, err := json.Marshal(map[string]string{"candidateId": candidateID.String(), "applicationId": value.ID.String()})
		if err != nil {
			return fmt.Errorf("marshal candidate assignment event: %w", err)
		}
		event, err := platform.NewEvent("argocd.application.candidate_assigned", "application", value.ID.String(), payload, now)
		if err != nil {
			return fmt.Errorf("create candidate assignment event: %w", err)
		}
		if err := database.AppendAudit(ctx, tx, database.AuditRecord{OccurredAt: now, ActorID: mutation.ActorID, OrganizationID: &value.OrganizationID, Action: "argocd_candidate.assign", ResourceType: "application", ResourceID: value.ID.String(), RequestID: mutation.RequestID, Metadata: map[string]any{"candidateId": candidateID.String()}}); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
}

func candidateDomain(model candidateModel) (argoapp.Candidate, error) {
	var sources []argodomain.Source
	if err := json.Unmarshal(model.Sources, &sources); err != nil {
		return argoapp.Candidate{}, fmt.Errorf("decode candidate sources: %w", err)
	}
	return argoapp.Candidate{ID: model.ID, ArgoCDNamespace: model.ArgoCDNamespace, ArgoCDApplicationName: model.ArgoCDApplicationName, ArgoCDProject: model.ArgoCDProject, Sources: sources, DestinationServer: model.DestinationServer, DestinationName: model.DestinationName, DestinationNamespace: model.DestinationNamespace, AutomatedSync: model.AutomatedSync, SyncStatus: model.SyncStatus, HealthStatus: model.HealthStatus, OperationPhase: model.OperationPhase, ResolvedRevision: model.ResolvedRevision, ResourceVersion: model.ResourceVersion, FirstSeenAt: model.FirstSeenAt, LastSeenAt: model.LastSeenAt, Version: model.Version}, nil
}
