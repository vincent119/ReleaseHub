package domain

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

var planKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// DeploymentPlanEdgeCondition identifies how one upstream node gates another.
type DeploymentPlanEdgeCondition string

const (
	PlanEdgeUpstreamSucceeded   DeploymentPlanEdgeCondition = "UpstreamSucceeded"
	PlanEdgePrerequisiteHealthy DeploymentPlanEdgeCondition = "PrerequisiteHealthy"
)

// DeploymentPlanCondition defines the observable success contract for one node.
type DeploymentPlanCondition struct {
	SyncStatuses         []string
	HealthStatuses       []string
	StabilizationSeconds int
	TimeoutSeconds       *int
}

// DeploymentPlanNode maps one logical node to an Application key.
type DeploymentPlanNode struct {
	Key              string
	ApplicationKey   string
	Order            int
	SuccessCondition DeploymentPlanCondition
}

// DeploymentPlanEdge defines one directed dependency.
type DeploymentPlanEdge struct {
	From      string
	To        string
	Condition DeploymentPlanEdgeCondition
}

// DeploymentPlanDocument is one versioned deployment graph.
type DeploymentPlanDocument struct {
	Nodes       []DeploymentPlanNode
	Edges       []DeploymentPlanEdge
	MaxParallel *int
}

// NewDeploymentPlanDocument validates and owns one deployment graph snapshot.
func NewDeploymentPlanDocument(value DeploymentPlanDocument) (DeploymentPlanDocument, error) {
	if len(value.Nodes) == 0 {
		return DeploymentPlanDocument{}, errors.New("deployment plan requires at least one node")
	}
	if value.MaxParallel != nil && *value.MaxParallel < 1 {
		return DeploymentPlanDocument{}, errors.New("deployment plan max parallel must be positive")
	}
	validated, keys, applications, err := validatePlanNodes(value.Nodes)
	if err != nil {
		return DeploymentPlanDocument{}, err
	}
	edges, err := validatePlanEdges(value.Edges, keys)
	if err != nil {
		return DeploymentPlanDocument{}, err
	}
	_ = applications
	return DeploymentPlanDocument{Nodes: validated, Edges: edges, MaxParallel: cloneInt(value.MaxParallel)}, nil
}

func validatePlanNodes(values []DeploymentPlanNode) ([]DeploymentPlanNode, map[string]struct{}, map[string]struct{}, error) {
	result := make([]DeploymentPlanNode, 0, len(values))
	keys, applications := map[string]struct{}{}, map[string]struct{}{}
	for _, value := range values {
		node, err := validatePlanNode(value)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, exists := keys[node.Key]; exists {
			return nil, nil, nil, errors.New("deployment plan node key must be unique")
		}
		if _, exists := applications[node.ApplicationKey]; exists {
			return nil, nil, nil, errors.New("deployment plan application key must be unique")
		}
		keys[node.Key], applications[node.ApplicationKey] = struct{}{}, struct{}{}
		result = append(result, node)
	}
	return result, keys, applications, nil
}

func validatePlanNode(value DeploymentPlanNode) (DeploymentPlanNode, error) {
	value.Key = strings.TrimSpace(value.Key)
	value.ApplicationKey = strings.TrimSpace(value.ApplicationKey)
	if !planKeyPattern.MatchString(value.Key) || len(value.Key) > 255 {
		return DeploymentPlanNode{}, errors.New("deployment plan node key is invalid")
	}
	if value.ApplicationKey == "" || len(value.ApplicationKey) > 255 || value.Order < 0 {
		return DeploymentPlanNode{}, errors.New("deployment plan application key or order is invalid")
	}
	condition, err := validatePlanCondition(value.SuccessCondition)
	if err != nil {
		return DeploymentPlanNode{}, err
	}
	value.SuccessCondition = condition
	return value, nil
}

func validatePlanCondition(value DeploymentPlanCondition) (DeploymentPlanCondition, error) {
	syncStatuses, err := normalizePlanStatuses(value.SyncStatuses)
	if err != nil {
		return DeploymentPlanCondition{}, err
	}
	healthStatuses, err := normalizePlanStatuses(value.HealthStatuses)
	if err != nil {
		return DeploymentPlanCondition{}, err
	}
	if value.StabilizationSeconds < 0 || value.TimeoutSeconds != nil && *value.TimeoutSeconds < 1 {
		return DeploymentPlanCondition{}, errors.New("deployment plan timing condition is invalid")
	}
	if value.TimeoutSeconds != nil && *value.TimeoutSeconds < value.StabilizationSeconds {
		return DeploymentPlanCondition{}, errors.New("deployment plan timeout cannot be shorter than stabilization")
	}
	value.SyncStatuses, value.HealthStatuses = syncStatuses, healthStatuses
	value.TimeoutSeconds = cloneInt(value.TimeoutSeconds)
	return value, nil
}

func normalizePlanStatuses(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("deployment plan status condition is required")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			return nil, errors.New("deployment plan status condition is invalid")
		}
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result, nil
}

func validatePlanEdges(values []DeploymentPlanEdge, keys map[string]struct{}) ([]DeploymentPlanEdge, error) {
	result := make([]DeploymentPlanEdge, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		edge, err := validatePlanEdge(value, keys)
		if err != nil {
			return nil, err
		}
		identity := edge.From + "\x00" + edge.To
		if _, exists := seen[identity]; exists {
			return nil, errors.New("deployment plan edge must be unique")
		}
		seen[identity] = struct{}{}
		result = append(result, edge)
	}
	return result, nil
}

func validatePlanEdge(value DeploymentPlanEdge, keys map[string]struct{}) (DeploymentPlanEdge, error) {
	value.From, value.To = strings.TrimSpace(value.From), strings.TrimSpace(value.To)
	_, fromExists := keys[value.From]
	_, toExists := keys[value.To]
	if !fromExists || !toExists || value.From == value.To {
		return DeploymentPlanEdge{}, errors.New("deployment plan edge reference is invalid")
	}
	if value.Condition != PlanEdgeUpstreamSucceeded && value.Condition != PlanEdgePrerequisiteHealthy {
		return DeploymentPlanEdge{}, errors.New("deployment plan edge condition is invalid")
	}
	return value, nil
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
