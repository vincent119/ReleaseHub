package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maximumDeploymentScheduleWeeklyWindows = 32
	maximumDeploymentScheduleBlackouts     = 64
	deploymentScheduleSearchLimit          = 366 * 24 * time.Hour
	minutesPerDay                          = 24 * 60
)

var (
	ErrInvalidDeploymentSchedule     = errors.New("invalid deployment schedule")
	ErrDeploymentScheduleUnavailable = errors.New("deployment schedule has no eligible time within search limit")
)

// DeploymentScheduleReason explains the constraint that determines the next eligible time.
type DeploymentScheduleReason string

const (
	DeploymentScheduleReady                       DeploymentScheduleReason = "Ready"
	DeploymentScheduleWaitingForScheduledTime     DeploymentScheduleReason = "ScheduledTime"
	DeploymentScheduleWaitingForMaintenanceWindow DeploymentScheduleReason = "MaintenanceWindow"
	DeploymentScheduleWaitingForBlackout          DeploymentScheduleReason = "Blackout"
)

// DeploymentScheduleWeeklyWindow defines an inclusive start and exclusive end in local wall-clock minutes.
type DeploymentScheduleWeeklyWindow struct {
	DayOfWeek   time.Weekday
	StartMinute int
	EndMinute   int
}

// DeploymentScheduleBlackout defines an inclusive start and exclusive end as absolute instants.
type DeploymentScheduleBlackout struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// DeploymentSchedulePolicy is one validated Environment-scoped schedule document.
type DeploymentSchedulePolicy struct {
	EnvironmentID uuid.UUID
	Enabled       bool
	TimeZone      string
	WeeklyWindows []DeploymentScheduleWeeklyWindow
	Blackouts     []DeploymentScheduleBlackout
	Version       uint64
}

// DeploymentSchedulePolicyDraft contains one schedule policy command.
type DeploymentSchedulePolicyDraft struct {
	EnvironmentID uuid.UUID
	Enabled       bool
	TimeZone      string
	WeeklyWindows []DeploymentScheduleWeeklyWindow
	Blackouts     []DeploymentScheduleBlackout
	Version       uint64
}

// DeploymentEligibility is the authoritative next-start decision for a deployment.
type DeploymentEligibility struct {
	EligibleNow    bool
	NextEligibleAt time.Time
	Reason         DeploymentScheduleReason
	PolicyVersion  uint64
}

// NewDeploymentSchedulePolicy validates and normalizes an Environment schedule policy.
func NewDeploymentSchedulePolicy(draft DeploymentSchedulePolicyDraft) (DeploymentSchedulePolicy, error) {
	draft.TimeZone = strings.TrimSpace(draft.TimeZone)
	if err := validateDeploymentScheduleDraft(&draft); err != nil {
		return DeploymentSchedulePolicy{}, err
	}
	blackouts, err := normalizeDeploymentScheduleBlackouts(draft.Blackouts)
	if err != nil {
		return DeploymentSchedulePolicy{}, err
	}
	return DeploymentSchedulePolicy{
		EnvironmentID: draft.EnvironmentID, Enabled: draft.Enabled, TimeZone: draft.TimeZone,
		WeeklyWindows: slices.Clone(draft.WeeklyWindows),
		Blackouts:     blackouts, Version: draft.Version,
	}, nil
}

func validateDeploymentScheduleDraft(draft *DeploymentSchedulePolicyDraft) error {
	if draft.EnvironmentID == uuid.Nil {
		return fmt.Errorf("%w: environment is required", ErrInvalidDeploymentSchedule)
	}
	if draft.TimeZone == "" && !draft.Enabled {
		draft.TimeZone = "UTC"
	}
	if draft.TimeZone == "" {
		return fmt.Errorf("%w: time zone is required", ErrInvalidDeploymentSchedule)
	}
	if _, err := time.LoadLocation(draft.TimeZone); err != nil {
		return fmt.Errorf("%w: time zone is not an IANA location", ErrInvalidDeploymentSchedule)
	}
	return validateDeploymentScheduleCollections(*draft)
}

