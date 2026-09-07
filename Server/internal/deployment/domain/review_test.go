package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReviewPolicies(t *testing.T) {
	users := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	roles := []uuid.UUID{uuid.New(), uuid.New()}
	tests := []struct {
		name      string
		policy    ReviewPolicy
		members   map[uuid.UUID][]uuid.UUID
		approvers []uuid.UUID
		approved  bool
	}{
		{name: "any approver", policy: reviewPolicy(ReviewPolicyAny, 1, users, nil), approvers: users[:1], approved: true},
		{name: "minimum approvals pending", policy: reviewPolicy(ReviewPolicyMinimum, 2, users, nil), approvers: users[:1]},
		{name: "minimum approvals", policy: reviewPolicy(ReviewPolicyMinimum, 2, users, nil), approvers: users[:2], approved: true},
		{name: "all approvers", policy: reviewPolicy(ReviewPolicyAll, 1, users, nil), approvers: users, approved: true},
		{name: "each assigned role", policy: reviewPolicy(ReviewPolicyRoleMinimumOne, 1, nil, roles), members: roleMembers(roles, users), approvers: users[:2], approved: true},
		{name: "sequential stage threshold", policy: reviewPolicy(ReviewPolicySequentialStages, 2, users, nil), approvers: users[:2], approved: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := pendingReviewTask(t, test.policy, ReviewAssignment{UserIDs: users, RoleMembers: test.members})
			for _, reviewerID := range test.approvers {
				task = decideReview(t, task, reviewerID, ReviewDecisionApprove, "")
			}
			if got := task.Status == ReviewTaskApproved; got != test.approved {
				t.Fatalf("approved = %t, want %t", got, test.approved)
			}
		})
	}
}

func TestReviewRejectRequiresReason(t *testing.T) {
	reviewerID := uuid.New()
	task := pendingReviewTask(t, reviewPolicy(ReviewPolicyAny, 1, []uuid.UUID{reviewerID}, nil), ReviewAssignment{UserIDs: []uuid.UUID{reviewerID}})
	_, err := task.Decide(reviewDecision(reviewerID, ReviewDecisionReject, ""))
	if err == nil {
		t.Fatal("expected rejection reason error")
	}
	task = decideReview(t, task, reviewerID, ReviewDecisionReject, "unsafe deployment")
	if task.Status != ReviewTaskRejected {
		t.Fatalf("status = %s", task.Status)
	}
}

func TestReviewSelfDecisionFollowsWorkflowPolicy(t *testing.T) {
	creatorID := uuid.New()
	policy := reviewPolicy(ReviewPolicyAny, 1, []uuid.UUID{creatorID}, nil)
	task := pendingReviewTask(t, policy, ReviewAssignment{UserIDs: []uuid.UUID{creatorID}})
	task.RequestCreatorID = creatorID
	if _, err := task.Decide(reviewDecision(creatorID, ReviewDecisionApprove, "")); err == nil {
		t.Fatal("expected self-review rejection")
	}
	policy.AllowSelfReview = true
	task.Policy = policy
	if task = decideReview(t, task, creatorID, ReviewDecisionApprove, ""); task.Status != ReviewTaskApproved {
		t.Fatalf("status = %s", task.Status)
	}
}

func TestReviewReassignmentPreservesExistingDecisions(t *testing.T) {
	first, replacement := uuid.New(), uuid.New()
	task := pendingReviewTask(t, reviewPolicy(ReviewPolicyMinimum, 2, []uuid.UUID{first, uuid.New()}, nil), ReviewAssignment{UserIDs: []uuid.UUID{first}})
	task = decideReview(t, task, first, ReviewDecisionApprove, "")
	policy := reviewPolicy(ReviewPolicyMinimum, 2, []uuid.UUID{replacement}, nil)
	updated, reassignment, err := task.Reassign(ReviewReassignment{
		ActorID: uuid.New(), Policy: policy, Assignment: ReviewAssignment{UserIDs: policy.UserIDs},
		Reason: "original reviewer is unavailable", ReassignedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("reassign review: %v", err)
	}
	if len(updated.Decisions) != 1 || updated.Policy.UserIDs[0] != replacement {
		t.Fatalf("reassignment lost decisions or assignment: %#v", updated)
	}
	if reassignment.Reason != "original reviewer is unavailable" {
		t.Fatalf("reassignment reason = %q", reassignment.Reason)
	}
}

func TestReviewReassignmentRejectsClosedTaskAndBlankReason(t *testing.T) {
	userID := uuid.New()
	policy := reviewPolicy(ReviewPolicyAny, 1, []uuid.UUID{userID}, nil)
	task := pendingReviewTask(t, policy, ReviewAssignment{UserIDs: policy.UserIDs})
	input := ReviewReassignment{
		ActorID: uuid.New(), Policy: policy, Assignment: ReviewAssignment{UserIDs: policy.UserIDs},
		Reason: " ", ReassignedAt: time.Now(),
	}
	if _, _, err := task.Reassign(input); err == nil {
		t.Fatal("expected blank reason rejection")
	}
	task.Status = ReviewTaskClosed
	input.Reason = "review owner changed"
	if _, _, err := task.Reassign(input); err == nil {
		t.Fatal("expected closed task rejection")
	}
}

func reviewPolicy(kind ReviewPolicyType, required int, users, roles []uuid.UUID) ReviewPolicy {
	return ReviewPolicy{Type: kind, RequiredApprovals: required, UserIDs: users, RoleIDs: roles}
}

func roleMembers(roles, users []uuid.UUID) map[uuid.UUID][]uuid.UUID {
	return map[uuid.UUID][]uuid.UUID{roles[0]: {users[0]}, roles[1]: {users[1]}}
}

func pendingReviewTask(t *testing.T, policy ReviewPolicy, assignment ReviewAssignment) ReviewTask {
	t.Helper()
	validated, err := NewReviewPolicy(policy)
	if err != nil {
		t.Fatalf("create review policy: %v", err)
	}
	return ReviewTask{ID: uuid.New(), RequestVersionID: uuid.New(), StateKey: "review", StageNumber: 1, Policy: validated, Assignment: assignment, Status: ReviewTaskPending}
}

func decideReview(t *testing.T, task ReviewTask, reviewerID uuid.UUID, decision ReviewDecisionType, reason string) ReviewTask {
	t.Helper()
	updated, err := task.Decide(reviewDecision(reviewerID, decision, reason))
	if err != nil {
		t.Fatalf("decide review: %v", err)
	}
	return updated
}

func reviewDecision(reviewerID uuid.UUID, decision ReviewDecisionType, reason string) ReviewDecision {
	return ReviewDecision{ReviewerID: reviewerID, Decision: decision, Reason: reason, DecidedAt: time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)}
}
