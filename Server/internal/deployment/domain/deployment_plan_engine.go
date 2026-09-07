package domain

import (
	"errors"
	"slices"
	"sort"
	"time"
)

// DeploymentNodeStatus identifies one node's execution state.
type DeploymentNodeStatus string

const (
	DeploymentNodeQueued    DeploymentNodeStatus = "Queued"
	DeploymentNodeRunning   DeploymentNodeStatus = "Running"
	DeploymentNodeSucceeded DeploymentNodeStatus = "Succeeded"
	DeploymentNodeFailed    DeploymentNodeStatus = "Failed"
	DeploymentNodeBlocked   DeploymentNodeStatus = "Blocked"
	DeploymentNodeSkipped   DeploymentNodeStatus = "Skipped"
)

// DeploymentNodeObservation contains typed Argo CD facts used by Plan conditions.
type DeploymentNodeObservation struct {
	SyncStatus   string
	HealthStatus string
	StableFor    time.Duration
	Elapsed      time.Duration
}

// DeploymentConditionResult identifies whether a node condition is still pending or final.
type DeploymentConditionResult string

const (
	DeploymentConditionPending   DeploymentConditionResult = "Pending"
	DeploymentConditionSucceeded DeploymentConditionResult = "Succeeded"
	DeploymentConditionTimedOut  DeploymentConditionResult = "TimedOut"
)

// DeploymentNodeRuntime contains the current state of one included node.
type DeploymentNodeRuntime struct {
	Status      DeploymentNodeStatus
	Observation DeploymentNodeObservation
}

// DeploymentPlanEvaluation contains one immutable execution snapshot and current facts.
type DeploymentPlanEvaluation struct {
	ApplicationKeys []string
	Nodes           map[string]DeploymentNodeRuntime
	Prerequisites   map[string]DeploymentNodeObservation
	MaxParallel     int
}

// DeploymentPlanResolution separates mapped nodes from independently blocked Applications.
type DeploymentPlanResolution struct {
	Nodes                  []DeploymentPlanNode
	MissingApplicationKeys []string
}

// DeploymentPlanEngine evaluates one immutable acyclic Plan Version.
type DeploymentPlanEngine struct {
	document      DeploymentPlanDocument
	nodes         map[string]DeploymentPlanNode
	byApplication map[string]DeploymentPlanNode
	incoming      map[string][]DeploymentPlanEdge
}

// NewDeploymentPlanEngine creates an executable Plan after DAG validation.
func NewDeploymentPlanEngine(document DeploymentPlanDocument) (*DeploymentPlanEngine, error) {
	validated, err := NewDeploymentPlanDocument(document)
	if err != nil {
		return nil, err
	}
	if hasPlanCycle(validated) {
		return nil, errors.New("deployment plan cannot contain a cycle")
	}
	engine := &DeploymentPlanEngine{document: validated}
	engine.indexDocument()
	return engine, nil
}

// Snapshot resolves logical Application keys to ordered Plan nodes.
func (e *DeploymentPlanEngine) Snapshot(applicationKeys []string) ([]DeploymentPlanNode, error) {
	resolution, err := e.ResolveSnapshot(applicationKeys)
	if err != nil {
		return nil, err
	}
	if len(resolution.MissingApplicationKeys) > 0 {
		return nil, errors.New("application key is not defined by deployment plan")
	}
	return resolution.Nodes, nil
}

// ResolveSnapshot preserves valid Applications when unrelated keys have no Plan mapping.
func (e *DeploymentPlanEngine) ResolveSnapshot(applicationKeys []string) (DeploymentPlanResolution, error) {
	result := DeploymentPlanResolution{Nodes: make([]DeploymentPlanNode, 0, len(applicationKeys))}
	seen := map[string]struct{}{}
	for _, applicationKey := range applicationKeys {
		if _, duplicate := seen[applicationKey]; duplicate {
			return DeploymentPlanResolution{}, errors.New("application key appears more than once in execution snapshot")
		}
		seen[applicationKey] = struct{}{}
		node, exists := e.byApplication[applicationKey]
		if exists {
			result.Nodes = append(result.Nodes, node)
		} else {
			result.MissingApplicationKeys = append(result.MissingApplicationKeys, applicationKey)
		}
	}
	sortPlanNodes(result.Nodes)
	slices.Sort(result.MissingApplicationKeys)
	return result, nil
}

// ReadyNodes returns ordered nodes whose explicit dependencies are satisfied.
func (e *DeploymentPlanEngine) ReadyNodes(value DeploymentPlanEvaluation) ([]DeploymentPlanNode, error) {
	active, err := e.activeNodes(value.ApplicationKeys)
	if err != nil {
		return nil, err
	}
	ready := make([]DeploymentPlanNode, 0, len(active))
	for key, node := range active {
		if e.nodeReady(key, active, value) {
			ready = append(ready, node)
		}
	}
	sortPlanNodes(ready)
	return limitReadyNodes(ready, e.availableSlots(active, value)), nil
}

// ConditionSatisfied evaluates a node's typed technical success condition.
func (e *DeploymentPlanEngine) ConditionSatisfied(nodeKey string, value DeploymentNodeObservation) (bool, error) {
	result, err := e.EvaluateCondition(nodeKey, value)
	return result == DeploymentConditionSucceeded, err
}

