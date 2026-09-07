package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WorkflowInstanceStatus identifies the lifecycle of one request workflow.
type WorkflowInstanceStatus string

const (
	WorkflowInstanceRunning    WorkflowInstanceStatus = "Running"
	WorkflowInstanceCompleted  WorkflowInstanceStatus = "Completed"
	WorkflowInstanceRejected   WorkflowInstanceStatus = "Rejected"
	WorkflowInstanceBlocked    WorkflowInstanceStatus = "Blocked"
	WorkflowInstanceSuperseded WorkflowInstanceStatus = "Superseded"
)

// WorkflowInstance tracks one request version through its pinned workflow graph.
type WorkflowInstance struct {
	ID                uuid.UUID
	RequestVersionID  uuid.UUID
	WorkflowVersionID uuid.UUID
	CurrentStateKey   string
	Status            WorkflowInstanceStatus
	LockVersion       uint64
	StartedAt         time.Time
	CompletedAt       *time.Time
}

// WorkflowTransitionCommand is one authorized state-change request.
type WorkflowTransitionCommand struct {
	TransitionKey     string
	Trigger           WorkflowTrigger
	Permission        string
	ActorID           uuid.UUID
	PermissionGranted bool
	Reason            string
	Facts             map[WorkflowFact]string
	OccurredAt        time.Time
}

// WorkflowTransitionRecord is append-only transition evidence.
type WorkflowTransitionRecord struct {
	FromStateKey  string
	ToStateKey    string
	TransitionKey string
	ActorID       uuid.UUID
	Reason        string
	OccurredAt    time.Time
}

// WorkflowResult contains the updated instance and emitted intent.
type WorkflowResult struct {
	Instance         WorkflowInstance
	Transition       WorkflowTransitionRecord
	ReviewPolicy     *ReviewPolicy
	DeploymentIntent bool
}

// WorkflowEngine executes one immutable workflow document.
type WorkflowEngine struct {
	document WorkflowDocument
	states   map[string]WorkflowState
}

// AllowsPermission reports whether the current state exposes a condition-matching action.
func (e *WorkflowEngine) AllowsPermission(stateKey, permission string, facts map[WorkflowFact]string) bool {
	for _, transition := range e.document.Transitions {
		if transition.From == stateKey && transition.Permission == permission && conditionsMatch(transition.Conditions, facts) {
			return true
		}
	}
	return false
}

// NewWorkflowEngine creates an engine for one pinned workflow version.
func NewWorkflowEngine(document WorkflowDocument) (*WorkflowEngine, error) {
	validated, err := NewWorkflowDocument(document)
	if err != nil {
		return nil, err
	}
	states := make(map[string]WorkflowState, len(validated.States))
	for _, state := range validated.States {
		states[state.Key] = state
	}
	return &WorkflowEngine{document: validated, states: states}, nil
}

// Start creates a running instance at the document initial state.
func (e *WorkflowEngine) Start(requestVersionID, workflowVersionID uuid.UUID, now time.Time) (WorkflowResult, error) {
	if requestVersionID == uuid.Nil || workflowVersionID == uuid.Nil || now.IsZero() {
		return WorkflowResult{}, errors.New("workflow instance requires request, version, and time")
	}
	instance := WorkflowInstance{
		ID: uuid.New(), RequestVersionID: requestVersionID, WorkflowVersionID: workflowVersionID,
		CurrentStateKey: e.document.InitialState, Status: WorkflowInstanceRunning,
		LockVersion: 1, StartedAt: now.UTC(),
	}
	return e.resultForState(instance, WorkflowTransitionRecord{}), nil
}

// Transition applies one legal graph edge and emits deployment intent only on entry.
func (e *WorkflowEngine) Transition(instance WorkflowInstance, command WorkflowTransitionCommand) (WorkflowResult, error) {
	if instance.Status != WorkflowInstanceRunning {
		return WorkflowResult{}, errors.New("workflow instance is not running")
	}
	transition, err := e.selectTransition(instance.CurrentStateKey, command)
	if err != nil {
		return WorkflowResult{}, err
	}
	record := transitionRecord(transition, command)
	instance.CurrentStateKey = transition.To
	instance.LockVersion++
	return e.resultForState(instance, record), nil
}

