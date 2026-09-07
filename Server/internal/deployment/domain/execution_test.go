package domain

import "testing"

func TestAggregateExecutionStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     ExecutionStatus
	}{
		{name: "succeeded", statuses: []string{"Succeeded", "Succeeded"}, want: ExecutionSucceeded},
		{name: "failed", statuses: []string{"Failed", "Skipped"}, want: ExecutionFailed},
		{name: "partial failed", statuses: []string{"Succeeded", "Failed"}, want: ExecutionPartialFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AggregateExecutionStatus(test.statuses)
			if err != nil || got != test.want {
				t.Fatalf("AggregateExecutionStatus() = %q, %v", got, err)
			}
		})
	}
}
