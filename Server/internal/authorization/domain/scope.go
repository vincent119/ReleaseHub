// Package domain contains authorization rules without framework dependencies.
package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ScopeKind identifies the resource level at which a policy is assigned.
type ScopeKind string

const (
	ScopePlatform    ScopeKind = "platform"
	ScopeProject     ScopeKind = "project"
	ScopeEnvironment ScopeKind = "environment"
	ScopeApplication ScopeKind = "application"
)

// NewPlatformScope creates the global authorization scope.
func NewPlatformScope() Scope { return Scope{Kind: ScopePlatform} }

// Scope is the complete tenant-aware path of one protected resource.
type Scope struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ApplicationID  uuid.UUID
	Kind           ScopeKind
}

// NewProjectScope creates a Project-level authorization scope.
func NewProjectScope(organizationID, projectID uuid.UUID) (Scope, error) {
	return newScope(ScopeProject, organizationID, projectID, uuid.Nil, uuid.Nil)
}

// NewEnvironmentScope creates an Environment-level authorization scope.
func NewEnvironmentScope(organizationID, projectID, environmentID uuid.UUID) (Scope, error) {
	return newScope(ScopeEnvironment, organizationID, projectID, environmentID, uuid.Nil)
}

// NewApplicationScope creates an Application-level authorization scope.
func NewApplicationScope(organizationID, projectID, environmentID, applicationID uuid.UUID) (Scope, error) {
	return newScope(ScopeApplication, organizationID, projectID, environmentID, applicationID)
}

func newScope(kind ScopeKind, organizationID, projectID, environmentID, applicationID uuid.UUID) (Scope, error) {
	if kind == ScopePlatform {
		if organizationID != uuid.Nil || projectID != uuid.Nil || environmentID != uuid.Nil || applicationID != uuid.Nil {
			return Scope{}, fmt.Errorf("platform scope cannot contain resource IDs")
		}
		return Scope{Kind: kind}, nil
	}
	if organizationID == uuid.Nil || projectID == uuid.Nil {
		return Scope{}, fmt.Errorf("organization and project IDs are required")
	}
	switch kind {
	case ScopeProject:
		if environmentID != uuid.Nil || applicationID != uuid.Nil {
			return Scope{}, fmt.Errorf("project scope cannot contain descendant IDs")
		}
	case ScopeEnvironment:
		if environmentID == uuid.Nil || applicationID != uuid.Nil {
			return Scope{}, fmt.Errorf("environment scope requires an environment ID only")
		}
	case ScopeApplication:
		if environmentID == uuid.Nil || applicationID == uuid.Nil {
			return Scope{}, fmt.Errorf("application scope requires environment and application IDs")
		}
	default:
		return Scope{}, fmt.Errorf("unsupported scope kind %q", kind)
	}
	return Scope{OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID, ApplicationID: applicationID, Kind: kind}, nil
}

// Contains reports whether this policy scope includes the target resource.
func (s Scope) Contains(target Scope) bool {
	if s.Kind == ScopePlatform {
		return target.Kind == ScopePlatform
	}
	if s.OrganizationID != target.OrganizationID || s.ProjectID != target.ProjectID {
		return false
	}
	switch s.Kind {
	case ScopeProject:
		return true
	case ScopeEnvironment:
		return s.EnvironmentID == target.EnvironmentID
	case ScopeApplication:
		return s.EnvironmentID == target.EnvironmentID && s.ApplicationID == target.ApplicationID
	default:
		return false
	}
}

// Path returns the canonical Casbin object path for this scope.
func (s Scope) Path() string {
	if s.Kind == ScopePlatform {
		return "/platform"
	}
	parts := []string{"organizations", s.OrganizationID.String(), "projects", s.ProjectID.String()}
	if s.Kind == ScopeEnvironment || s.Kind == ScopeApplication {
		parts = append(parts, "environments", s.EnvironmentID.String())
	}
	if s.Kind == ScopeApplication {
		parts = append(parts, "applications", s.ApplicationID.String())
	}
	return "/" + strings.Join(parts, "/")
}

// Tenant returns the Casbin tenant key for this scope.
func (s Scope) Tenant() string {
	if s.Kind == ScopePlatform {
		return "platform"
	}
	return s.OrganizationID.String()
}
