package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentRequestListProjectsServerScheduleEligibility(t *testing.T) {
	now := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	scheduledFor := now.Add(2 * time.Hour)
	detail := capabilityRequestFixture()
	detail.Summary.ScheduledFor = &scheduledFor
	repository := &capabilityRequestRepository{detail: detail}
	authorizer := &capabilityAuthorizer{allowed: map[authz.Permission]bool{"deployment_request.view": true}}
	service, err := NewDeploymentRequestService(repository, &requestScheduleReader{}, authorizer, scheduleClock{now: now})
	require.NoError(t, err)
	scope, err := authz.NewEnvironmentScope(detail.Summary.OrganizationID, detail.Summary.ProjectID, detail.Summary.EnvironmentID)
	require.NoError(t, err)

	values, err := service.List(context.Background(), RequestPrincipal{UserID: uuid.New()}, scope)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, deploydomain.DeploymentRequestScheduleWaiting, values[0].Schedule.State)
	require.Equal(t, deploydomain.DeploymentScheduleWaitingForScheduledTime, values[0].Schedule.Reason)
	require.Equal(t, scheduledFor, values[0].Schedule.NextEligibleAt)
}

func TestDeploymentRequestDetailProjectsLatestMaintenanceWindow(t *testing.T) {
	now := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	detail := capabilityRequestFixture()
	repository := &capabilityRequestRepository{detail: detail}
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: detail.Summary.EnvironmentID,
		Enabled:       true,
		TimeZone:      "Asia/Taipei",
		WeeklyWindows: []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Tuesday, StartMinute: 10 * 60, EndMinute: 12 * 60}},
		Version:       3,
	})
	require.NoError(t, err)
	authorizer := &capabilityAuthorizer{allowed: map[authz.Permission]bool{"deployment_request.view": true}}
	service, err := NewDeploymentRequestService(repository, &requestScheduleReader{policy: &policy}, authorizer, scheduleClock{now: now})
	require.NoError(t, err)

	result, err := service.Get(context.Background(), RequestPrincipal{UserID: uuid.New()}, detail.Summary.ID)
	require.NoError(t, err)
	require.Equal(t, deploydomain.DeploymentRequestScheduleWaiting, result.Summary.Schedule.State)
	require.Equal(t, deploydomain.DeploymentScheduleWaitingForMaintenanceWindow, result.Summary.Schedule.Reason)
	require.Equal(t, time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC), result.Summary.Schedule.NextEligibleAt)
}
