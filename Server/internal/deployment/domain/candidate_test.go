package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCandidateFingerprintIgnoresRepositoryTopologyAndRevisionEvidence(t *testing.T) {
	base := validCandidateObservation()
	first, err := NewCandidateObservation(base)
	if err != nil {
		t.Fatalf("create first observation: %v", err)
	}
	base.TargetRevision = "commit-b"
	base.TargetRevisions = []string{"commit-b"}
	base.Sources = []SourceEvidence{{RepositoryURL: "https://git.example/per-app.git", Path: "production/api"}}
	second, err := NewCandidateObservation(base)
	if err != nil {
		t.Fatalf("create second observation: %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("source topology and revision evidence changed fingerprint: %s != %s", first.Fingerprint, second.Fingerprint)
	}
}

func TestCandidateFingerprintChangesWithDeployableContentOrBinding(t *testing.T) {
	base := validCandidateObservation()
	original := base
	first, err := NewCandidateObservation(base)
	if err != nil {
		t.Fatalf("create first observation: %v", err)
	}
	base.ManifestHash = hashOf('c')
	second, err := NewCandidateObservation(base)
	if err != nil {
		t.Fatalf("create second observation: %v", err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("deployable content must change fingerprint")
	}
	base = original
	base.WorkflowVersionID = uuid.New()
	third, err := NewCandidateObservation(base)
	if err != nil {
		t.Fatalf("create third observation: %v", err)
	}
	if first.Fingerprint == third.Fingerprint {
		t.Fatal("workflow binding version must change fingerprint")
	}
}

func TestRequestPinsWorkflowAndPlanVersions(t *testing.T) {
	base := validCandidateObservation()
	original := mustCandidateObservation(t, base)

	workflowChanged := base
	workflowChanged.WorkflowVersionID = uuid.New()
	assertCandidateFingerprintChanged(t, original, workflowChanged)

	planChanged := base
	planChanged.PlanVersionID = uuid.New()
	assertCandidateFingerprintChanged(t, original, planChanged)
}

func mustCandidateObservation(t *testing.T, value CandidateObservation) CandidateObservation {
	t.Helper()
	result, err := NewCandidateObservation(value)
	if err != nil {
		t.Fatalf("create candidate observation: %v", err)
	}
	return result
}

func assertCandidateFingerprintChanged(t *testing.T, original CandidateObservation, changed CandidateObservation) {
	t.Helper()
	result := mustCandidateObservation(t, changed)
	if original.Fingerprint == result.Fingerprint {
		t.Fatal("pinned definition version must change candidate fingerprint")
	}
}

func validCandidateObservation() CandidateObservation {
	return CandidateObservation{
		ApplicationID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		ApplicationKey: "api", WorkflowVersionID: uuid.New(), PlanVersionID: uuid.New(), TargetRevision: "commit-a",
		TargetRevisions: []string{"commit-a"}, ManifestHash: hashOf('a'), DiffHash: hashOf('b'),
		Diffs:      []ResourceDiffEvidence{{Kind: "Deployment", Namespace: "api", Name: "api", NormalizedLiveHash: hashOf('d'), PredictedLiveHash: hashOf('e')}},
		Sources:    []SourceEvidence{{RepositoryURL: "https://git.example/central.git", Path: "production/api"}},
		Images:     []ImageSnapshot{{ImageReference: "registry.example/api:v1", Registry: "registry.example", Repository: "api", Tag: "v1", Digest: "sha256:" + hashOf('f')}},
		ObservedAt: time.Now(),
	}
}

func hashOf(character byte) string {
	value := make([]byte, 64)
	for index := range value {
		value[index] = character
	}
	return string(value)
}
