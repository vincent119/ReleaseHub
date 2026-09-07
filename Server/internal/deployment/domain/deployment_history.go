package domain

import (
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
)

const (
	DeploymentClassificationStandard        = "Standard"
	DeploymentClassificationForwardRollback = "ForwardRollback"
)

// DeploymentHistoryItem is one fully successful immutable deployment snapshot.
type DeploymentHistoryItem struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	Classification   string
	CompletedAt      time.Time
	Applications     []DeploymentRequestApplicationSnapshot
}

// ClassifyDeployment identifies a normal request or a safe Forward Rollback.
func ClassifyDeployment(target, current []DeploymentRequestApplicationSnapshot, history []DeploymentHistoryItem) string {
	if len(target) == 0 || len(target) != len(current) {
		return DeploymentClassificationStandard
	}
	for _, item := range history {
		if isForwardRollback(target, current, item.Applications) {
			return DeploymentClassificationForwardRollback
		}
	}
	return DeploymentClassificationStandard
}

func isForwardRollback(target, current, historical []DeploymentRequestApplicationSnapshot) bool {
	if len(target) != len(historical) {
		return false
	}
	currentByID := applicationSnapshotsByID(current)
	historyByID := applicationSnapshotsByID(historical)
	changed := false
	for _, application := range target {
		currentApplication, currentFound := currentByID[application.ApplicationID]
		historyApplication, historyFound := historyByID[application.ApplicationID]
		valid, applicationChanged := matchesRollbackApplication(application, currentApplication, historyApplication)
		if !currentFound || !historyFound || !valid {
			return false
		}
		changed = changed || applicationChanged
	}
	return changed
}

func matchesRollbackApplication(target, current, historical DeploymentRequestApplicationSnapshot) (bool, bool) {
	if len(target.Images) == 0 || len(target.Images) != len(current.Images) || len(target.Images) != len(historical.Images) {
		return false, false
	}
	currentImages := imagesByRepository(current.Images)
	historyImages := imagesByRepository(historical.Images)
	changed := false
	for _, image := range target.Images {
		currentImage, currentFound := currentImages[imageRepositoryKey(image)]
		historyImage, historyFound := historyImages[imageRepositoryKey(image)]
		valid, imageChanged := matchesRollbackImage(image, currentImage, historyImage)
		if !currentFound || !historyFound || !valid {
			return false, false
		}
		changed = changed || imageChanged
	}
	return true, changed
}

func matchesRollbackImage(target, current, historical DeploymentRequestImageSnapshot) (bool, bool) {
	if target.Digest == "" || current.Digest == "" || historical.Digest == "" || target.Digest != historical.Digest {
		return false, false
	}
	if target.Digest == current.Digest {
		return true, false
	}
	return newerSemanticVersion(target.Tag, historical.Tag), true
}

func newerSemanticVersion(target, historical string) bool {
	targetVersion, targetErr := semver.StrictNewVersion(strings.TrimPrefix(strings.TrimSpace(target), "v"))
	historyVersion, historyErr := semver.StrictNewVersion(strings.TrimPrefix(strings.TrimSpace(historical), "v"))
	return targetErr == nil && historyErr == nil && targetVersion.GreaterThan(historyVersion)
}

func applicationSnapshotsByID(values []DeploymentRequestApplicationSnapshot) map[uuid.UUID]DeploymentRequestApplicationSnapshot {
	result := make(map[uuid.UUID]DeploymentRequestApplicationSnapshot, len(values))
	for _, value := range values {
		result[value.ApplicationID] = value
	}
	return result
}

func imagesByRepository(values []DeploymentRequestImageSnapshot) map[string]DeploymentRequestImageSnapshot {
	result := make(map[string]DeploymentRequestImageSnapshot, len(values))
	for _, value := range values {
		result[imageRepositoryKey(value)] = value
	}
	return result
}

func imageRepositoryKey(value DeploymentRequestImageSnapshot) string {
	return value.Registry + "\x00" + value.Repository
}
