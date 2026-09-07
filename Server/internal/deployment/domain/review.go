package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReviewPolicyType identifies how approvals satisfy a review task.
type ReviewPolicyType string

const (
	ReviewPolicyAny              ReviewPolicyType = "AnyApprover"
	ReviewPolicyMinimum          ReviewPolicyType = "MinimumApprovals"
	ReviewPolicyAll              ReviewPolicyType = "AllApprovers"
	ReviewPolicyRoleMinimumOne   ReviewPolicyType = "RoleMinimumOne"
	ReviewPolicySequentialStages ReviewPolicyType = "SequentialStages"
)

// ReviewPolicy defines immutable reviewer selection and approval rules.
type ReviewPolicy struct {
	Type              ReviewPolicyType
	RequiredApprovals int
	AllowSelfReview   bool
	UserIDs           []uuid.UUID
	RoleIDs           []uuid.UUID
}

// NewReviewPolicy validates and copies a review policy.
func NewReviewPolicy(value ReviewPolicy) (ReviewPolicy, error) {
	if !slices.Contains(validReviewPolicyTypes(), value.Type) {
		return ReviewPolicy{}, errors.New("review policy type is invalid")
	}
	if value.RequiredApprovals < 1 || !validUUIDs(value.UserIDs) || !validUUIDs(value.RoleIDs) {
		return ReviewPolicy{}, errors.New("review policy approvals or assignments are invalid")
	}
	if len(value.UserIDs) == 0 && len(value.RoleIDs) == 0 {
		return ReviewPolicy{}, errors.New("review policy requires assigned users or roles")
	}
	if value.Type == ReviewPolicyRoleMinimumOne && len(value.RoleIDs) == 0 {
		return ReviewPolicy{}, errors.New("role review policy requires roles")
	}
	value.UserIDs = slices.Clone(value.UserIDs)
	value.RoleIDs = slices.Clone(value.RoleIDs)
	return value, nil
}

// ReviewAssignment is the immutable user and role membership snapshot.
type ReviewAssignment struct {
	UserIDs     []uuid.UUID
	RoleMembers map[uuid.UUID][]uuid.UUID
}

// ReviewDecisionType identifies an append-only reviewer decision.
type ReviewDecisionType string

const (
	ReviewDecisionApprove ReviewDecisionType = "Approve"
	ReviewDecisionReject  ReviewDecisionType = "Reject"
)

// ReviewDecision records one immutable reviewer decision.
type ReviewDecision struct {
	ReviewerID uuid.UUID
	Decision   ReviewDecisionType
	Reason     string
	DecidedAt  time.Time
}

// ReviewReassignment records one authorized change to a pending assignment.
type ReviewReassignment struct {
	ActorID      uuid.UUID
	Policy       ReviewPolicy
	Assignment   ReviewAssignment
	Reason       string
	ReassignedAt time.Time
}

// ReviewTaskStatus identifies the current review result.
type ReviewTaskStatus string

const (
	ReviewTaskPending              ReviewTaskStatus = "Pending"
	ReviewTaskApproved             ReviewTaskStatus = "Approved"
	ReviewTaskRejected             ReviewTaskStatus = "Rejected"
	ReviewTaskReassignmentRequired ReviewTaskStatus = "ReassignmentRequired"
	ReviewTaskClosed               ReviewTaskStatus = "Closed"
)

// ReviewTask evaluates decisions against an immutable assignment snapshot.
type ReviewTask struct {
	ID               uuid.UUID
	RequestVersionID uuid.UUID
	StateKey         string
	StageNumber      int
	RequestCreatorID uuid.UUID
	Policy           ReviewPolicy
	Assignment       ReviewAssignment
	Decisions        []ReviewDecision
	Status           ReviewTaskStatus
}

// ReviewTaskDraft contains immutable inputs captured on review entry.
type ReviewTaskDraft struct {
	RequestVersionID uuid.UUID
	StateKey         string
	StageNumber      int
	RequestCreatorID uuid.UUID
	Policy           ReviewPolicy
	Assignment       ReviewAssignment
}

// NewReviewTask creates a pending task with a copied assignment snapshot.
func NewReviewTask(draft ReviewTaskDraft) (ReviewTask, error) {
	policy, err := NewReviewPolicy(draft.Policy)
	if err != nil {
		return ReviewTask{}, err
	}
	if draft.RequestVersionID == uuid.Nil || strings.TrimSpace(draft.StateKey) == "" || draft.StageNumber < 1 {
		return ReviewTask{}, errors.New("review task identity, state, and stage are required")
	}
	return ReviewTask{
		ID: uuid.New(), RequestVersionID: draft.RequestVersionID,
		StateKey: strings.TrimSpace(draft.StateKey), StageNumber: draft.StageNumber,
		RequestCreatorID: draft.RequestCreatorID, Policy: policy,
		Assignment: cloneReviewAssignment(draft.Assignment), Status: ReviewTaskPending,
	}, nil
}

func cloneReviewAssignment(value ReviewAssignment) ReviewAssignment {
	result := ReviewAssignment{UserIDs: slices.Clone(value.UserIDs), RoleMembers: make(map[uuid.UUID][]uuid.UUID, len(value.RoleMembers))}
	for roleID, members := range value.RoleMembers {
		result.RoleMembers[roleID] = slices.Clone(members)
	}
	return result
}

