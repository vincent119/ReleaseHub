package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// DeploymentBinding pins one Environment to published Workflow and Plan Versions.
type DeploymentBinding struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	Version           uint64
	CreatedBy         uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewDeploymentBinding creates the first binding version for an Environment.
func NewDeploymentBinding(value DeploymentBinding) (DeploymentBinding, error) {
	if !validBindingIdentity(value) || value.Version != 0 || value.CreatedAt.IsZero() {
		return DeploymentBinding{}, errors.New("deployment binding identity and time are required")
	}
	value.Version = 1
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.CreatedAt
	return value, nil
}

// Rebind switches an Environment to another published definition pair.
func (b DeploymentBinding) Rebind(workflowVersionID, planVersionID uuid.UUID, expected uint64, now time.Time) (DeploymentBinding, error) {
	if expected != b.Version || workflowVersionID == uuid.Nil || planVersionID == uuid.Nil || now.IsZero() {
		return DeploymentBinding{}, errors.New("deployment binding changed concurrently or selection is invalid")
	}
	b.WorkflowVersionID, b.PlanVersionID = workflowVersionID, planVersionID
	b.Version++
	b.UpdatedAt = now.UTC()
	return b, nil
}

func validBindingIdentity(value DeploymentBinding) bool {
	return value.ID != uuid.Nil && value.OrganizationID != uuid.Nil &&
		value.ProjectID != uuid.Nil && value.EnvironmentID != uuid.Nil &&
		value.WorkflowVersionID != uuid.Nil && value.PlanVersionID != uuid.Nil &&
		value.CreatedBy != uuid.Nil
}
