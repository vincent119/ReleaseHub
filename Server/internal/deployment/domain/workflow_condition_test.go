package domain

import "testing"

func TestWorkflowConditionDoesNotMatchMissingFact(t *testing.T) {
	condition := WorkflowCondition{
		Fact: WorkflowFactReviewStatus, Operator: WorkflowOperatorNotEquals,
		Values: []string{"Rejected"},
	}
	if condition.Matches(map[WorkflowFact]string{}) {
		t.Fatal("missing workflow fact satisfied a negative condition")
	}
}
