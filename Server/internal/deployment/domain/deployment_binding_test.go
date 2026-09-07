package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeploymentBindingVersionSwitch(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	binding, err := NewDeploymentBinding(DeploymentBinding{
		ID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		WorkflowVersionID: uuid.New(), PlanVersionID: uuid.New(), CreatedBy: uuid.New(), CreatedAt: now,
	})
	if err != nil || binding.Version != 1 {
		t.Fatalf("new binding = %#v, error = %v", binding, err)
	}
	updated, err := binding.Rebind(uuid.New(), uuid.New(), 1, now.Add(time.Minute))
	if err != nil || updated.Version != 2 || updated.ID != binding.ID {
		t.Fatalf("updated binding = %#v, error = %v", updated, err)
	}
	if _, err := binding.Rebind(uuid.New(), uuid.New(), 2, now); err == nil {
		t.Fatal("expected stale binding switch to fail")
	}
}
