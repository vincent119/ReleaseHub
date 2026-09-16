package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewDeploymentSchedulePolicyValidatesDocument(t *testing.T) {
	validWindow := DeploymentScheduleWeeklyWindow{
		DayOfWeek:   time.Monday,
		StartMinute: 9 * 60,
		EndMinute:   17 * 60,
	}
	validBlackout := DeploymentScheduleBlackout{
		StartsAt: mustTime(t, "2026-09-21T02:00:00+08:00"),
		EndsAt:   mustTime(t, "2026-09-21T03:00:00+08:00"),
	}

	tests := []struct {
		name  string
		draft DeploymentSchedulePolicyDraft
	}{
		{
			name:  "missing environment",
			draft: DeploymentSchedulePolicyDraft{Enabled: true, TimeZone: "Asia/Taipei", WeeklyWindows: []DeploymentScheduleWeeklyWindow{validWindow}},
		},
		{
			name:  "invalid timezone",
			draft: DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New(), Enabled: true, TimeZone: "UTC+8", WeeklyWindows: []DeploymentScheduleWeeklyWindow{validWindow}},
		},
		{
			name:  "enabled without weekly window",
			draft: DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New(), Enabled: true, TimeZone: "Asia/Taipei"},
		},
		{
			name:  "window crosses midnight",
			draft: DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New(), Enabled: true, TimeZone: "Asia/Taipei", WeeklyWindows: []DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: 22 * 60, EndMinute: 2 * 60}}},
		},
		{
			name: "overlapping windows",
			draft: DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New(), Enabled: true, TimeZone: "Asia/Taipei", WeeklyWindows: []DeploymentScheduleWeeklyWindow{
				validWindow,
				{DayOfWeek: time.Monday, StartMinute: 16 * 60, EndMinute: 18 * 60},
			}},
		},
		{
			name: "invalid blackout",
			draft: DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New(), Enabled: true, TimeZone: "Asia/Taipei", WeeklyWindows: []DeploymentScheduleWeeklyWindow{validWindow}, Blackouts: []DeploymentScheduleBlackout{{
				StartsAt: validBlackout.EndsAt,
				EndsAt:   validBlackout.StartsAt,
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewDeploymentSchedulePolicy(tt.draft); !errors.Is(err, ErrInvalidDeploymentSchedule) {
				t.Fatalf("expected invalid schedule error, got %v", err)
			}
		})
	}

	policy, err := NewDeploymentSchedulePolicy(DeploymentSchedulePolicyDraft{
		EnvironmentID: uuid.New(),
		Enabled:       true,
		TimeZone:      " Asia/Taipei ",
		WeeklyWindows: []DeploymentScheduleWeeklyWindow{validWindow},
		Blackouts:     []DeploymentScheduleBlackout{validBlackout},
		Version:       2,
	})
	if err != nil {
		t.Fatalf("create schedule policy: %v", err)
	}
	if policy.TimeZone != "Asia/Taipei" || policy.Blackouts[0].StartsAt.Location() != time.UTC || policy.Version != 2 {
		t.Fatalf("policy was not normalized: %#v", policy)
	}
}

func TestCalculateDeploymentEligibility(t *testing.T) {
	now := mustTime(t, "2026-09-21T00:00:00Z") // Monday 08:00 in Taipei.
	scheduled := mustTime(t, "2026-09-21T02:30:00Z")
	policy := mustSchedulePolicy(t, "Asia/Taipei", []DeploymentScheduleWeeklyWindow{{
		DayOfWeek:   time.Monday,
		StartMinute: 9 * 60,
		EndMinute:   17 * 60,
	}}, nil)

	tests := []struct {
		name         string
		policy       *DeploymentSchedulePolicy
		scheduledFor *time.Time
		wantAt       string
		wantNow      bool
		wantReason   DeploymentScheduleReason
	}{
		{name: "empty policy is unrestricted", wantAt: "2026-09-21T00:00:00Z", wantNow: true, wantReason: DeploymentScheduleReady},
		{name: "disabled policy is unrestricted", policy: disabledSchedulePolicy(t), wantAt: "2026-09-21T00:00:00Z", wantNow: true, wantReason: DeploymentScheduleReady},
		{name: "scheduled time is earliest start", scheduledFor: &scheduled, wantAt: "2026-09-21T02:30:00Z", wantReason: DeploymentScheduleWaitingForScheduledTime},
		{name: "window delays start", policy: &policy, wantAt: "2026-09-21T01:00:00Z", wantReason: DeploymentScheduleWaitingForMaintenanceWindow},
		{name: "scheduled time inside window", policy: &policy, scheduledFor: &scheduled, wantAt: "2026-09-21T02:30:00Z", wantReason: DeploymentScheduleWaitingForScheduledTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, err := CalculateDeploymentEligibility(now, tt.scheduledFor, tt.policy)
			if err != nil {
				t.Fatalf("calculate eligibility: %v", err)
			}
			wantAt := mustTime(t, tt.wantAt)
			if !eligibility.NextEligibleAt.Equal(wantAt) || eligibility.EligibleNow != tt.wantNow || eligibility.Reason != tt.wantReason {
				t.Fatalf("unexpected eligibility: %#v", eligibility)
			}
		})
	}
}

