// Package domain contains deployment governance concepts independent of external clients.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImageSnapshot locks one target-manifest image reference to an immutable digest.
type ImageSnapshot struct {
	ImageReference string `json:"imageReference"`
	Registry       string `json:"registry"`
	Repository     string `json:"repository"`
	Tag            string `json:"tag"`
	Digest         string `json:"digest"`
}

// ResourceDiffEvidence records content hashes without persisting potentially sensitive manifests.
type ResourceDiffEvidence struct {
	Group              string `json:"group"`
	Kind               string `json:"kind"`
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	NormalizedLiveHash string `json:"normalizedLiveHash"`
	PredictedLiveHash  string `json:"predictedLiveHash"`
}

// SourceEvidence preserves traceability but is intentionally excluded from the content fingerprint.
type SourceEvidence struct {
	RepositoryURL  string `json:"repositoryUrl"`
	TargetRevision string `json:"targetRevision"`
	Path           string `json:"path"`
	Chart          string `json:"chart"`
	Reference      string `json:"reference"`
	Name           string `json:"name"`
}

// CandidateObservation is one normalized, immutable Application-level deployment candidate.
type CandidateObservation struct {
	ID                uuid.UUID
	ApplicationID     uuid.UUID
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	ApplicationKey    string
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	TargetRevision    string
	TargetRevisions   []string
	ManifestHash      string
	DiffHash          string
	Diffs             []ResourceDiffEvidence
	Sources           []SourceEvidence
	Images            []ImageSnapshot
	Fingerprint       string
	ObservedAt        time.Time
}

// NewCandidateObservation validates an Application observation and computes its repository-independent fingerprint.
func NewCandidateObservation(value CandidateObservation) (CandidateObservation, error) {
	if err := validateCandidateObservation(value); err != nil {
		return CandidateObservation{}, err
	}
	value.ApplicationKey = strings.TrimSpace(value.ApplicationKey)
	value.TargetRevision = strings.TrimSpace(value.TargetRevision)
	value.ID = uuid.New()
	value = cloneCandidateObservation(value)
	value.ObservedAt = value.ObservedAt.UTC()
	slices.SortFunc(value.Images, func(left, right ImageSnapshot) int {
		return strings.Compare(left.ImageReference+"\x00"+left.Digest, right.ImageReference+"\x00"+right.Digest)
	})
	fingerprint, err := fingerprint(value)
	if err != nil {
		return CandidateObservation{}, err
	}
	value.Fingerprint = fingerprint
	return value, nil
}

func validateCandidateObservation(value CandidateObservation) error {
	if value.ApplicationID == uuid.Nil || value.OrganizationID == uuid.Nil || value.ProjectID == uuid.Nil || value.EnvironmentID == uuid.Nil || value.WorkflowVersionID == uuid.Nil || value.PlanVersionID == uuid.Nil {
		return errors.New("candidate observation requires complete resource and binding identities")
	}
	if strings.TrimSpace(value.ApplicationKey) == "" || strings.TrimSpace(value.TargetRevision) == "" || len(value.Diffs) == 0 || len(value.Images) == 0 || value.ObservedAt.IsZero() {
		return errors.New("candidate observation requires an application key, revision, diff, images, and observation time")
	}
	if !validSHA256(value.ManifestHash) || !validSHA256(value.DiffHash) {
		return errors.New("candidate observation requires SHA-256 content hashes")
	}
	return nil
}

func cloneCandidateObservation(value CandidateObservation) CandidateObservation {
	value.TargetRevisions = slices.Clone(value.TargetRevisions)
	value.Diffs = slices.Clone(value.Diffs)
	value.Sources = slices.Clone(value.Sources)
	value.Images = slices.Clone(value.Images)
	return value
}

func fingerprint(value CandidateObservation) (string, error) {
	payload := struct {
		ApplicationID     uuid.UUID       `json:"applicationId"`
		WorkflowVersionID uuid.UUID       `json:"workflowVersionId"`
		PlanVersionID     uuid.UUID       `json:"planVersionId"`
		ManifestHash      string          `json:"manifestHash"`
		DiffHash          string          `json:"diffHash"`
		Images            []ImageSnapshot `json:"images"`
	}{
		ApplicationID: value.ApplicationID, WorkflowVersionID: value.WorkflowVersionID,
		PlanVersionID: value.PlanVersionID, ManifestHash: value.ManifestHash, DiffHash: value.DiffHash, Images: value.Images,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal candidate fingerprint input: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