func validateDeploymentScheduleCollections(draft DeploymentSchedulePolicyDraft) error {
	if len(draft.WeeklyWindows) > maximumDeploymentScheduleWeeklyWindows {
		return fmt.Errorf("%w: too many weekly windows", ErrInvalidDeploymentSchedule)
	}
	if draft.Enabled && len(draft.WeeklyWindows) == 0 {
		return fmt.Errorf("%w: enabled policy requires a weekly window", ErrInvalidDeploymentSchedule)
	}
	if len(draft.Blackouts) > maximumDeploymentScheduleBlackouts {
		return fmt.Errorf("%w: too many blackouts", ErrInvalidDeploymentSchedule)
	}
	return validateDeploymentScheduleWindows(draft.WeeklyWindows)
}

func normalizeDeploymentScheduleBlackouts(values []DeploymentScheduleBlackout) ([]DeploymentScheduleBlackout, error) {
	result := slices.Clone(values)
	for index := range result {
		if result[index].StartsAt.IsZero() || !result[index].EndsAt.After(result[index].StartsAt) {
			return nil, fmt.Errorf("%w: blackout interval is invalid", ErrInvalidDeploymentSchedule)
		}
		result[index].StartsAt = result[index].StartsAt.UTC()
		result[index].EndsAt = result[index].EndsAt.UTC()
	}
	return result, nil
}

func validateDeploymentScheduleWindows(windows []DeploymentScheduleWeeklyWindow) error {
	ordered := slices.Clone(windows)
	for _, window := range ordered {
		if !validDeploymentScheduleWindow(window) {
			return fmt.Errorf("%w: weekly window is invalid", ErrInvalidDeploymentSchedule)
		}
	}
	slices.SortFunc(ordered, func(left, right DeploymentScheduleWeeklyWindow) int {
		if left.DayOfWeek != right.DayOfWeek {
			return int(left.DayOfWeek - right.DayOfWeek)
		}
		return left.StartMinute - right.StartMinute
	})
	if deploymentScheduleWindowsOverlap(ordered) {
		return fmt.Errorf("%w: weekly windows overlap", ErrInvalidDeploymentSchedule)
	}
	return nil
}

func validDeploymentScheduleWindow(window DeploymentScheduleWeeklyWindow) bool {
	return window.DayOfWeek >= time.Sunday && window.DayOfWeek <= time.Saturday &&
		window.StartMinute >= 0 && window.StartMinute < minutesPerDay &&
		window.EndMinute > window.StartMinute && window.EndMinute <= minutesPerDay
}

func deploymentScheduleWindowsOverlap(windows []DeploymentScheduleWeeklyWindow) bool {
	for index := 1; index < len(windows); index++ {
		previous, current := windows[index-1], windows[index]
		if previous.DayOfWeek == current.DayOfWeek && current.StartMinute < previous.EndMinute {
			return true
		}
	}
	return false
}

// CalculateDeploymentEligibility returns the first allowed instant at or after both now and scheduledFor.
func CalculateDeploymentEligibility(now time.Time, scheduledFor *time.Time, policy *DeploymentSchedulePolicy) (DeploymentEligibility, error) {
	if now.IsZero() || scheduledFor != nil && scheduledFor.IsZero() {
		return DeploymentEligibility{}, fmt.Errorf("%w: calculation time is invalid", ErrInvalidDeploymentSchedule)
	}
	now, candidate, waiting := deploymentScheduleCandidate(now, scheduledFor)
	if policy == nil || !policy.Enabled {
		return eligibleDeploymentSchedule(candidate, now, waiting, policyVersion(policy)), nil
	}
	validated, err := validateDeploymentSchedulePolicy(*policy)
	if err != nil {
		return DeploymentEligibility{}, err
	}
	return calculatePolicyEligibility(now, candidate, waiting, validated)
}

func validateDeploymentSchedulePolicy(policy DeploymentSchedulePolicy) (DeploymentSchedulePolicy, error) {
	return NewDeploymentSchedulePolicy(DeploymentSchedulePolicyDraft{
		EnvironmentID: policy.EnvironmentID,
		Enabled:       policy.Enabled, TimeZone: policy.TimeZone,
		WeeklyWindows: policy.WeeklyWindows,
		Blackouts:     policy.Blackouts, Version: policy.Version,
	})
}

