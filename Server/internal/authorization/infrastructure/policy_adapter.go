package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"gorm.io/gorm"
)

var errReadOnlyPolicyAdapter = errors.New("authorization policies must be changed through the policy manager")

// PolicyAdapter loads flattened user policies from the normalized authorization tables.
type PolicyAdapter struct {
	db          *gorm.DB
	loadTimeout time.Duration
}

// NewPolicyAdapter creates a read-only Casbin adapter backed by PostgreSQL and GORM.
func NewPolicyAdapter(db *gorm.DB) (*PolicyAdapter, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &PolicyAdapter{db: db, loadTimeout: 10 * time.Second}, nil
}

// LoadPolicy loads policy during Casbin construction with a bounded startup context.
func (a *PolicyAdapter) LoadPolicy(target model.Model) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.loadTimeout)
	defer cancel()
	return a.LoadPolicyCtx(ctx, target)
}

// LoadPolicyCtx loads effective allow and deny rules without exposing GORM models to the domain.
func (a *PolicyAdapter) LoadPolicyCtx(ctx context.Context, target model.Model) error {
	var rows []policyRow
	if err := a.db.WithContext(ctx).Raw(effectivePolicyQuery).Scan(&rows).Error; err != nil {
		return fmt.Errorf("load authorization policy: %w", err)
	}
	for _, row := range rows {
		if err := persist.LoadPolicyArray([]string{"p", row.UserID, row.OrganizationID, row.ScopePath, row.Permission, row.Effect}, target); err != nil {
			return fmt.Errorf("load authorization policy row: %w", err)
		}
	}
	return nil
}

// CurrentRevision returns the committed PostgreSQL policy revision.
func (a *PolicyAdapter) CurrentRevision(ctx context.Context) (uint64, error) {
	var revision uint64
	if err := a.db.WithContext(ctx).Table("authorization_policy_revision").Select("revision").Where("singleton = true").Scan(&revision).Error; err != nil {
		return 0, fmt.Errorf("load authorization policy revision: %w", err)
	}
	return revision, nil
}

func (*PolicyAdapter) SavePolicy(model.Model) error { return errReadOnlyPolicyAdapter }
func (*PolicyAdapter) AddPolicy(string, string, []string) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) RemovePolicy(string, string, []string) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) SavePolicyCtx(context.Context, model.Model) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) AddPolicyCtx(context.Context, string, string, []string) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) RemovePolicyCtx(context.Context, string, string, []string) error {
	return errReadOnlyPolicyAdapter
}
func (*PolicyAdapter) RemoveFilteredPolicyCtx(context.Context, string, string, int, ...string) error {
	return errReadOnlyPolicyAdapter
}

type policyRow struct {
	UserID         string
	OrganizationID string
	ScopePath      string
	Permission     string
	Effect         string
}

const effectivePolicyQuery = `
WITH platform_allow_rules AS (
    SELECT DISTINCT membership.user_id::text AS user_id, 'platform' AS organization_id,
        '/platform' AS scope_path, role_permission.permission_key AS permission, 'allow' AS effect
    FROM authorization_group_memberships membership
    JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
    JOIN authorization_platform_role_bindings binding ON binding.group_id = auth_group.id
    JOIN authorization_roles auth_role ON auth_role.id = binding.role_id
    JOIN authorization_role_permissions role_permission ON role_permission.role_id = auth_role.id
    JOIN users auth_user ON auth_user.id = membership.user_id
    WHERE membership.active AND membership.source = 'manual' AND auth_group.disabled_at IS NULL
      AND binding.active AND auth_role.active AND auth_user.disabled_at IS NULL
), allow_rules AS (
    SELECT DISTINCT
        membership.user_id::text AS user_id,
        binding.organization_id::text AS organization_id,
        '/organizations/' || binding.organization_id::text ||
            '/projects/' || binding.project_id::text ||
            CASE WHEN binding.environment_id IS NULL THEN '' ELSE '/environments/' || binding.environment_id::text END ||
            CASE WHEN binding.application_id IS NULL THEN '' ELSE '/applications/' || binding.application_id::text END AS scope_path,
        role_permission.permission_key AS permission,
        'allow' AS effect
    FROM authorization_group_memberships membership
    JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
    JOIN authorization_group_role_bindings binding ON binding.group_id = auth_group.id
    JOIN authorization_roles auth_role ON auth_role.id = binding.role_id
    JOIN authorization_role_permissions role_permission ON role_permission.role_id = auth_role.id
    JOIN users auth_user ON auth_user.id = membership.user_id
    WHERE membership.active
      AND auth_group.disabled_at IS NULL
      AND binding.active
      AND auth_role.active
      AND auth_user.disabled_at IS NULL
      AND (membership.source = 'manual' OR (auth_group.oidc_viewer_only AND role_permission.permission_key = 'resource.view'))
), deny_rules AS (
    SELECT DISTINCT
        membership.user_id::text AS user_id,
        deny.organization_id::text AS organization_id,
        '/organizations/' || deny.organization_id::text ||
            '/projects/' || deny.project_id::text ||
            CASE WHEN deny.environment_id IS NULL THEN '' ELSE '/environments/' || deny.environment_id::text END ||
            CASE WHEN deny.application_id IS NULL THEN '' ELSE '/applications/' || deny.application_id::text END AS scope_path,
        deny.permission_key AS permission,
        'deny' AS effect
    FROM authorization_group_memberships membership
    JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
    JOIN authorization_deny_policies deny ON deny.group_id = auth_group.id
    JOIN users auth_user ON auth_user.id = membership.user_id
    WHERE membership.active
      AND auth_group.disabled_at IS NULL
      AND deny.active
      AND auth_user.disabled_at IS NULL
)
SELECT user_id, organization_id, scope_path, permission, effect FROM platform_allow_rules
UNION
SELECT user_id, organization_id, scope_path, permission, effect FROM allow_rules
UNION
SELECT user_id, organization_id, scope_path, permission, effect FROM deny_rules
ORDER BY user_id, organization_id, scope_path, permission, effect`
