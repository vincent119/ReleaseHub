package domain

import (
	"errors"
	"slices"
)

// WorkflowFact identifies one allowlisted workflow input.
type WorkflowFact string

const (
	WorkflowFactReviewStatus         WorkflowFact = "review.status"
	WorkflowFactDeploymentStatus     WorkflowFact = "deployment.status"
	WorkflowFactDeploymentSyncStatus WorkflowFact = "deployment.syncStatus"
	WorkflowFactDeploymentHealth     WorkflowFact = "deployment.healthStatus"
	WorkflowFactRequestClass         WorkflowFact = "request.classification"
)

// WorkflowOperator identifies one allowlisted comparison.
type WorkflowOperator string

const (
	WorkflowOperatorEquals    WorkflowOperator = "Equals"
	WorkflowOperatorNotEquals WorkflowOperator = "NotEquals"
	WorkflowOperatorIn        WorkflowOperator = "In"
	WorkflowOperatorNotIn     WorkflowOperator = "NotIn"
)

// WorkflowCondition compares one typed fact with immutable values.
type WorkflowCondition struct {
	Fact     WorkflowFact
	Operator WorkflowOperator
	Values   []string
}

// Validate rejects unknown facts, operators, and empty values.
func (c WorkflowCondition) Validate() error {
	if !slices.Contains(validWorkflowFacts(), c.Fact) {
		return errors.New("workflow condition fact is not allowed")
	}
	if !slices.Contains(validWorkflowOperators(), c.Operator) {
		return errors.New("workflow condition operator is not allowed")
	}
	if len(c.Values) == 0 || slices.Contains(c.Values, "") {
		return errors.New("workflow condition requires non-empty values")
	}
	return nil
}

// Matches evaluates the condition against one fact set.
func (c WorkflowCondition) Matches(facts map[WorkflowFact]string) bool {
	actual, exists := facts[c.Fact]
	if !exists {
		return false
	}
	matched := slices.Contains(c.Values, actual)
	switch c.Operator {
	case WorkflowOperatorEquals, WorkflowOperatorIn:
		return matched
	case WorkflowOperatorNotEquals, WorkflowOperatorNotIn:
		return !matched
	default:
		return false
	}
}

func validWorkflowFacts() []WorkflowFact {
	return []WorkflowFact{
		WorkflowFactReviewStatus, WorkflowFactDeploymentStatus,
		WorkflowFactDeploymentSyncStatus, WorkflowFactDeploymentHealth,
		WorkflowFactRequestClass,
	}
}

func validWorkflowOperators() []WorkflowOperator {
	return []WorkflowOperator{
		WorkflowOperatorEquals, WorkflowOperatorNotEquals,
		WorkflowOperatorIn, WorkflowOperatorNotIn,
	}
}
