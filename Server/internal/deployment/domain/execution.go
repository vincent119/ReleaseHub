package domain

import "errors"

// ExecutionStatus identifies one deployment attempt result.
type ExecutionStatus string

const (
	ExecutionSucceeded     ExecutionStatus = "Succeeded"
	ExecutionFailed        ExecutionStatus = "Failed"
	ExecutionPartialFailed ExecutionStatus = "PartialFailed"
	ExecutionTerminated    ExecutionStatus = "Terminated"
)

// AggregateExecutionStatus derives the terminal result from durable node states.
func AggregateExecutionStatus(statuses []string) (ExecutionStatus, error) {
	if len(statuses) == 0 {
		return "", errors.New("deployment execution requires nodes")
	}
	succeeded, failed := 0, 0
	for _, status := range statuses {
		result, valid := terminalNodeResult(status)
		if !valid {
			return "", errors.New("deployment execution has active nodes")
		}
		succeeded += result
		failed += 1 - result
	}
	return aggregateTerminalCounts(succeeded, failed), nil
}

func aggregateTerminalCounts(succeeded, failed int) ExecutionStatus {
	if failed == 0 {
		return ExecutionSucceeded
	}
	if succeeded == 0 {
		return ExecutionFailed
	}
	return ExecutionPartialFailed
}

func terminalNodeResult(status string) (int, bool) {
	if status == "Succeeded" {
		return 1, true
	}
	switch status {
	case "Failed", "Blocked", "Terminated", "Skipped", "Waiting", "Queued", "Preflight":
		return 0, true
	default:
		return 0, false
	}
}

// ReleasesApplicationLocks reports whether this result has converged safely.
func (s ExecutionStatus) ReleasesApplicationLocks() bool {
	return s == ExecutionSucceeded || s == ExecutionFailed
}
