package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

var workflowKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// WorkflowStateType identifies the responsibility of one workflow state.
type WorkflowStateType string

const (
	WorkflowStateStart        WorkflowStateType = "Start"
	WorkflowStateReview       WorkflowStateType = "Review"
	WorkflowStateManualAction WorkflowStateType = "ManualAction"
	WorkflowStateDeployment   WorkflowStateType = "Deployment"
	WorkflowStateTerminal     WorkflowStateType = "Terminal"
)

// WorkflowTrigger identifies how a transition may be requested.
type WorkflowTrigger string

const (
	WorkflowTriggerAutomatic        WorkflowTrigger = "Automatic"
	WorkflowTriggerManual           WorkflowTrigger = "Manual"
	WorkflowTriggerReviewSatisfied  WorkflowTrigger = "ReviewSatisfied"
	WorkflowTriggerDeploymentResult WorkflowTrigger = "DeploymentResult"
)

// WorkflowState defines one typed state in an immutable workflow document.
type WorkflowState struct {
	Key          string
	Name         string
	Type         WorkflowStateType
	ReviewPolicy *ReviewPolicy
}

// WorkflowTransition defines one directed state change.
type WorkflowTransition struct {
	Key        string
	From       string
	To         string
	Trigger    WorkflowTrigger
	Permission string
	Conditions []WorkflowCondition
}

// WorkflowDocument is the immutable graph stored in a workflow version.
type WorkflowDocument struct {
	InitialState string
	States       []WorkflowState
	Transitions  []WorkflowTransition
}

// NewWorkflowDocument validates a parseable and executable graph structure.
func NewWorkflowDocument(value WorkflowDocument) (WorkflowDocument, error) {
	value.InitialState = strings.TrimSpace(value.InitialState)
	value.States = slices.Clone(value.States)
	value.Transitions = slices.Clone(value.Transitions)
	states, err := validateWorkflowStates(value.States)
	if err != nil {
		return WorkflowDocument{}, err
	}
	if _, exists := states[value.InitialState]; !exists {
		return WorkflowDocument{}, errors.New("workflow initial state does not exist")
	}
	if err := validateWorkflowTransitions(value.Transitions, states); err != nil {
		return WorkflowDocument{}, err
	}
	return value, nil
}

func validateWorkflowStates(values []WorkflowState) (map[string]WorkflowState, error) {
	states := make(map[string]WorkflowState, len(values))
	for index := range values {
		state, err := validateWorkflowState(values[index])
		if err != nil {
			return nil, fmt.Errorf("validate workflow state: %w", err)
		}
		if _, exists := states[state.Key]; exists {
			return nil, fmt.Errorf("workflow state key %q is duplicated", state.Key)
		}
		values[index] = state
		states[state.Key] = state
	}
	return states, nil
}

func validateWorkflowState(value WorkflowState) (WorkflowState, error) {
	value.Key = strings.TrimSpace(value.Key)
	value.Name = strings.TrimSpace(value.Name)
	if !validWorkflowKey(value.Key) || value.Name == "" || len(value.Name) > 128 {
		return WorkflowState{}, errors.New("workflow state key or name is invalid")
	}
	if !validWorkflowStateType(value.Type) {
		return WorkflowState{}, fmt.Errorf("workflow state type %q is invalid", value.Type)
	}
	if value.Type == WorkflowStateReview {
		return validateReviewState(value)
	}
	if value.ReviewPolicy != nil {
		return WorkflowState{}, errors.New("review policy is only allowed on review states")
	}
	return value, nil
}

func validateReviewState(value WorkflowState) (WorkflowState, error) {
	if value.ReviewPolicy == nil {
		return WorkflowState{}, errors.New("review state requires a review policy")
	}
	policy, err := NewReviewPolicy(*value.ReviewPolicy)
	if err != nil {
		return WorkflowState{}, err
	}
	value.ReviewPolicy = &policy
	return value, nil
}

func validateWorkflowTransitions(values []WorkflowTransition, states map[string]WorkflowState) error {
	keys := make(map[string]struct{}, len(values))
	for index := range values {
		value, err := validateWorkflowTransition(values[index], states)
		if err != nil {
			return fmt.Errorf("validate workflow transition: %w", err)
		}
		if _, exists := keys[value.Key]; exists {
			return fmt.Errorf("workflow transition key %q is duplicated", value.Key)
		}
		values[index] = value
		keys[value.Key] = struct{}{}
	}
	return nil
}

func validateWorkflowTransition(value WorkflowTransition, states map[string]WorkflowState) (WorkflowTransition, error) {
	value = normalizeWorkflowTransition(value)
	if !validWorkflowKey(value.Key) || !validPermission(value.Permission) {
		return WorkflowTransition{}, errors.New("workflow transition key or permission is invalid")
	}
	if err := validateTransitionStates(value, states); err != nil {
		return WorkflowTransition{}, err
	}
	if !validWorkflowTrigger(value.Trigger) {
		return WorkflowTransition{}, fmt.Errorf("workflow transition trigger %q is invalid", value.Trigger)
	}
	for _, condition := range value.Conditions {
		if err := condition.Validate(); err != nil {
			return WorkflowTransition{}, err
		}
	}
	return value, nil
}

func normalizeWorkflowTransition(value WorkflowTransition) WorkflowTransition {
	value.Key = strings.TrimSpace(value.Key)
	value.From = strings.TrimSpace(value.From)
	value.To = strings.TrimSpace(value.To)
	value.Permission = strings.TrimSpace(value.Permission)
	value.Conditions = slices.Clone(value.Conditions)
	for index := range value.Conditions {
		value.Conditions[index].Values = slices.Clone(value.Conditions[index].Values)
	}
	return value
}

func validateTransitionStates(value WorkflowTransition, states map[string]WorkflowState) error {
	if _, exists := states[value.From]; !exists {
		return fmt.Errorf("workflow transition source %q does not exist", value.From)
	}
	if _, exists := states[value.To]; !exists {
		return fmt.Errorf("workflow transition target %q does not exist", value.To)
	}
	return nil
}

func validWorkflowKey(value string) bool {
	return len(value) <= 128 && workflowKeyPattern.MatchString(value)
}

func validPermission(value string) bool {
	parts := strings.Split(value, ".")
	return len(parts) >= 2 && !slices.Contains(parts, "")
}

func validWorkflowStateType(value WorkflowStateType) bool {
	return slices.Contains([]WorkflowStateType{
		WorkflowStateStart, WorkflowStateReview, WorkflowStateManualAction,
		WorkflowStateDeployment, WorkflowStateTerminal,
	}, value)
}

func validWorkflowTrigger(value WorkflowTrigger) bool {
	return slices.Contains([]WorkflowTrigger{
		WorkflowTriggerAutomatic, WorkflowTriggerManual,
		WorkflowTriggerReviewSatisfied, WorkflowTriggerDeploymentResult,
	}, value)
}

func validUUIDs(values []uuid.UUID) bool {
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(seen) == len(values)
}