// ApplyDeploymentResult advances the unique result edge selected by deployment facts.
func (e *WorkflowEngine) ApplyDeploymentResult(instance WorkflowInstance, status ExecutionStatus, now time.Time) (WorkflowResult, error) {
	facts := map[WorkflowFact]string{WorkflowFactDeploymentStatus: string(status)}
	transition, err := e.selectTriggeredTransition(instance.CurrentStateKey, WorkflowTriggerDeploymentResult, facts)
	if err != nil {
		return WorkflowResult{}, err
	}
	return e.Transition(instance, WorkflowTransitionCommand{
		TransitionKey: transition.Key, Trigger: WorkflowTriggerDeploymentResult,
		PermissionGranted: true, Facts: facts, OccurredAt: now,
	})
}

// ApplyPermissionAction advances the unique manual action exposed for a permission.
func (e *WorkflowEngine) ApplyPermissionAction(instance WorkflowInstance, command WorkflowTransitionCommand) (WorkflowResult, error) {
	transition, err := e.selectPermissionTransition(instance.CurrentStateKey, command.Permission, command.Facts)
	if err != nil {
		return WorkflowResult{}, err
	}
	command.TransitionKey = transition.Key
	command.Trigger = WorkflowTriggerManual
	return e.Transition(instance, command)
}

func (e *WorkflowEngine) selectPermissionTransition(stateKey, permission string, facts map[WorkflowFact]string) (WorkflowTransition, error) {
	var matches []WorkflowTransition
	for _, transition := range e.document.Transitions {
		if transition.From == stateKey && transition.Trigger == WorkflowTriggerManual &&
			transition.Permission == permission && conditionsMatch(transition.Conditions, facts) {
			matches = append(matches, transition)
		}
	}
	if len(matches) != 1 {
		return WorkflowTransition{}, errors.New("workflow action requires exactly one matching transition")
	}
	return matches[0], nil
}

func (e *WorkflowEngine) selectTriggeredTransition(stateKey string, trigger WorkflowTrigger, facts map[WorkflowFact]string) (WorkflowTransition, error) {
	var matches []WorkflowTransition
	for _, transition := range e.document.Transitions {
		if transition.From == stateKey && transition.Trigger == trigger && conditionsMatch(transition.Conditions, facts) {
			matches = append(matches, transition)
		}
	}
	if len(matches) != 1 {
		return WorkflowTransition{}, errors.New("workflow result requires exactly one matching transition")
	}
	return matches[0], nil
}

func (e *WorkflowEngine) selectTransition(stateKey string, command WorkflowTransitionCommand) (WorkflowTransition, error) {
	if command.OccurredAt.IsZero() {
		return WorkflowTransition{}, errors.New("workflow transition time is required")
	}
	for _, transition := range e.document.Transitions {
		if transition.From != stateKey || transition.Key != strings.TrimSpace(command.TransitionKey) {
			continue
		}
		if transition.Trigger != command.Trigger || !command.PermissionGranted {
			return WorkflowTransition{}, errors.New("workflow transition is not authorized")
		}
		if !conditionsMatch(transition.Conditions, command.Facts) {
			return WorkflowTransition{}, errors.New("workflow transition conditions are not satisfied")
		}
		return transition, nil
	}
	return WorkflowTransition{}, errors.New("workflow transition is not available")
}

func (e *WorkflowEngine) resultForState(instance WorkflowInstance, record WorkflowTransitionRecord) WorkflowResult {
	state := e.states[instance.CurrentStateKey]
	result := WorkflowResult{Instance: instance, Transition: record}
	if state.Type == WorkflowStateReview {
		policy := *state.ReviewPolicy
		result.ReviewPolicy = &policy
	}
	result.DeploymentIntent = state.Type == WorkflowStateDeployment
	if state.Type == WorkflowStateTerminal {
		result.Instance.Status = WorkflowInstanceCompleted
		completedAt := completionTime(instance, record)
		result.Instance.CompletedAt = &completedAt
	}
	return result
}

func completionTime(instance WorkflowInstance, record WorkflowTransitionRecord) time.Time {
	if !record.OccurredAt.IsZero() {
		return record.OccurredAt.UTC()
	}
	return instance.StartedAt.UTC()
}

func transitionRecord(transition WorkflowTransition, command WorkflowTransitionCommand) WorkflowTransitionRecord {
	return WorkflowTransitionRecord{
		FromStateKey: transition.From, ToStateKey: transition.To,
		TransitionKey: transition.Key, ActorID: command.ActorID,
		Reason: strings.TrimSpace(command.Reason), OccurredAt: command.OccurredAt.UTC(),
	}
}

func conditionsMatch(conditions []WorkflowCondition, facts map[WorkflowFact]string) bool {
	return !slices.ContainsFunc(conditions, func(condition WorkflowCondition) bool {
		return !condition.Matches(facts)
	})
}
