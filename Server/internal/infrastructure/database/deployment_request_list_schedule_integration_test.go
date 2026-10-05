//go:build integration

package database_test

import (
	"testing"
	"time"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentRequestListPreservesPersistedScheduleProjectionAcrossPages(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	scheduledFor := time.Date(2026, 9, 21, 9, 30, 0, 0, time.UTC)
	seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
		number: 1, updatedAt: fixture.now, status: deploydomain.DeploymentRequestFailed,
	})
	seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
		number: 2, updatedAt: fixture.now, scheduledFor: &scheduledFor,
	})
	limit := 1
	first := fixture.list(t, deployapp.DeploymentRequestListQuery{Limit: &limit})
	if len(first.Items) != 1 || !first.HasMore || first.Items[0].ScheduledFor == nil || !first.Items[0].ScheduledFor.Equal(scheduledFor) ||
		first.Items[0].Schedule.State != deploydomain.DeploymentRequestScheduleWaiting ||
		first.Items[0].Schedule.Reason != deploydomain.DeploymentScheduleWaitingForScheduledTime ||
		!first.Items[0].Schedule.NextEligibleAt.Equal(scheduledFor) {
		t.Fatalf("scheduled first page = %#v", first)
	}
	last := fixture.list(t, deployapp.DeploymentRequestListQuery{Limit: &limit, Cursor: &first.NextCursor})
	if len(last.Items) != 1 || last.HasMore || last.Items[0].ScheduledFor != nil ||
		last.Items[0].Status != deploydomain.DeploymentRequestFailed ||
		last.Items[0].Schedule.State != deploydomain.DeploymentRequestScheduleReady ||
		last.Items[0].Schedule.Reason != deploydomain.DeploymentScheduleReady ||
		!last.Items[0].Schedule.NextEligibleAt.Equal(fixture.now) {
		t.Fatalf("unscheduled last page = %#v", last)
	}

	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: fixture.ids.environmentID, Enabled: true, TimeZone: "UTC", Version: 1,
		WeeklyWindows: []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60}},
	})
	if err != nil {
		t.Fatalf("create schedule policy: %v", err)
	}
	_, err = fixture.schedules.Put(t.Context(), deployapp.DeploymentScheduleMutation{
		ActorID: fixture.ids.userID, OrganizationID: fixture.ids.organizationID,
		RequestID: "request-list-policy", IdempotencyKey: "request-list-policy-1", OccurredAt: fixture.now,
	}, policy)
	if err != nil {
		t.Fatalf("persist schedule policy: %v", err)
	}
	assertCountWhere(t, fixture.db, "deployment_schedule_policies", "environment_id = ?", fixture.ids.environmentID, 1)
	first = fixture.list(t, deployapp.DeploymentRequestListQuery{Limit: &limit})
	if len(first.Items) != 1 || !first.HasMore || !first.Items[0].Schedule.NextEligibleAt.Equal(scheduledFor) ||
		first.Items[0].Schedule.Reason != deploydomain.DeploymentScheduleWaitingForScheduledTime {
		t.Fatalf("policy scheduled page = %#v", first)
	}
	last = fixture.list(t, deployapp.DeploymentRequestListQuery{Limit: &limit, Cursor: &first.NextCursor})
	windowStart := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if len(last.Items) != 1 || last.HasMore || last.NextCursor != "" ||
		last.Items[0].Status != deploydomain.DeploymentRequestFailed ||
		last.Items[0].Schedule.State != deploydomain.DeploymentRequestScheduleWaiting ||
		last.Items[0].Schedule.Reason != deploydomain.DeploymentScheduleWaitingForMaintenanceWindow ||
		!last.Items[0].Schedule.NextEligibleAt.Equal(windowStart) {
		t.Fatalf("policy maintenance-window page = %#v", last)
	}
}
