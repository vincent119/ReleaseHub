// Package domain contains Argo CD observations without vendor client types.
package domain

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	// ManagedLabelKey opts an Argo CD Application into ReleaseHub discovery.
	ManagedLabelKey = "releasehub.io/managed"
	// ManagedLabelValue is matched exactly according to Kubernetes label semantics.
	ManagedLabelValue = "true"
)

// ApplicationIdentity identifies one Application in the configured Argo CD instance.
type ApplicationIdentity struct {
	Namespace string
	Name      string
}

// Source describes one desired manifest source without leaking Argo CD API types.
type Source struct {
	RepositoryURL  string `json:"repositoryUrl"`
	TargetRevision string `json:"targetRevision"`
	Path           string `json:"path"`
	Chart          string `json:"chart"`
	Reference      string `json:"reference"`
	Name           string `json:"name"`
}

// Destination identifies the target cluster and namespace.
type Destination struct {
	Server    string
	Name      string
	Namespace string
}

// ResourceDiff is the normalized Application-level difference between live and target state.
type ResourceDiff struct {
	Group               string
	Kind                string
	Namespace           string
	Name                string
	NormalizedLiveState string
	PredictedLiveState  string
	Modified            bool
}

// Application is one immutable observation returned by Argo CD.
type Application struct {
	Identity          ApplicationIdentity
	Project           string
	Labels            map[string]string
	Sources           []Source
	Destination       Destination
	AutomatedSync     bool
	SyncStatus        string
	HealthStatus      string
	OperationPhase    string
	OperationID       string
	ResolvedRevision  string
	ResolvedRevisions []string
	ResourceVersion   string
	ReconciledAt      *time.Time
}

// SyncRequest identifies one immutable deployment operation.
type SyncRequest struct {
	Identity    ApplicationIdentity
	Project     string
	Revision    string
	Revisions   []string
	OperationID string
}

// ApplicationWatch streams immutable Application observations.
type ApplicationWatch interface {
	Recv() (Application, error)
	Close()
}

// NewApplication validates and copies an external observation.
func NewApplication(value Application) (Application, error) {
	value.Identity.Namespace = strings.TrimSpace(value.Identity.Namespace)
	value.Identity.Name = strings.TrimSpace(value.Identity.Name)
	if value.Identity.Namespace == "" || value.Identity.Name == "" {
		return Application{}, fmt.Errorf("invalid Argo CD Application: namespace and name are required")
	}
	value.Labels = maps.Clone(value.Labels)
	value.Sources = slices.Clone(value.Sources)
	value.ResolvedRevisions = slices.Clone(value.ResolvedRevisions)
	if value.ReconciledAt != nil {
		observed := value.ReconciledAt.UTC()
		value.ReconciledAt = &observed
	}
	return value, nil
}

// IsCandidate reports whether the exact opt-in label is present.
func (a Application) IsCandidate() bool {
	return a.Labels[ManagedLabelKey] == ManagedLabelValue
}
