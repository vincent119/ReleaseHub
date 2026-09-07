package domain

import "testing"

func TestUnrelatedRepositoryCommitDoesNotSupersedeRequest(t *testing.T) {
	expected := preflightApplication()
	actual := PreflightEvidence{
		ResolvedRevision: "unrelated-new-commit", ManifestHash: expected.ManifestHash,
		DiffHash: expected.DiffHash, Images: expected.Images,
	}
	result := EvaluatePreflight(expected, actual)
	if !result.Allowed {
		t.Fatalf("unchanged Application content must remain deployable: %#v", result)
	}
}

func TestApplicationContentChangeSupersedesRequest(t *testing.T) {
	expected := preflightApplication()
	actual := PreflightEvidence{ManifestHash: "new-manifest", DiffHash: expected.DiffHash, Images: expected.Images}
	result := EvaluatePreflight(expected, actual)
	if result.Allowed || result.Code != "application_content_changed" {
		t.Fatalf("changed content must block the reviewed version: %#v", result)
	}
}

func TestDeploymentBlockedOnDigestDrift(t *testing.T) {
	expected := preflightApplication()
	actualImages := append([]DeploymentRequestImageSnapshot(nil), expected.Images...)
	actualImages[0].Digest = "sha256:new"
	result := EvaluatePreflight(expected, PreflightEvidence{
		ManifestHash: expected.ManifestHash, DiffHash: expected.DiffHash, Images: actualImages,
	})
	if result.Allowed || result.Code != "image_digest_drift" {
		t.Fatalf("digest drift must block deployment: %#v", result)
	}
}

func preflightApplication() DeploymentRequestApplicationSnapshot {
	return DeploymentRequestApplicationSnapshot{
		ManifestHash: "manifest", DiffHash: "diff",
		Images: []DeploymentRequestImageSnapshot{{
			ImageReference: "registry/repository:v1", Registry: "registry",
			Repository: "repository", Tag: "v1", Digest: "sha256:old",
		}},
	}
}
