package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrImageNotFound means ECR cannot find the exact requested tag or digest.
	ErrImageNotFound = errors.New("ecr image was not found")
	// ErrDigestMismatch means a tag plus digest reference no longer resolves to the requested digest.
	ErrDigestMismatch = errors.New("ecr image digest does not match the target manifest")
	// ErrRegistryUnavailable prevents vendor details from crossing the application boundary.
	ErrRegistryUnavailable = errors.New("ecr image metadata is unavailable")
)

// DigestStatus describes whether one exact image reference could be locked to a digest.
type DigestStatus string

const (
	DigestAvailable   DigestStatus = "Available"
	DigestUnavailable DigestStatus = "Unavailable"
)

// DigestSnapshot records a target manifest image reference and its immutable resolution outcome.
type DigestSnapshot struct {
	ApplicationID    uuid.UUID
	ManifestRevision string
	ImageReference   string
	Registry         string
	Repository       string
	Tag              string
	RequestedDigest  string
	ResolvedDigest   string
	Status           DigestStatus
	ErrorCode        string
	ResolvedAt       time.Time
}

// NewAvailableDigestSnapshot creates a successful immutable ECR resolution.
func NewAvailableDigestSnapshot(applicationID uuid.UUID, manifestRevision string, reference ImageReference, digest string, resolvedAt time.Time) (DigestSnapshot, error) {
	if applicationID == uuid.Nil || strings.TrimSpace(manifestRevision) == "" || strings.TrimSpace(digest) == "" || resolvedAt.IsZero() {
		return DigestSnapshot{}, errors.New("invalid available digest snapshot")
	}
	return DigestSnapshot{
		ApplicationID: applicationID, ManifestRevision: manifestRevision, ImageReference: reference.Original,
		Registry: reference.Registry, Repository: reference.Repository, Tag: reference.Tag,
		RequestedDigest: reference.RequestedDigest, ResolvedDigest: digest, Status: DigestAvailable,
		ResolvedAt: resolvedAt.UTC(),
	}, nil
}

// NewUnavailableDigestSnapshot creates a stable failure record without external error details.
func NewUnavailableDigestSnapshot(applicationID uuid.UUID, manifestRevision, imageReference, code string, resolvedAt time.Time) (DigestSnapshot, error) {
	if applicationID == uuid.Nil || strings.TrimSpace(manifestRevision) == "" || strings.TrimSpace(imageReference) == "" || strings.TrimSpace(code) == "" || resolvedAt.IsZero() {
		return DigestSnapshot{}, errors.New("invalid unavailable digest snapshot")
	}
	return DigestSnapshot{
		ApplicationID: applicationID, ManifestRevision: manifestRevision, ImageReference: imageReference,
		Status: DigestUnavailable, ErrorCode: code, ResolvedAt: resolvedAt.UTC(),
	}, nil
}
