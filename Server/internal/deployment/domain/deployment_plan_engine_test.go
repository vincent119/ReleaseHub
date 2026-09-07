package domain

import (
	"testing"
	"time"
)

func TestDeploymentPlanDAGDependencies(t *testing.T) {
	engine := planEngineFixture(t, planDocumentFixture([]DeploymentPlanEdge{
		{From: "api", To: "worker", Condition: PlanEdgeUpstreamSucceeded},
		{From: "database", To: "worker", Condition: PlanEdgeUpstreamSucceeded},
	}))
	evaluation := DeploymentPlanEvaluation{
		ApplicationKeys: []string{"api", "database", "worker"},
		Nodes:           map[string]DeploymentNodeRuntime{},
	}
	assertReadyKeys(t, engine, evaluation, []string{"api", "database"})
	evaluation.Nodes = map[string]DeploymentNodeRuntime{
		"api": {Status: DeploymentNodeSucceeded}, "database": {Status: DeploymentNodeRunning},
	}
	assertReadyKeys(t, engine, evaluation, nil)
	evaluation.Nodes["database"] = DeploymentNodeRuntime{Status: DeploymentNodeSucceeded}
	assertReadyKeys(t, engine, evaluation, []string{"worker"})
}

func TestIndependentApplicationsAreGroupedByPlan(t *testing.T) {
	engine := planEngineFixture(t, planDocumentForApplications("api", "worker"))
	resolution, err := engine.ResolveSnapshot([]string{"unmapped-admin", "api"})
	if err != nil {
		t.Fatalf("resolve independent Applications: %v", err)
	}
	if len(resolution.Nodes) != 1 || resolution.Nodes[0].ApplicationKey != "api" {
		t.Fatalf("mapped nodes = %#v", resolution.Nodes)
	}
	if !slicesEqual(resolution.MissingApplicationKeys, []string{"unmapped-admin"}) {
		t.Fatalf("missing Applications = %v", resolution.MissingApplicationKeys)
	}
}

func TestDeploymentPlanParallelLimitAndOrder(t *testing.T) {
	limit := 2
	document := planDocumentForApplications("third", "first", "second")
	document.MaxParallel = &limit
	document.Nodes[0].Order, document.Nodes[1].Order, document.Nodes[2].Order = 3, 1, 2
	engine := planEngineFixture(t, document)
	assertReadyKeys(t, engine, DeploymentPlanEvaluation{
		ApplicationKeys: []string{"third", "first", "second"}, MaxParallel: 3,
	}, []string{"first", "second"})
}

func TestDeploymentPlanUsesHealthyPrerequisiteOutsideSnapshot(t *testing.T) {
	document := planDocumentForApplications("database", "api")
	document.Edges = []DeploymentPlanEdge{{
		From: "database", To: "api", Condition: PlanEdgePrerequisiteHealthy,
	}}
	engine := planEngineFixture(t, document)
	evaluation := DeploymentPlanEvaluation{
		ApplicationKeys: []string{"api"},
		Prerequisites: map[string]DeploymentNodeObservation{
			"database": {SyncStatus: "Synced", HealthStatus: "Healthy", StableFor: time.Minute},
		},
	}
	assertReadyKeys(t, engine, evaluation, []string{"api"})
	evaluation.Prerequisites["database"] = DeploymentNodeObservation{SyncStatus: "OutOfSync", HealthStatus: "Healthy"}
	assertReadyKeys(t, engine, evaluation, nil)
}

func TestDeploymentPlanPublishRejectsCycle(t *testing.T) {
	document := planDocumentForApplications("api", "worker")
	document.Edges = []DeploymentPlanEdge{
		{From: "api", To: "worker", Condition: PlanEdgeUpstreamSucceeded},
		{From: "worker", To: "api", Condition: PlanEdgeUpstreamSucceeded},
	}
	if _, err := NewDeploymentPlanDocument(document); err != nil {
		t.Fatalf("draft should preserve editable cycle: %v", err)
	}
	if _, err := NewDeploymentPlanEngine(document); err == nil {
		t.Fatal("published plan cycle was accepted")
	}
}

func TestDeploymentPlanConditionUsesStabilizationAndTimeout(t *testing.T) {
	timeout := 60
	document := planDocumentForApplications("api")
	document.Nodes[0].SuccessCondition.TimeoutSeconds = &timeout
	engine := planEngineFixture(t, document)
	result, err := engine.EvaluateCondition("api", DeploymentNodeObservation{
		SyncStatus: "Synced", HealthStatus: "Healthy", StableFor: 20 * time.Second,
		Elapsed: 61 * time.Second,
	})
	if err != nil || result != DeploymentConditionTimedOut {
		t.Fatalf("condition result = %s, error = %v", result, err)
	}
	result, err = engine.EvaluateCondition("api", DeploymentNodeObservation{
		SyncStatus: "Synced", HealthStatus: "Healthy", StableFor: 30 * time.Second,
	})
	if err != nil || result != DeploymentConditionSucceeded {
		t.Fatalf("condition result = %s, error = %v", result, err)
	}
}

func assertReadyKeys(t *testing.T, engine *DeploymentPlanEngine, evaluation DeploymentPlanEvaluation, want []string) {
	t.Helper()
	ready, err := engine.ReadyNodes(evaluation)
	if err != nil {
		t.Fatalf("evaluate ready nodes: %v", err)
	}
	got := make([]string, 0, len(ready))
	for _, node := range ready {
		got = append(got, node.Key)
	}
	if !slicesEqual(got, want) {
		t.Fatalf("ready = %v, want %v", got, want)
	}
}

func planEngineFixture(t *testing.T, document DeploymentPlanDocument) *DeploymentPlanEngine {
	t.Helper()
	engine, err := NewDeploymentPlanEngine(document)
	if err != nil {
		t.Fatalf("create plan engine: %v", err)
	}
	return engine
}

func planDocumentForApplications(keys ...string) DeploymentPlanDocument {
	nodes := make([]DeploymentPlanNode, 0, len(keys))
	for index, key := range keys {
		nodes = append(nodes, DeploymentPlanNode{
			Key: key, ApplicationKey: key, Order: index,
			SuccessCondition: healthyPlanCondition(),
		})
	}
	return DeploymentPlanDocument{Nodes: nodes, Edges: []DeploymentPlanEdge{}}
}

func planDocumentFixture(edges []DeploymentPlanEdge) DeploymentPlanDocument {
	document := planDocumentForApplications("api", "database", "worker")
	document.Edges = edges
	return document
}

func healthyPlanCondition() DeploymentPlanCondition {
	return DeploymentPlanCondition{
		SyncStatuses: []string{"Synced"}, HealthStatuses: []string{"Healthy"}, StabilizationSeconds: 30,
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
