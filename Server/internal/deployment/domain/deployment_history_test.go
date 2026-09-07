package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestForwardRollbackRequiresNewerTagForHistoricalDigest(t *testing.T) {
	applicationID := uuid.New()
	target := historyApplication(applicationID, "v3.0.0", "sha256:old")
	current := historyApplication(applicationID, "v2.0.0", "sha256:current")
	history := DeploymentHistoryItem{Applications: []DeploymentRequestApplicationSnapshot{
		historyApplication(applicationID, "v1.0.0", "sha256:old"),
	}}
	classification := ClassifyDeployment([]DeploymentRequestApplicationSnapshot{target},
		[]DeploymentRequestApplicationSnapshot{current}, []DeploymentHistoryItem{history})
	if classification != DeploymentClassificationForwardRollback {
		t.Fatalf("classification = %s", classification)
	}
}

func TestForwardRollbackRejectsMissingOrCurrentDigest(t *testing.T) {
	applicationID := uuid.New()
	history := historyApplication(applicationID, "v1.0.0", "sha256:old")
	tests := []struct {
		name    string
		target  DeploymentRequestApplicationSnapshot
		current DeploymentRequestApplicationSnapshot
	}{
		{name: "missing digest", target: historyApplication(applicationID, "v3.0.0", ""), current: historyApplication(applicationID, "v2.0.0", "sha256:current")},
		{name: "same current", target: historyApplication(applicationID, "v3.0.0", "sha256:old"), current: historyApplication(applicationID, "v2.0.0", "sha256:old")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification := ClassifyDeployment([]DeploymentRequestApplicationSnapshot{test.target},
				[]DeploymentRequestApplicationSnapshot{test.current}, []DeploymentHistoryItem{{Applications: []DeploymentRequestApplicationSnapshot{history}}})
			if classification != DeploymentClassificationStandard {
				t.Fatalf("classification = %s", classification)
			}
		})
	}
}

func TestForwardRollbackRequiresCompleteMultiApplicationHistory(t *testing.T) {
	firstID, secondID := uuid.New(), uuid.New()
	target := []DeploymentRequestApplicationSnapshot{
		historyApplication(firstID, "v3.0.0", "sha256:first-old"),
		historyApplication(secondID, "v2.0.0", "sha256:second-stable"),
	}
	current := []DeploymentRequestApplicationSnapshot{
		historyApplication(firstID, "v2.0.0", "sha256:first-current"),
		historyApplication(secondID, "v2.0.0", "sha256:second-stable"),
	}
	incomplete := DeploymentHistoryItem{Applications: target[:1]}
	complete := DeploymentHistoryItem{Applications: []DeploymentRequestApplicationSnapshot{
		historyApplication(firstID, "v1.0.0", "sha256:first-old"),
		historyApplication(secondID, "v2.0.0", "sha256:second-stable"),
	}}
	if ClassifyDeployment(target, current, []DeploymentHistoryItem{incomplete}) != DeploymentClassificationStandard {
		t.Fatal("incomplete successful history must not classify the request")
	}
	if ClassifyDeployment(target, current, []DeploymentHistoryItem{complete}) != DeploymentClassificationForwardRollback {
		t.Fatal("complete successful history must classify the request")
	}
}

func historyApplication(applicationID uuid.UUID, tag, digest string) DeploymentRequestApplicationSnapshot {
	return DeploymentRequestApplicationSnapshot{ApplicationID: applicationID, Images: []DeploymentRequestImageSnapshot{{
		Registry: "registry.example", Repository: "service", Tag: tag, Digest: digest,
	}}}
}
