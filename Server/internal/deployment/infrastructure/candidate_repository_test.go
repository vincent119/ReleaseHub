package infrastructure

import (
	"testing"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestMarshalCandidateEvidencePreservesEmptyRevisionVector(t *testing.T) {
	_, _, _, revisions, err := marshalCandidateEvidence(deploydomain.CandidateObservation{
		Sources: []deploydomain.SourceEvidence{}, TargetRevisions: []string{},
		Diffs: []deploydomain.ResourceDiffEvidence{}, Images: []deploydomain.ImageSnapshot{},
	})
	if err != nil {
		t.Fatalf("marshal candidate evidence: %v", err)
	}
	if string(revisions) != "[]" {
		t.Fatalf("target revisions JSON = %s", revisions)
	}
}
