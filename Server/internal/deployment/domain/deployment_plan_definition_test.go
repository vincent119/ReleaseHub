package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeploymentPlanDraftCycleIsRejectedOnPublish(t *testing.T) {
	document := planDocumentForApplications("api", "worker")
	document.Edges = []DeploymentPlanEdge{
		{From: "api", To: "worker", Condition: PlanEdgeUpstreamSucceeded},
		{From: "worker", To: "api", Condition: PlanEdgeUpstreamSucceeded},
	}
	version, err := NewDeploymentPlanVersion(DeploymentPlanVersionDraft{
		PlanID: uuid.New(), VersionNumber: 1, ActorID: uuid.New(),
		Document: document, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create editable draft: %v", err)
	}
	if _, err := version.ChangeLifecycle(DefinitionPublished, 1, time.Now()); err == nil {
		t.Fatal("cycle was published")
	}
}

func TestDeploymentPlanVersionLifecycleIsImmutable(t *testing.T) {
	version, err := NewDeploymentPlanVersion(DeploymentPlanVersionDraft{
		PlanID: uuid.New(), VersionNumber: 1, ActorID: uuid.New(),
		Document: planDocumentForApplications("api"), CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create plan version: %v", err)
	}
	published, err := version.ChangeLifecycle(DefinitionPublished, 1, time.Now())
	if err != nil || published.Lifecycle != DefinitionPublished || published.LockVersion != 2 {
		t.Fatalf("publish plan version: %#v, %v", published, err)
	}
	disabled, err := published.ChangeLifecycle(DefinitionDisabled, 2, time.Now())
	if err != nil || disabled.Lifecycle != DefinitionDisabled || disabled.LockVersion != 3 {
		t.Fatalf("disable plan version: %#v, %v", disabled, err)
	}
}
