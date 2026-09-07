package domain

import (
	"testing"
	"time"
)

func TestDeploymentRequestVersionSupersedeEligibility(t *testing.T) {
	tests := []struct {
		status DeploymentRequestStatus
		want   bool
	}{
		{DeploymentRequestCandidate, true},
		{DeploymentRequestPendingReview, true},
		{DeploymentRequestApproved, true},
		{DeploymentRequestDeploying, false},
		{DeploymentRequestSucceeded, false},
		{DeploymentRequestSuperseded, false},
	}
	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			if got := (DeploymentRequestVersionSummary{Status: test.status}).CanBeSuperseded(); got != test.want {
				t.Fatalf("supersede eligibility = %t; want %t", got, test.want)
			}
		})
	}
}

func TestDeploymentRequestMetadataNormalizesSchedule(t *testing.T) {
	input := time.Date(2026, 9, 3, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	value, err := NewDeploymentRequestMetadata(DeploymentRequestMetadata{
		ChangeDescription: "  release note  ", IssueURL: " https://example.test/change/1 ", ScheduledFor: &input,
	})
	if err != nil {
		t.Fatalf("normalize metadata: %v", err)
	}
	if value.ChangeDescription != "release note" || value.IssueURL != "https://example.test/change/1" || value.ScheduledFor.Location() != time.UTC {
		t.Fatalf("normalized metadata = %#v", value)
	}
}
