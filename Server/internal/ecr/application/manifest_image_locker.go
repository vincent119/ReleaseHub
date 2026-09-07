package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

// ManifestImageLocker applies the existing ECR scope and digest rules to rendered manifests.
type ManifestImageLocker struct {
	registry DigestRegistry
	scope    ecrdomain.RegistryScope
	now      func() time.Time
}

// NewManifestImageLocker creates the shared immutable image resolution capability.
func NewManifestImageLocker(registry DigestRegistry, scope ecrdomain.RegistryScope) (*ManifestImageLocker, error) {
	if registry == nil || len(scope.Repositories) == 0 {
		return nil, errors.New("invalid manifest image locker dependencies")
	}
	return &ManifestImageLocker{registry: registry, scope: scope, now: time.Now}, nil
}

// Lock resolves every target-manifest image and returns all evidence even when one image is unavailable.
func (l *ManifestImageLocker) Lock(ctx context.Context, applicationID uuid.UUID, revision string, manifests []string) ([]ecrdomain.DigestSnapshot, error) {
	images, err := ExtractManifestImages(manifests)
	if err != nil {
		return nil, ErrTargetManifestUnavailable
	}
	resolvedAt := l.now().UTC()
	snapshots := make([]ecrdomain.DigestSnapshot, 0, len(images))
	allAvailable := true
	for _, image := range images {
		snapshot := l.resolveImage(ctx, applicationID, revision, image, resolvedAt)
		if snapshot.Status != ecrdomain.DigestAvailable {
			allAvailable = false
		}
		snapshots = append(snapshots, snapshot)
	}
	if !allAvailable {
		return snapshots, ErrDigestUnavailable
	}
	return snapshots, nil
}

func (l *ManifestImageLocker) resolveImage(ctx context.Context, applicationID uuid.UUID, revision, image string, resolvedAt time.Time) ecrdomain.DigestSnapshot {
	reference, err := l.scope.ParseImageReference(image)
	if err != nil {
		return unavailableSnapshot(applicationID, revision, image, referenceErrorCode(err), resolvedAt)
	}
	digest, err := l.registry.ResolveDigest(ctx, reference)
	if err != nil {
		return unavailableSnapshot(applicationID, revision, image, registryErrorCode(err), resolvedAt)
	}
	snapshot, err := ecrdomain.NewAvailableDigestSnapshot(applicationID, revision, reference, digest, resolvedAt)
	if err != nil {
		return unavailableSnapshot(applicationID, revision, image, "invalid_ecr_digest", resolvedAt)
	}
	return snapshot
}

func unavailableSnapshot(applicationID uuid.UUID, revision, image, code string, resolvedAt time.Time) ecrdomain.DigestSnapshot {
	snapshot, err := ecrdomain.NewUnavailableDigestSnapshot(applicationID, revision, image, code, resolvedAt)
	if err != nil {
		return ecrdomain.DigestSnapshot{ApplicationID: applicationID, ManifestRevision: revision, ImageReference: image, Status: ecrdomain.DigestUnavailable, ErrorCode: "invalid_resolution_input", ResolvedAt: resolvedAt}
	}
	return snapshot
}
