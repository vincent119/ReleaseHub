package application

import (
	"testing"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestTargetManifestHashIsStableAcrossFormattingAndOrdering(t *testing.T) {
	first, err := HashTargetManifests([]string{
		"apiVersion: v1\nkind: Service\nmetadata:\n  name: api\n",
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n",
	})
	if err != nil {
		t.Fatalf("hash first manifests: %v", err)
	}
	second, err := HashTargetManifests([]string{
		"kind: Deployment\nmetadata: {name: api}\napiVersion: apps/v1\n",
		"kind: Service\nmetadata:\n  name: api\napiVersion: v1\n",
	})
	if err != nil {
		t.Fatalf("hash second manifests: %v", err)
	}
	if first != second {
		t.Fatalf("formatting changed manifest hash: %s != %s", first, second)
	}
}

func TestResourceDiffEvidenceIsStableAndRedacted(t *testing.T) {
	values := []argodomain.ResourceDiff{
		{Kind: "Service", Name: "api", Modified: false},
		{Group: "apps", Kind: "Deployment", Namespace: "api", Name: "api", Modified: true,
			NormalizedLiveState: `{"spec":{"replicas":1},"metadata":{"name":"api"}}`,
			PredictedLiveState:  `{"metadata":{"name":"api"},"spec":{"replicas":2}}`},
	}
	evidence, hash, err := NormalizeResourceDiffs(values)
	if err != nil {
		t.Fatalf("normalize differences: %v", err)
	}
	if len(evidence) != 1 || hash == "" {
		t.Fatalf("unexpected evidence: %#v %q", evidence, hash)
	}
	if evidence[0].NormalizedLiveHash == values[1].NormalizedLiveState || evidence[0].PredictedLiveHash == values[1].PredictedLiveState {
		t.Fatal("resource bodies must not be persisted as diff evidence")
	}
}