func deploymentScheduleCandidate(now time.Time, scheduledFor *time.Time) (time.Time, time.Time, bool) {
	now = now.UTC()
	if scheduledFor != nil && scheduledFor.After(now) {
		return now, scheduledFor.UTC(), true
	}
	return now, now, false
}

func calculatePolicyEligibility(now, candidate time.Time, waiting bool, policy DeploymentSchedulePolicy) (DeploymentEligibility, error) {
	location, _ := time.LoadLocation(policy.TimeZone)
	if deploymentScheduleAllows(candidate, policy, location) {
		return eligibleDeploymentSchedule(candidate, now, waiting, policy.Version), nil
	}
	reason := deploymentScheduleBlockedReason(candidate, policy.Blackouts)
	return searchDeploymentSchedule(candidate, reason, policy, location)
}

func deploymentScheduleBlockedReason(candidate time.Time, blackouts []DeploymentScheduleBlackout) DeploymentScheduleReason {
	if deploymentScheduleBlackoutAt(candidate, blackouts) != nil {
		return DeploymentScheduleWaitingForBlackout
	}
	return DeploymentScheduleWaitingForMaintenanceWindow
}

func searchDeploymentSchedule(candidate time.Time, reason DeploymentScheduleReason, policy DeploymentSchedulePolicy, location *time.Location) (DeploymentEligibility, error) {
	limit := candidate.Add(deploymentScheduleSearchLimit)
	cursor := ceilToMinute(candidate)
	for !cursor.After(limit) {
		if blackout := deploymentScheduleBlackoutAt(cursor, policy.Blackouts); blackout != nil {
			cursor = blackout.EndsAt
			continue
		}
		if deploymentScheduleWithinWeeklyWindow(cursor, policy.WeeklyWindows, location) {
			return DeploymentEligibility{
				EligibleNow: false, NextEligibleAt: cursor.UTC(),
				Reason: reason, PolicyVersion: policy.Version,
			}, nil
		}
		cursor = nextDeploymentScheduleMinute(cursor)
	}
	return DeploymentEligibility{}, ErrDeploymentScheduleUnavailable
}

func eligibleDeploymentSchedule(at, now time.Time, waitingForScheduledTime bool, version uint64) DeploymentEligibility {
	reason := DeploymentScheduleReady
	if waitingForScheduledTime {
		reason = DeploymentScheduleWaitingForScheduledTime
	}
	return DeploymentEligibility{
		EligibleNow:    at.Equal(now),
		NextEligibleAt: at.UTC(),
		Reason:         reason,
		PolicyVersion:  version,
	}
}

func deploymentScheduleAllows(at time.Time, policy DeploymentSchedulePolicy, location *time.Location) bool {
	return deploymentScheduleWithinWeeklyWindow(at, policy.WeeklyWindows, location) &&
		deploymentScheduleBlackoutAt(at, policy.Blackouts) == nil
}

func deploymentScheduleWithinWeeklyWindow(at time.Time, windows []DeploymentScheduleWeeklyWindow, location *time.Location) bool {
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	for _, window := range windows {
		if window.DayOfWeek == local.Weekday() && minute >= window.StartMinute && minute < window.EndMinute {
			return true
		}
	}
	return false
}

func deploymentScheduleBlackoutAt(at time.Time, blackouts []DeploymentScheduleBlackout) *DeploymentScheduleBlackout {
	for index := range blackouts {
		if !at.Before(blackouts[index].StartsAt) && at.Before(blackouts[index].EndsAt) {
			return &blackouts[index]
		}
	}
	return nil
}

func ceilToMinute(value time.Time) time.Time {
	value = value.UTC()
	truncated := value.Truncate(time.Minute)
	if value.Equal(truncated) {
		return value
	}
	return truncated.Add(time.Minute)
}

func nextDeploymentScheduleMinute(value time.Time) time.Time {
	if !value.Equal(value.Truncate(time.Minute)) {
		return ceilToMinute(value)
	}
	return value.Add(time.Minute)
}

func policyVersion(policy *DeploymentSchedulePolicy) uint64 {
	if policy == nil {
		return 0
	}
	return policy.Version
}
