package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// VisibilityResolver projects the existing active explicit-deny policy into query constraints.
type VisibilityResolver struct{ db *gorm.DB }

type denyScopeRow struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
	ScopeKind      string
}

var _ auditapp.VisibilityResolver = (*VisibilityResolver)(nil)
var _ auditapp.CapabilityResolver = (*VisibilityResolver)(nil)

// NewVisibilityResolver creates the deny projection adapter.
func NewVisibilityResolver(db *gorm.DB) (*VisibilityResolver, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &VisibilityResolver{db: db}, nil
}

// ListRoots returns active audit.view role-binding roots with presentation labels.
func (r *VisibilityResolver) ListRoots(ctx context.Context, userID uuid.UUID) ([]auditapp.ScopeRoot, error) {
	var rows []struct {
		OrganizationID uuid.UUID
		ProjectID      uuid.UUID
		EnvironmentID  *uuid.UUID
		ApplicationID  *uuid.UUID
		ScopeKind      string
		Label          string
	}
	err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT binding.organization_id, binding.project_id,
binding.environment_id, binding.application_id, binding.scope_kind,
CASE binding.scope_kind
  WHEN 'project' THEN project.name
  WHEN 'environment' THEN project.name || ' / ' || environment.name
  ELSE project.name || ' / ' || environment.name || ' / ' || application.name
END AS label
FROM authorization_group_memberships membership
JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
JOIN authorization_group_role_bindings binding ON binding.group_id = auth_group.id
JOIN authorization_roles auth_role ON auth_role.id = binding.role_id
JOIN authorization_role_permissions role_permission ON role_permission.role_id = auth_role.id
JOIN users auth_user ON auth_user.id = membership.user_id
JOIN projects project ON project.id = binding.project_id AND project.active
LEFT JOIN environments environment ON environment.id = binding.environment_id AND environment.active
LEFT JOIN applications application ON application.id = binding.application_id AND application.active
WHERE membership.user_id = ? AND membership.active AND auth_group.disabled_at IS NULL
  AND binding.active AND auth_role.active AND role_permission.permission_key = 'audit.view'
  AND auth_user.disabled_at IS NULL
  AND (membership.source = 'manual' OR NOT auth_group.oidc_viewer_only)
ORDER BY label`, userID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load audit capability roots: %w", err)
	}
	result := make([]auditapp.ScopeRoot, 0, len(rows))
	for _, row := range rows {
		scope, scopeErr := denyScope(denyScopeRow{OrganizationID: row.OrganizationID, ProjectID: row.ProjectID,
			EnvironmentID: row.EnvironmentID, ApplicationID: row.ApplicationID, ScopeKind: row.ScopeKind})
		if scopeErr != nil {
			return nil, scopeErr
		}
		result = append(result, auditapp.ScopeRoot{Scope: scope, Label: row.Label})
	}
	return result, nil
}

// Resolve returns lower-scope explicit denies for one already-authorized root.
func (r *VisibilityResolver) Resolve(ctx context.Context, userID uuid.UUID, root authz.Scope) (auditapp.VisibilityConstraint, error) {
	constraint := auditapp.VisibilityConstraint{Root: root}
	if root.Kind == authz.ScopePlatform {
		return constraint, nil
	}
	var rows []denyScopeRow
	err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT deny.organization_id, deny.project_id, deny.environment_id,
deny.application_id, deny.scope_kind
FROM authorization_group_memberships membership
JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
JOIN authorization_deny_policies deny ON deny.group_id = auth_group.id
JOIN users auth_user ON auth_user.id = membership.user_id
WHERE membership.user_id = ? AND membership.active AND auth_group.disabled_at IS NULL
  AND deny.active AND deny.permission_key = 'audit.view' AND auth_user.disabled_at IS NULL`, userID).Scan(&rows).Error
	if err != nil {
		return auditapp.VisibilityConstraint{}, fmt.Errorf("load audit deny scopes: %w", err)
	}
	for _, row := range rows {
		scope, scopeErr := denyScope(row)
		if scopeErr != nil {
			return auditapp.VisibilityConstraint{}, scopeErr
		}
		if root.Contains(scope) {
			constraint.Denied = append(constraint.Denied, scope)
		}
	}
	return constraint, nil
}

func denyScope(row denyScopeRow) (authz.Scope, error) {
	switch authz.ScopeKind(row.ScopeKind) {
	case authz.ScopeProject:
		return authz.NewProjectScope(row.OrganizationID, row.ProjectID)
	case authz.ScopeEnvironment:
		if row.EnvironmentID == nil {
			return authz.Scope{}, errors.New("audit deny Environment is incomplete")
		}
		return authz.NewEnvironmentScope(row.OrganizationID, row.ProjectID, *row.EnvironmentID)
	case authz.ScopeApplication:
		if row.EnvironmentID == nil || row.ApplicationID == nil {
			return authz.Scope{}, errors.New("audit deny Application is incomplete")
		}
		return authz.NewApplicationScope(row.OrganizationID, row.ProjectID, *row.EnvironmentID, *row.ApplicationID)
	default:
		return authz.Scope{}, errors.New("audit deny scope kind is invalid")
	}
}
