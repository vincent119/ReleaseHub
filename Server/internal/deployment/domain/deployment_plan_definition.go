package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeploymentPlanOwnerKind identifies whether a Plan is platform-wide or Project-owned.
type DeploymentPlanOwnerKind string

const (
	DeploymentPlanOwnerPlatform DeploymentPlanOwnerKind = "platform"
	DeploymentPlanOwnerProject  DeploymentPlanOwnerKind = "project"
)

// DeploymentPlan is the stable identity for versioned Plan documents.
type DeploymentPlan struct {
	ID             uuid.UUID
	OwnerKind      DeploymentPlanOwnerKind
	OwnerProjectID *uuid.UUID
	Name           string
	Description    string
	Active         bool
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Versions       []DeploymentPlanVersion
}

// DeploymentPlanVersion is one immutable Plan graph revision.
type DeploymentPlanVersion struct {
	ID            uuid.UUID
	PlanID        uuid.UUID
	VersionNumber uint64
	Lifecycle     DefinitionLifecycle
	Document      DeploymentPlanDocument
	LockVersion   uint64
	CreatedBy     uuid.UUID
	PublishedAt   *time.Time
	DisabledAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DeploymentPlanVersionDraft contains one new immutable version command.
type DeploymentPlanVersionDraft struct {
	PlanID        uuid.UUID
	VersionNumber uint64
	ActorID       uuid.UUID
	Document      DeploymentPlanDocument
	CreatedAt     time.Time
}

// NewDeploymentPlan creates a Plan and its first Draft Version.
func NewDeploymentPlan(value DeploymentPlan, document DeploymentPlanDocument) (DeploymentPlan, error) {
	value, err := normalizeDeploymentPlan(value)
	if err != nil {
		return DeploymentPlan{}, err
	}
	version, err := NewDeploymentPlanVersion(DeploymentPlanVersionDraft{
		PlanID: value.ID, VersionNumber: 1, ActorID: value.CreatedBy,
		Document: document, CreatedAt: value.CreatedAt,
	})
	if err != nil {
		return DeploymentPlan{}, err
	}
	value.Active = true
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.CreatedAt
	value.Versions = []DeploymentPlanVersion{version}
	return value, nil
}

func normalizeDeploymentPlan(value DeploymentPlan) (DeploymentPlan, error) {
	value.Name, value.Description = strings.TrimSpace(value.Name), strings.TrimSpace(value.Description)
	if value.Name == "" || len(value.Name) > 128 || len(value.Description) > 4096 {
		return DeploymentPlan{}, errors.New("deployment plan name or description is invalid")
	}
	if value.ID == uuid.Nil || value.CreatedBy == uuid.Nil || value.CreatedAt.IsZero() {
		return DeploymentPlan{}, errors.New("deployment plan identity, creator, and creation time are required")
	}
	if !validDeploymentPlanOwner(value.OwnerKind, value.OwnerProjectID) {
		return DeploymentPlan{}, errors.New("deployment plan owner is invalid")
	}
	return value, nil
}

func validDeploymentPlanOwner(kind DeploymentPlanOwnerKind, projectID *uuid.UUID) bool {
	if kind == DeploymentPlanOwnerPlatform {
		return projectID == nil
	}
	return kind == DeploymentPlanOwnerProject && projectID != nil && *projectID != uuid.Nil
}

// NewDeploymentPlanVersion creates one structurally valid Draft Version.
func NewDeploymentPlanVersion(draft DeploymentPlanVersionDraft) (DeploymentPlanVersion, error) {
	if draft.PlanID == uuid.Nil || draft.VersionNumber == 0 || draft.ActorID == uuid.Nil || draft.CreatedAt.IsZero() {
		return DeploymentPlanVersion{}, errors.New("deployment plan version identity, number, actor, and time are required")
	}
	document, err := NewDeploymentPlanDocument(draft.Document)
	if err != nil {
		return DeploymentPlanVersion{}, err
	}
	return DeploymentPlanVersion{
		ID: uuid.New(), PlanID: draft.PlanID, VersionNumber: draft.VersionNumber,
		Lifecycle: DefinitionDraft, Document: document, LockVersion: 1,
		CreatedBy: draft.ActorID, CreatedAt: draft.CreatedAt.UTC(), UpdatedAt: draft.CreatedAt.UTC(),
	}, nil
}

// ChangeLifecycle publishes an executable Draft or disables a Published Version.
func (v DeploymentPlanVersion) ChangeLifecycle(target DefinitionLifecycle, expected uint64, now time.Time) (DeploymentPlanVersion, error) {
	if expected != v.LockVersion {
		return DeploymentPlanVersion{}, errors.New("deployment plan version changed concurrently")
	}
	if now.IsZero() || !validLifecycleTransition(v.Lifecycle, target) {
		return DeploymentPlanVersion{}, errors.New("deployment plan lifecycle transition is invalid")
	}
	if target == DefinitionPublished {
		if _, err := NewDeploymentPlanEngine(v.Document); err != nil {
			return DeploymentPlanVersion{}, err
		}
	}
	v.Lifecycle, v.LockVersion, v.UpdatedAt = target, v.LockVersion+1, now.UTC()
	if target == DefinitionPublished {
		v.PublishedAt = timePointer(now)
	} else {
		v.DisabledAt = timePointer(now)
	}
	return v, nil
}
