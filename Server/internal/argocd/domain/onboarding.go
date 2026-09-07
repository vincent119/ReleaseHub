package domain

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// OnboardingStatus represents the durable production onboarding lifecycle.
type OnboardingStatus string

const (
	OnboardingValidationFailed     OnboardingStatus = "ValidationFailed"
	OnboardingAwaitingConfirmation OnboardingStatus = "AwaitingConfirmation"
	OnboardingApplying             OnboardingStatus = "Applying"
	OnboardingManaged              OnboardingStatus = "Managed"
	OnboardingConfigurationDrift   OnboardingStatus = "ConfigurationDrift"
)

// ValidationIssueCode is a stable non-vendor failure reason.
type ValidationIssueCode string

const (
	IssueCandidateUnavailable        ValidationIssueCode = "candidate_unavailable"
	IssueApplicationUnavailable      ValidationIssueCode = "application_unavailable"
	IssueApplicationIdentityMismatch ValidationIssueCode = "application_identity_mismatch"
	IssueProductionRequired          ValidationIssueCode = "production_environment_required"
	IssueManagedLabelMissing         ValidationIssueCode = "managed_label_missing"
	IssueProjectMismatch             ValidationIssueCode = "argocd_project_mismatch"
	IssueDestinationMismatch         ValidationIssueCode = "destination_mismatch"
	IssueSourceMismatch              ValidationIssueCode = "source_mapping_mismatch"
	IssueUpdatePermissionDenied      ValidationIssueCode = "update_permission_denied"
	IssueAutomatedSyncStillActive    ValidationIssueCode = "automated_sync_still_active"
)

// DriftReason is a stable condition that blocks downstream deployment use.
type DriftReason string

const (
	DriftApplicationMissing   DriftReason = "application_missing"
	DriftManagedLabelMissing  DriftReason = "managed_label_missing"
	DriftAutomatedSyncEnabled DriftReason = "automated_sync_enabled"
	DriftProjectMismatch      DriftReason = "argocd_project_mismatch"
	DriftDestinationMismatch  DriftReason = "destination_mismatch"
	DriftSourceMismatch       DriftReason = "source_mapping_mismatch"
)

// CatalogSource is the source mapping confirmed for one ReleaseHub Application.
type CatalogSource struct {
	RepositoryURL  string
	TargetRevision string
	Path           string
}

// OnboardingTarget contains the catalog and discovery facts needed by validation.
type OnboardingTarget struct {
	ApplicationID        uuid.UUID
	OrganizationID       uuid.UUID
	ProjectID            uuid.UUID
	EnvironmentID        uuid.UUID
	Production           bool
	Argo                 ApplicationIdentity
	ArgoProject          string
	DestinationServer    string
	DestinationNamespace string
	Source               CatalogSource
	CandidatePresent     bool
}

// Onboarding is the durable state returned to application commands.
type Onboarding struct {
	ApplicationID            uuid.UUID
	Status                   OnboardingStatus
	ValidationIssues         []ValidationIssueCode
	ValidatedResourceVersion string
	ValidatedAt              *time.Time
	ConfirmedBy              *uuid.UUID
	ConfirmedAt              *time.Time
	ManagedAt                *time.Time
	DriftDetectedAt          *time.Time
	DriftReasons             []DriftReason
	LastErrorCode            string
	Version                  uint64
}

// ValidateObservation compares a fresh Argo CD read with the confirmed catalog mapping.
func (t OnboardingTarget) ValidateObservation(observation Application) []ValidationIssueCode {
	issues := make([]ValidationIssueCode, 0, 6)
	if !t.Production {
		issues = append(issues, IssueProductionRequired)
	}
	if !t.CandidatePresent {
		issues = append(issues, IssueCandidateUnavailable)
	}
	if observation.Identity != t.Argo {
		issues = append(issues, IssueApplicationIdentityMismatch)
	}
	if !observation.IsCandidate() {
		issues = append(issues, IssueManagedLabelMissing)
	}
	if observation.Project != t.ArgoProject {
		issues = append(issues, IssueProjectMismatch)
	}
	if observation.Destination.Server != t.DestinationServer || observation.Destination.Namespace != t.DestinationNamespace {
		issues = append(issues, IssueDestinationMismatch)
	}
	if !sourcePresent(t.Source, observation.Sources) {
		issues = append(issues, IssueSourceMismatch)
	}
	return issues
}

// DetectDrift compares the latest tracked observation with the mapping that was confirmed during onboarding.
func (t OnboardingTarget) DetectDrift(observation Application, present bool) []DriftReason {
	if !present {
		return []DriftReason{DriftApplicationMissing}
	}
	reasons := make([]DriftReason, 0, 5)
	if !observation.IsCandidate() {
		reasons = append(reasons, DriftManagedLabelMissing)
	}
	if observation.AutomatedSync {
		reasons = append(reasons, DriftAutomatedSyncEnabled)
	}
	if observation.Project != t.ArgoProject {
		reasons = append(reasons, DriftProjectMismatch)
	}
	if observation.Destination.Server != t.DestinationServer || observation.Destination.Namespace != t.DestinationNamespace {
		reasons = append(reasons, DriftDestinationMismatch)
	}
	if !sourcePresent(t.Source, observation.Sources) {
		reasons = append(reasons, DriftSourceMismatch)
	}
	return reasons
}

// Copy returns an onboarding state whose slices cannot be mutated by the caller.
func (o Onboarding) Copy() Onboarding {
	o.ValidationIssues = slices.Clone(o.ValidationIssues)
	o.DriftReasons = slices.Clone(o.DriftReasons)
	return o
}

func sourcePresent(expected CatalogSource, actual []Source) bool {
	for _, source := range actual {
		if source.RepositoryURL == expected.RepositoryURL && source.TargetRevision == expected.TargetRevision && source.Path == expected.Path {
			return true
		}
	}
	return false
}
