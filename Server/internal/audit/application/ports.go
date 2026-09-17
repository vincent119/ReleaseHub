// Package application coordinates authorized Audit Trail queries.
package application

import (
	"context"

	"github.com/google/uuid"

	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// Principal is the authenticated identity used for fresh authorization.
type Principal struct {
	UserID   uuid.UUID
	Disabled bool
}

// VisibilityConstraint is the SQL-ready root and explicit-deny projection.
type VisibilityConstraint struct {
	Root   authz.Scope
	Denied []authz.Scope
}

// ListQuery carries normalized filters and a verified keyset position.
type ListQuery struct {
	Filter     auditdomain.QueryFilter
	Position   *auditdomain.CursorPosition
	Visibility VisibilityConstraint
}

// StoredPage is one repository page before the next cursor is encoded.
type StoredPage struct {
	Items   []auditdomain.EventSummary
	HasMore bool
}

// FilterOption is one safe autocomplete value and presentation label.
type FilterOption struct {
	Value string
	Label string
}

// Repository is the read-only Audit Trail storage port.
type Repository interface {
	ResolveScope(context.Context, authz.ScopeKind, *uuid.UUID) (authz.Scope, error)
	List(context.Context, ListQuery) (StoredPage, error)
	FilterOptions(context.Context, auditdomain.FilterOptionFilter, VisibilityConstraint) ([]FilterOption, error)
	Locate(context.Context, uuid.UUID) (auditdomain.ScopeAncestry, error)
	Get(context.Context, uuid.UUID, VisibilityConstraint) (auditdomain.StoredDetail, error)
}

// ScopeRoot is one server-owned Audit Trail scope option.
type ScopeRoot struct {
	Scope authz.Scope
	Label string
}

// CapabilityResolver returns candidate roots granted by active role bindings.
type CapabilityResolver interface {
	ListRoots(context.Context, uuid.UUID) ([]ScopeRoot, error)
}

// Authorizer evaluates the existing authorization policy engine.
type Authorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// VisibilityResolver projects active explicit denies into SQL-ready constraints.
type VisibilityResolver interface {
	Resolve(context.Context, uuid.UUID, authz.Scope) (VisibilityConstraint, error)
}