// EvaluateCondition applies status, stabilization, and optional timeout rules.
func (e *DeploymentPlanEngine) EvaluateCondition(nodeKey string, value DeploymentNodeObservation) (DeploymentConditionResult, error) {
	node, exists := e.nodes[nodeKey]
	if !exists {
		return "", errors.New("deployment plan node was not found")
	}
	if planConditionSatisfied(node.SuccessCondition, value) {
		return DeploymentConditionSucceeded, nil
	}
	if planConditionTimedOut(node.SuccessCondition, value) {
		return DeploymentConditionTimedOut, nil
	}
	return DeploymentConditionPending, nil
}

func (e *DeploymentPlanEngine) indexDocument() {
	e.nodes = make(map[string]DeploymentPlanNode, len(e.document.Nodes))
	e.byApplication = make(map[string]DeploymentPlanNode, len(e.document.Nodes))
	e.incoming = make(map[string][]DeploymentPlanEdge, len(e.document.Nodes))
	for _, node := range e.document.Nodes {
		e.nodes[node.Key], e.byApplication[node.ApplicationKey] = node, node
	}
	for _, edge := range e.document.Edges {
		e.incoming[edge.To] = append(e.incoming[edge.To], edge)
	}
}

func (e *DeploymentPlanEngine) activeNodes(applicationKeys []string) (map[string]DeploymentPlanNode, error) {
	snapshot, err := e.Snapshot(applicationKeys)
	if err != nil {
		return nil, err
	}
	result := make(map[string]DeploymentPlanNode, len(snapshot))
	for _, node := range snapshot {
		result[node.Key] = node
	}
	return result, nil
}

func (e *DeploymentPlanEngine) nodeReady(key string, active map[string]DeploymentPlanNode, value DeploymentPlanEvaluation) bool {
	runtime := value.Nodes[key]
	if runtime.Status != "" && runtime.Status != DeploymentNodeQueued {
		return false
	}
	for _, edge := range e.incoming[key] {
		if !e.edgeSatisfied(edge, active, value) {
			return false
		}
	}
	return true
}

func (e *DeploymentPlanEngine) edgeSatisfied(edge DeploymentPlanEdge, active map[string]DeploymentPlanNode, value DeploymentPlanEvaluation) bool {
	upstream := e.nodes[edge.From]
	if _, included := active[edge.From]; included {
		return value.Nodes[edge.From].Status == DeploymentNodeSucceeded
	}
	if edge.Condition == PlanEdgeUpstreamSucceeded {
		return true
	}
	observation, exists := value.Prerequisites[upstream.ApplicationKey]
	return exists && planConditionSatisfied(upstream.SuccessCondition, observation)
}

func (e *DeploymentPlanEngine) availableSlots(active map[string]DeploymentPlanNode, value DeploymentPlanEvaluation) int {
	running := 0
	for key := range active {
		if value.Nodes[key].Status == DeploymentNodeRunning {
			running++
		}
	}
	limit := value.MaxParallel
	if e.document.MaxParallel != nil && (limit == 0 || *e.document.MaxParallel < limit) {
		limit = *e.document.MaxParallel
	}
	if limit == 0 {
		return len(e.document.Nodes)
	}
	return max(limit-running, 0)
}

func planConditionSatisfied(condition DeploymentPlanCondition, value DeploymentNodeObservation) bool {
	stable := value.StableFor >= time.Duration(condition.StabilizationSeconds)*time.Second
	return slices.Contains(condition.SyncStatuses, value.SyncStatus) &&
		slices.Contains(condition.HealthStatuses, value.HealthStatus) && stable
}

func planConditionTimedOut(condition DeploymentPlanCondition, value DeploymentNodeObservation) bool {
	return condition.TimeoutSeconds != nil &&
		value.Elapsed >= time.Duration(*condition.TimeoutSeconds)*time.Second
}

func limitReadyNodes(values []DeploymentPlanNode, limit int) []DeploymentPlanNode {
	if limit >= len(values) {
		return values
	}
	return values[:limit]
}

func sortPlanNodes(values []DeploymentPlanNode) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].Order != values[right].Order {
			return values[left].Order < values[right].Order
		}
		return values[left].Key < values[right].Key
	})
}

func hasPlanCycle(document DeploymentPlanDocument) bool {
	indegree, adjacent := planGraph(document)
	queue := zeroIndegreeNodes(indegree)
	visited := visitPlanGraph(queue, indegree, adjacent)
	return visited != len(document.Nodes)
}

func planGraph(document DeploymentPlanDocument) (map[string]int, map[string][]string) {
	indegree := make(map[string]int, len(document.Nodes))
	adjacent := make(map[string][]string, len(document.Nodes))
	for _, node := range document.Nodes {
		indegree[node.Key] = 0
	}
	for _, edge := range document.Edges {
		indegree[edge.To]++
		adjacent[edge.From] = append(adjacent[edge.From], edge.To)
	}
	return indegree, adjacent
}

func zeroIndegreeNodes(indegree map[string]int) []string {
	queue := make([]string, 0, len(indegree))
	for key, degree := range indegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	return queue
}

func visitPlanGraph(queue []string, indegree map[string]int, adjacent map[string][]string) int {
	visited := 0
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adjacent[key] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	return visited
}
