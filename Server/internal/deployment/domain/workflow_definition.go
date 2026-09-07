package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefinitionLifecycle identifies whether a workflow version may be bound.
type DefinitionLifecycle string

const (
	DefinitionDraft     DefinitionLifecycle = "Draft"
	DefinitionPublished DefinitionLifecycle = "Published"
	DefinitionDisabled  DefinitionLifecycle = "Disabled"
)

// ReleaseWorkflow is the stable identity for versioned workflow documents.
type ReleaseWorkflow struct {
	ID          uuid.UUID
	Name        string
	Description string
	Active      bool
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Versions    []ReleaseWorkflowVersion
}

// ReleaseWorkflowVersion is one immutable workflow graph revision.
type ReleaseWorkflowVersion struct {
	ID            uuid.UUID
	WorkflowID    uuid.UUID
	VersionNumber uint64
	Lifecycle     DefinitionLifecycle
	Document      WorkflowDocument
	LockVersion   uint64
	CreatedBy     uuid.UUID
	PublishedAt   *time.Time
	DisabledAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WorkflowVersionDraft contains the inputs for one new immutable version.
type WorkflowVersionDraft struct {
	WorkflowID    uuid.UUID
	VersionNumber uint64
	ActorID       uuid.UUID
	Document      WorkflowDocument
	CreatedAt     time.Time
}

// NewReleaseWorkflow creates a workflow with its first draft version.
func NewReleaseWorkflow(value ReleaseWorkflow, document WorkflowDocument) (ReleaseWorkflow, error) {
	value, err := normalizeReleaseWorkflow(value)
	if err != nil {
		return ReleaseWorkflow{}, err
	}
	version, err := NewReleaseWorkflowVersion(WorkflowVersionDraft{
		WorkflowID: value.ID, VersionNumber: 1, ActorID: value.CreatedBy,
		Document: document, CreatedAt: value.CreatedAt,
	})
	if err != nil {
		return ReleaseWorkflow{}, err
	}
	value.Active = true
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.CreatedAt
	value.Versions = []ReleaseWorkflowVersion{version}
	return value, nil
}

func normalizeReleaseWorkflow(value ReleaseWorkflow) (ReleaseWorkflow, error) {
	value.Name = strings.TrimSpace(value.Name)
	value.Description = strings.TrimSpace(value.Description)
	if value.Name == "" || len(value.Name) > 128 || len(value.Description) > 4096 {
		return ReleaseWorkflow{}, errors.New("workflow name or description is invalid")
	}
	if value.ID == uuid.Nil || value.CreatedBy == uuid.Nil || value.CreatedAt.IsZero() {
		return ReleaseWorkflow{}, errors.New("workflow identity, creator, and creation time are required")
	}
	return value, nil
}

// NewReleaseWorkflowVersion creates one validated draft version.
func NewReleaseWorkflowVersion(draft WorkflowVersionDraft) (ReleaseWorkflowVersion, error) {
	if draft.WorkflowID == uuid.Nil || draft.VersionNumber == 0 || draft.ActorID == uuid.Nil || draft.CreatedAt.IsZero() {
		return ReleaseWorkflowVersion{}, errors.New("workflow version identity, number, actor, and time are required")
	}
	validated, err := NewWorkflowDocument(draft.Document)
	if err != nil {
		return ReleaseWorkflowVersion{}, err
	}
	return ReleaseWorkflowVersion{
		ID: uuid.New(), WorkflowID: draft.WorkflowID, VersionNumber: draft.VersionNumber,
		Lifecycle: DefinitionDraft, Document: validated, LockVersion: 1,
		CreatedBy: draft.ActorID, CreatedAt: draft.CreatedAt.UTC(), UpdatedAt: draft.CreatedAt.UTC(),
	}, nil
}

// ChangeLifecycle publishes a draft or disables a published version.
func (v ReleaseWorkflowVersion) ChangeLifecycle(target DefinitionLifecycle, expected uint64, now time.Time) (ReleaseWorkflowVersion, error) {
	if expected != v.LockVersion {
		return ReleaseWorkflowVersion{}, errors.New("workflow version changed concurrently")
	}
	if now.IsZero() {
		return ReleaseWorkflowVersion{}, errors.New("workflow lifecycle change time is required")
	}
	if !validLifecycleTransition(v.Lifecycle, target) {
		return ReleaseWorkflowVersion{}, errors.New("workflow lifecycle transition is invalid")
	}
	v.Lifecycle = target
	v.LockVersion++
	v.UpdatedAt = now.UTC()
	if target == DefinitionPublished {
		v.PublishedAt = timePointer(now)
	} else {
		v.DisabledAt = timePointer(now)
	}
	return v, nil
}

func validLifecycleTransition(current, target DefinitionLifecycle) bool {
	return current == DefinitionDraft && target == DefinitionPublished ||
		current == DefinitionPublished && target == DefinitionDisabled
}

func timePointer(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}