// Decide appends one decision and recalculates the task status.
func (t ReviewTask) Decide(value ReviewDecision) (ReviewTask, error) {
	if t.Status != ReviewTaskPending && t.Status != ReviewTaskReassignmentRequired {
		return ReviewTask{}, errors.New("review task is not pending")
	}
	if err := t.validateDecision(value); err != nil {
		return ReviewTask{}, err
	}
	t.Decisions = append(slices.Clone(t.Decisions), normalizedDecision(value))
	t.Status = t.nextStatus()
	return t, nil
}

// Reassign replaces eligible reviewers while preserving append-only decisions.
func (t ReviewTask) Reassign(value ReviewReassignment) (ReviewTask, ReviewReassignment, error) {
	if t.Status != ReviewTaskPending && t.Status != ReviewTaskReassignmentRequired {
		return ReviewTask{}, ReviewReassignment{}, errors.New("review task is not pending")
	}
	policy, err := NewReviewPolicy(value.Policy)
	if err != nil {
		return ReviewTask{}, ReviewReassignment{}, err
	}
	if value.ActorID == uuid.Nil || value.ReassignedAt.IsZero() || strings.TrimSpace(value.Reason) == "" {
		return ReviewTask{}, ReviewReassignment{}, errors.New("review reassignment requires actor, reason, and time")
	}
	t.Policy = policy
	t.Assignment = cloneReviewAssignment(value.Assignment)
	t.Status = ReviewTaskPending
	value.Policy = policy
	value.Assignment = cloneReviewAssignment(value.Assignment)
	value.Reason = strings.TrimSpace(value.Reason)
	value.ReassignedAt = value.ReassignedAt.UTC()
	return t, value, nil
}

func (t ReviewTask) validateDecision(value ReviewDecision) error {
	if err := validateReviewDecision(value); err != nil {
		return err
	}
	if slices.ContainsFunc(t.Decisions, func(existing ReviewDecision) bool { return existing.ReviewerID == value.ReviewerID }) {
		return errors.New("reviewer already decided this task")
	}
	if !t.Policy.AllowSelfReview && value.ReviewerID == t.RequestCreatorID {
		return errors.New("request creator cannot review this task")
	}
	if !t.Assignment.eligible(value.ReviewerID, t.Policy) {
		return errors.New("reviewer is not assigned to this task")
	}
	return nil
}

func validateReviewDecision(value ReviewDecision) error {
	if value.ReviewerID == uuid.Nil || value.DecidedAt.IsZero() {
		return errors.New("review decision requires reviewer and time")
	}
	if value.Decision != ReviewDecisionApprove && value.Decision != ReviewDecisionReject {
		return errors.New("review decision is invalid")
	}
	if value.Decision == ReviewDecisionReject && strings.TrimSpace(value.Reason) == "" {
		return errors.New("review rejection requires a reason")
	}
	return nil
}

func normalizedDecision(value ReviewDecision) ReviewDecision {
	value.Reason = strings.TrimSpace(value.Reason)
	value.DecidedAt = value.DecidedAt.UTC()
	return value
}

func (t ReviewTask) nextStatus() ReviewTaskStatus {
	if slices.ContainsFunc(t.Decisions, func(value ReviewDecision) bool { return value.Decision == ReviewDecisionReject }) {
		return ReviewTaskRejected
	}
	if t.approvalsSatisfied() {
		return ReviewTaskApproved
	}
	return ReviewTaskPending
}

func (t ReviewTask) approvalsSatisfied() bool {
	approvers := t.approvers()
	switch t.Policy.Type {
	case ReviewPolicyAny:
		return len(approvers) >= 1
	case ReviewPolicyMinimum, ReviewPolicySequentialStages:
		return len(approvers) >= t.Policy.RequiredApprovals
	case ReviewPolicyAll:
		eligible := t.Assignment.eligibleUsers(t.Policy)
		return len(eligible) > 0 && len(approvers) >= len(eligible)
	case ReviewPolicyRoleMinimumOne:
		return t.Assignment.everyRoleApproved(t.Policy.RoleIDs, approvers)
	default:
		return false
	}
}

func (t ReviewTask) approvers() []uuid.UUID {
	result := make([]uuid.UUID, 0, len(t.Decisions))
	for _, value := range t.Decisions {
		if value.Decision == ReviewDecisionApprove {
			result = append(result, value.ReviewerID)
		}
	}
	return result
}

func (a ReviewAssignment) eligible(userID uuid.UUID, policy ReviewPolicy) bool {
	return slices.Contains(a.eligibleUsers(policy), userID)
}

func (a ReviewAssignment) eligibleUsers(policy ReviewPolicy) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	for _, userID := range policy.UserIDs {
		seen[userID] = struct{}{}
	}
	for _, roleID := range policy.RoleIDs {
		for _, userID := range a.RoleMembers[roleID] {
			seen[userID] = struct{}{}
		}
	}
	result := make([]uuid.UUID, 0, len(seen))
	for userID := range seen {
		result = append(result, userID)
	}
	return result
}

func (a ReviewAssignment) everyRoleApproved(roleIDs, approvers []uuid.UUID) bool {
	for _, roleID := range roleIDs {
		if !slices.ContainsFunc(a.RoleMembers[roleID], func(userID uuid.UUID) bool { return slices.Contains(approvers, userID) }) {
			return false
		}
	}
	return len(roleIDs) > 0
}

func validReviewPolicyTypes() []ReviewPolicyType {
	return []ReviewPolicyType{
		ReviewPolicyAny, ReviewPolicyMinimum, ReviewPolicyAll,
		ReviewPolicyRoleMinimumOne, ReviewPolicySequentialStages,
	}
}
