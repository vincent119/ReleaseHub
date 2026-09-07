package domain

import (
	"errors"
	"slices"
	"sort"
	"strings"
)

var ErrPreflightBlocked = errors.New("deployment preflight blocked")

// PreflightEvidence contains fresh Application-level content evidence.
type PreflightEvidence struct {
	AutomatedSync    bool
	ResolvedRevision string
	ManifestHash     string
	DiffHash         string
	Images           []DeploymentRequestImageSnapshot
}

// PreflightResult records whether immutable reviewed content remains deployable.
type PreflightResult struct {
	Allowed bool
	Code    string
}

// EvaluatePreflight compares content rather than repository-wide revision movement.
func EvaluatePreflight(expected DeploymentRequestApplicationSnapshot, actual PreflightEvidence) PreflightResult {
	if actual.AutomatedSync {
		return blockedPreflight("automated_sync_enabled")
	}
	if expected.ManifestHash != actual.ManifestHash || expected.DiffHash != actual.DiffHash {
		return blockedPreflight("application_content_changed")
	}
	if !sameDeploymentImages(expected.Images, actual.Images) {
		return blockedPreflight("image_digest_drift")
	}
	return PreflightResult{Allowed: true}
}

func blockedPreflight(code string) PreflightResult {
	return PreflightResult{Code: code}
}

func sameDeploymentImages(left, right []DeploymentRequestImageSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	return slices.Equal(canonicalImages(left), canonicalImages(right))
}

func canonicalImages(values []DeploymentRequestImageSnapshot) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strings.Join([]string{
			value.ImageReference, value.Registry, value.Repository, value.Tag, value.Digest,
		}, "\x00"))
	}
	sort.Strings(result)
	return result
}
