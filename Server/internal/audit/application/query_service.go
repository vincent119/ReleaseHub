package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

var (
	ErrForbidden = errors.New("audit trail is not available")
	ErrNotFound  = errors.New("audit event was not found")
)

// QueryPage is one authorized stable Audit Trail page.
type QueryPage struct {
	Items      []auditdomain.EventSummary
	NextCursor string
	HasMore    bool
}

// Capabilities contains only presentation-safe Audit Trail access roots.
type Capabilities struct {
	Visible    bool
	ScopeRoots []ScopeRoot
}

// QueryService performs fresh authorization before every Audit Trail read.
type QueryService struct {
	repository Repository
	authorizer Authorizer
	visibility VisibilityResolver
	capability CapabilityResolver
	cursors    *auditdomain.CursorCodec
	view       authz.Permission
	now        func() time.Time
}

// ResolveScope validates one public scope kind and identifier against catalog ancestry.
func (s *QueryService) ResolveScope(ctx context.Context, kind authz.ScopeKind, id *uuid.UUID) (authz.Scope, error) {
	scope, err := s.repository.ResolveScope(ctx, kind, id)
	if err != nil {
		return authz.Scope{}, maskNotFound(err)
	}
	return scope, nil
}

// Capabilities returns fresh-authorized scope roots without exposing policy composition.
func (s *QueryService) Capabilities(ctx context.Context, principal Principal) (Capabilities, error) {
	platform := authz.NewPlatformScope()
	allowed, err := s.allowed(ctx, principal, platform)
	if err != nil {
		return Capabilities{}, err
	}
	if allowed {
		return Capabilities{Visible: true, ScopeRoots: []ScopeRoot{{Scope: platform, Label: "Platform"}}}, nil
	}
	candidates, err := s.capability.ListRoots(ctx, principal.UserID)
	if err != nil {
		return Capabilities{}, fmt.Errorf("list audit capability roots: %w", err)
	}
	result := Capabilities{ScopeRoots: make([]ScopeRoot, 0, len(candidates))}
	for _, candidate := range candidates {
		allowed, authorizeErr := s.allowed(ctx, principal, candidate.Scope)
		if authorizeErr != nil {
			return Capabilities{}, authorizeErr
		}
		if allowed {
			result.ScopeRoots = append(result.ScopeRoots, candidate)
		}
	}
	result.Visible = len(result.ScopeRoots) > 0
	return result, nil
}

// NewQueryService creates the Audit Trail read use case.
func NewQueryService(repository Repository, authorizer Authorizer, visibility VisibilityResolver, capability CapabilityResolver, cursors *auditdomain.CursorCodec) (*QueryService, error) {
	if repository == nil || authorizer == nil || visibility == nil || capability == nil || cursors == nil {
		return nil, errors.New("audit query dependencies are required")
	}
	permission, err := authz.NewPermission("audit.view")
	if err != nil {
		return nil, err
	}
	return &QueryService{repository: repository, authorizer: authorizer, visibility: visibility, capability: capability, cursors: cursors, view: permission, now: time.Now}, nil
}

// List returns one metadata-free page after applying authorization in SQL.
func (s *QueryService) List(ctx context.Context, principal Principal, filter auditdomain.QueryFilter, cursor string) (QueryPage, error) {
	normalized, err := filter.Normalize(s.now())
	if err != nil {
		return QueryPage{}, err
	}
	if err := s.authorize(ctx, principal, normalized.Scope); err != nil {
		return QueryPage{}, err
	}
	visibility, err := s.visibility.Resolve(ctx, principal.UserID, normalized.Scope)
	if err != nil {
		return QueryPage{}, fmt.Errorf("resolve audit visibility: %w", err)
	}
	position, err := s.cursors.Decode(cursor, normalized.Fingerprint())
	if err != nil {
		return QueryPage{}, err
	}
	page, err := s.repository.List(ctx, ListQuery{Filter: normalized, Position: position, Visibility: visibility})
	if err != nil {
		return QueryPage{}, fmt.Errorf("list audit events: %w", err)
	}
	next := ""
	if page.HasMore && len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		next, err = s.cursors.Encode(auditdomain.CursorPosition{OccurredAt: last.OccurredAt, EventID: last.ID}, normalized.Fingerprint())
		if err != nil {
			return QueryPage{}, err
		}
	}
	return QueryPage{Items: page.Items, NextCursor: next, HasMore: page.HasMore}, nil
}

// FilterOptions returns bounded autocomplete values after fresh authorization and visibility projection.
func (s *QueryService) FilterOptions(ctx context.Context, principal Principal, filter auditdomain.FilterOptionFilter) ([]FilterOption, error) {
	normalized, err := filter.Normalize(s.now())
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, principal, normalized.Scope); err != nil {
		return nil, err
	}
	visibility, err := s.visibility.Resolve(ctx, principal.UserID, normalized.Scope)
	if err != nil {
		return nil, fmt.Errorf("resolve audit visibility: %w", err)
	}
	options, err := s.repository.FilterOptions(ctx, normalized, visibility)
	if err != nil {
		return nil, fmt.Errorf("list audit filter options: %w", err)
	}
	return options, nil
}

// Detail returns one authorized event with defensively sanitized metadata.
func (s *QueryService) Detail(ctx context.Context, principal Principal, eventID uuid.UUID) (auditdomain.EventDetail, error) {
	if eventID == uuid.Nil {
		return auditdomain.EventDetail{}, ErrNotFound
	}
	ancestry, err := s.repository.Locate(ctx, eventID)
	if err != nil {
		return auditdomain.EventDetail{}, maskNotFound(err)
	}
	platform := authz.NewPlatformScope()
	platformAllowed, err := s.allowed(ctx, principal, platform)
	if err != nil {
		return auditdomain.EventDetail{}, err
	}
	target := platform
	if !platformAllowed {
		var ok bool
		target, ok = ancestry.AuthorizationScope()
		if !ok || target.Kind == authz.ScopePlatform {
			return auditdomain.EventDetail{}, ErrNotFound
		}
		if err := s.authorize(ctx, principal, target); err != nil {
			return auditdomain.EventDetail{}, maskForbidden(err)
		}
	}
	visibility, err := s.visibility.Resolve(ctx, principal.UserID, target)
	if err != nil {
		return auditdomain.EventDetail{}, fmt.Errorf("resolve audit visibility: %w", err)
	}
	stored, err := s.repository.Get(ctx, eventID, visibility)
	if err != nil {
		return auditdomain.EventDetail{}, maskNotFound(err)
	}
	metadata, truncated := auditdomain.SanitizeMetadata(stored.Metadata)
	return auditdomain.EventDetail{EventSummary: stored.EventSummary, Metadata: metadata, MetadataTruncated: truncated}, nil
}

func (s *QueryService) authorize(ctx context.Context, principal Principal, scope authz.Scope) error {
	allowed, err := s.allowed(ctx, principal, scope)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *QueryService) allowed(ctx context.Context, principal Principal, scope authz.Scope) (bool, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.view, Scope: scope,
	})
	if err != nil {
		return false, fmt.Errorf("authorize audit query: %w", err)
	}
	return allowed, nil
}

func maskForbidden(err error) error {
	if errors.Is(err, ErrForbidden) {
		return ErrNotFound
	}
	return err
}

func maskNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}
