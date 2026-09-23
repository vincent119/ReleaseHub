package domain

import (
	"fmt"
	"strings"
	"time"
)

// RuntimeResourceRef identifies one Kubernetes resource observed through Argo CD.
type RuntimeResourceRef struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
	UID       string
}

// Key returns a stable identity suitable for topology nodes and edges.
func (r RuntimeResourceRef) Key() string {
	return strings.Join([]string{r.Group, r.Version, r.Kind, r.Namespace, r.Name}, "/")
}

// Validate rejects incomplete resource identities before a vendor request is created.
func (r RuntimeResourceRef) Validate() error {
	if strings.TrimSpace(r.Version) == "" || strings.TrimSpace(r.Kind) == "" || strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("resource version, kind, and name are required")
	}
	return nil
}

// RuntimeInfo is one human-readable item reported by Argo CD.
type RuntimeInfo struct {
	Name  string
	Value string
}

// RuntimeNetworking contains only relationships explicitly reported by Argo CD.
type RuntimeNetworking struct {
	TargetRefs   []RuntimeResourceRef
	Ingress      []string
	ExternalURLs []string
}

// RuntimeResource is the normalized read-only representation of one live resource.
type RuntimeResource struct {
	Ref           RuntimeResourceRef
	ParentRefs    []RuntimeResourceRef
	Info          []RuntimeInfo
	Networking    RuntimeNetworking
	Images        []string
	HealthStatus  string
	HealthMessage string
	CreatedAt     *time.Time
	Orphaned      bool
}

// RuntimeTree is one point-in-time Application resource observation.
type RuntimeTree struct {
	Resources  []RuntimeResource
	ObservedAt time.Time
}

// RuntimeEvent is a bounded Kubernetes Event projection.
type RuntimeEvent struct {
	Type          string
	Reason        string
	Message       string
	Count         int32
	FirstObserved *time.Time
	LastObserved  *time.Time
}

// RuntimeLogEntry is one entry returned by Argo CD's PodLogs stream.
type RuntimeLogEntry struct {
	Content   string
	Timestamp string
	PodName   string
}

// RuntimeLogQuery defines the bounded log snapshot supported by ReleaseHub.
type RuntimeLogQuery struct {
	Resource  RuntimeResourceRef
	Container string
	TailLines int64
}
