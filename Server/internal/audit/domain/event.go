// Package domain defines the framework-independent Audit Trail read model.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// ScopeResolution identifies whether an event has trustworthy resource ancestry.
type ScopeResolution string

const (
	ScopeResolved   ScopeResolution = "resolved"
	ScopeUnresolved ScopeResolution = "unresolved"
)

// ScopeAncestry is the immutable scope snapshot stored with an audit event.
type ScopeAncestry struct {
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	Resolution     ScopeResolution
}

// AuthorizationScope returns the most specific trustworthy authorization scope.
func (s ScopeAncestry) AuthorizationScope() (authz.Scope, bool) {
	if s.Resolution != ScopeResolved {
		return authz.Scope{}, false
	}
	if s.ApplicationID != nil && s.EnvironmentID != nil && s.ProjectID != nil && s.OrganizationID != nil {
		value, err := authz.NewApplicationScope(*s.OrganizationID, *s.ProjectID, *s.EnvironmentID, *s.ApplicationID)
		return value, err == nil
	}
	if s.EnvironmentID != nil && s.ProjectID != nil && s.OrganizationID != nil {
		value, err := authz.NewEnvironmentScope(*s.OrganizationID, *s.ProjectID, *s.EnvironmentID)
		return value, err == nil
	}
	if s.ProjectID != nil && s.OrganizationID != nil {
		value, err := authz.NewProjectScope(*s.OrganizationID, *s.ProjectID)
		return value, err == nil
	}
	return authz.NewPlatformScope(), true
}

// EventSummary is the metadata-free representation returned by list queries.
type EventSummary struct {
	ID               uuid.UUID
	OccurredAt       time.Time
	ActorID          *uuid.UUID
	ActorDisplayName string
	Action           string
	ResourceType     string
	ResourceID       string
	Scope            ScopeAncestry
	RequestID        string
	HasMetadata      bool
}

// EventDetail contains sanitized metadata for one authorized event.
type EventDetail struct {
	EventSummary
	Metadata          map[string]any
	MetadataTruncated bool
}

// StoredDetail is the repository result before defensive metadata sanitization.
type StoredDetail struct {
	EventSummary
	Metadata json.RawMessage
}