func TestCalculateDeploymentEligibilityDoesNotCarryBlackoutSecondsIntoNextWindow(t *testing.T) {
	policy := mustSchedulePolicy(t, "UTC", []DeploymentScheduleWeeklyWindow{{
		DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60,
	}}, []DeploymentScheduleBlackout{{
		StartsAt: mustTime(t, "2026-09-14T09:00:00Z"),
		EndsAt:   mustTime(t, "2026-09-14T10:00:30Z"),
	}})

	eligibility, err := CalculateDeploymentEligibility(mustTime(t, "2026-09-14T09:30:00Z"), nil, &policy)
	if err != nil {
		t.Fatalf("calculate eligibility: %v", err)
	}
	if !eligibility.NextEligibleAt.Equal(mustTime(t, "2026-09-21T09:00:00Z")) {
		t.Fatalf("unexpected eligibility: %#v", eligibility)
	}
}

func TestCalculateDeploymentEligibilityHonorsWindowBoundaryAndBlackout(t *testing.T) {
	policy := mustSchedulePolicy(t, "UTC", []DeploymentScheduleWeeklyWindow{{
		DayOfWeek:   time.Monday,
		StartMinute: 9 * 60,
		EndMinute:   10 * 60,
	}}, []DeploymentScheduleBlackout{{
		StartsAt: mustTime(t, "2026-09-21T09:15:00Z"),
		EndsAt:   mustTime(t, "2026-09-21T09:45:00Z"),
	}})

	tests := []struct {
		name       string
		now        string
		wantAt     string
		wantNow    bool
		wantReason DeploymentScheduleReason
	}{
		{name: "start boundary is inclusive", now: "2026-09-21T09:00:00Z", wantAt: "2026-09-21T09:00:00Z", wantNow: true, wantReason: DeploymentScheduleReady},
		{name: "blackout start is inclusive", now: "2026-09-21T09:15:00Z", wantAt: "2026-09-21T09:45:00Z", wantReason: DeploymentScheduleWaitingForBlackout},
		{name: "blackout end is exclusive", now: "2026-09-21T09:45:00Z", wantAt: "2026-09-21T09:45:00Z", wantNow: true, wantReason: DeploymentScheduleReady},
		{name: "window end is exclusive", now: "2026-09-21T10:00:00Z", wantAt: "2026-09-28T09:00:00Z", wantReason: DeploymentScheduleWaitingForMaintenanceWindow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, err := CalculateDeploymentEligibility(mustTime(t, tt.now), nil, &policy)
			if err != nil {
				t.Fatalf("calculate eligibility: %v", err)
			}
			if !eligibility.NextEligibleAt.Equal(mustTime(t, tt.wantAt)) || eligibility.EligibleNow != tt.wantNow || eligibility.Reason != tt.wantReason {
				t.Fatalf("unexpected eligibility: %#v", eligibility)
			}
		})
	}
}

func TestCalculateDeploymentEligibilityHandlesDST(t *testing.T) {
	tests := []struct {
		name   string
		now    string
		day    time.Weekday
		start  int
		end    int
		wantAt string
	}{
		{
			name: "spring forward uses first representable time after gap",
			now:  "2026-03-08T06:59:30Z", day: time.Sunday,
			start: 2 * 60, end: 4 * 60,
			wantAt: "2026-03-08T07:00:00Z",
		},
		{
			name: "fall back second occurrence remains eligible",
			now:  "2026-11-01T06:30:00Z", day: time.Sunday,
			start: 60, end: 2 * 60,
			wantAt: "2026-11-01T06:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := mustSchedulePolicy(t, "America/New_York", []DeploymentScheduleWeeklyWindow{{
				DayOfWeek: tt.day, StartMinute: tt.start, EndMinute: tt.end,
			}}, nil)
			eligibility, err := CalculateDeploymentEligibility(mustTime(t, tt.now), nil, &policy)
			if err != nil {
				t.Fatalf("calculate eligibility: %v", err)
			}
			if !eligibility.NextEligibleAt.Equal(mustTime(t, tt.wantAt)) {
				t.Fatalf("expected %s, got %s", tt.wantAt, eligibility.NextEligibleAt.Format(time.RFC3339))
			}
		})
	}
}

func TestCalculateDeploymentEligibilityStopsAfterBoundedSearch(t *testing.T) {
	policy := mustSchedulePolicy(t, "UTC", []DeploymentScheduleWeeklyWindow{{
		DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60,
	}}, []DeploymentScheduleBlackout{{
		StartsAt: mustTime(t, "2026-01-01T00:00:00Z"),
		EndsAt:   mustTime(t, "2028-01-01T00:00:00Z"),
	}})

	_, err := CalculateDeploymentEligibility(mustTime(t, "2026-01-01T00:00:00Z"), nil, &policy)
	if !errors.Is(err, ErrDeploymentScheduleUnavailable) {
		t.Fatalf("expected bounded search error, got %v", err)
	}
}

func mustSchedulePolicy(t *testing.T, zone string, windows []DeploymentScheduleWeeklyWindow, blackouts []DeploymentScheduleBlackout) DeploymentSchedulePolicy {
	t.Helper()
	policy, err := NewDeploymentSchedulePolicy(DeploymentSchedulePolicyDraft{
		EnvironmentID: uuid.New(), Enabled: true, TimeZone: zone,
		WeeklyWindows: windows, Blackouts: blackouts, Version: 3,
	})
	if err != nil {
		t.Fatalf("create schedule policy: %v", err)
	}
	return policy
}

func disabledSchedulePolicy(t *testing.T) *DeploymentSchedulePolicy {
	t.Helper()
	policy, err := NewDeploymentSchedulePolicy(DeploymentSchedulePolicyDraft{EnvironmentID: uuid.New()})
	if err != nil {
		t.Fatalf("create disabled schedule policy: %v", err)
	}
	return &policy
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
