package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ApplicationStatus is the read-only Argo CD and onboarding state for one catalog Application.
type ApplicationStatus struct {
	CandidatePresent  bool
	AutomatedSync     bool
	SyncStatus        string
	HealthStatus      string
	OperationPhase    string
	ResolvedRevision  string
	OnboardingStatus  string
	OnboardingVersion uint64
	DriftReasons      []string
}

// StatusRepository reads the state that may be disclosed only after catalog authorization succeeds.
type StatusRepository struct{ db *gorm.DB }

// NewStatusRepository creates the PostgreSQL status reader.
func NewStatusRepository(db *gorm.DB) (*StatusRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &StatusRepository{db: db}, nil
}

// Load returns current stored observation and onboarding state without contacting Argo CD.
func (r *StatusRepository) Load(ctx context.Context, applicationID uuid.UUID) (ApplicationStatus, error) {
	var model applicationStatusModel
	query := r.db.WithContext(ctx).Raw(`
SELECT EXISTS (
           SELECT 1 FROM argocd_application_candidates candidate
           JOIN applications application ON application.argocd_namespace = candidate.argocd_namespace
                                      AND application.argocd_application_name = candidate.argocd_application_name
           WHERE application.id = ? AND candidate.discovery_status = 'present'
       ) AS candidate_present,
       COALESCE(snapshot.automated_sync, false) AS automated_sync,
       COALESCE(snapshot.sync_status, '') AS sync_status,
       COALESCE(snapshot.health_status, '') AS health_status,
       COALESCE(snapshot.operation_phase, '') AS operation_phase,
       COALESCE(snapshot.resolved_revision, '') AS resolved_revision,
       COALESCE(onboarding.status, '') AS onboarding_status,
	       COALESCE(onboarding.version, 0) AS onboarding_version,
       COALESCE(onboarding.drift_reasons, '[]'::jsonb) AS drift_reasons
FROM applications application
LEFT JOIN argocd_application_snapshots snapshot ON snapshot.application_id = application.id
LEFT JOIN application_onboardings onboarding ON onboarding.application_id = application.id
WHERE application.id = ?`, applicationID, applicationID).Scan(&model)
	if query.Error != nil {
		return ApplicationStatus{}, fmt.Errorf("load Application status: %w", query.Error)
	}
	if query.RowsAffected != 1 {
		return ApplicationStatus{}, gorm.ErrRecordNotFound
	}
	return model.domain(), nil
}

type applicationStatusModel struct {
	CandidatePresent  bool
	AutomatedSync     bool
	SyncStatus        string
	HealthStatus      string
	OperationPhase    string
	ResolvedRevision  string
	OnboardingStatus  string
	OnboardingVersion uint64
	DriftReasons      datatypes.JSON
}

func (m applicationStatusModel) domain() ApplicationStatus {
	var reasons []string
	_ = json.Unmarshal(m.DriftReasons, &reasons)
	if reasons == nil {
		reasons = []string{}
	}
	return ApplicationStatus{
		CandidatePresent: m.CandidatePresent, AutomatedSync: m.AutomatedSync, SyncStatus: m.SyncStatus,
		HealthStatus: m.HealthStatus, OperationPhase: m.OperationPhase, ResolvedRevision: m.ResolvedRevision,
		OnboardingStatus: m.OnboardingStatus, OnboardingVersion: m.OnboardingVersion, DriftReasons: reasons,
	}
}
