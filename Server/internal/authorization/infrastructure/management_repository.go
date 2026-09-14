package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// AccessManagementRepository reads access-management records without applying presentation authorization.
type AccessManagementRepository struct{ db *gorm.DB }

// NewAccessManagementRepository creates the PostgreSQL access-management read adapter.
func NewAccessManagementRepository(db *gorm.DB) (*AccessManagementRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &AccessManagementRepository{db: db}, nil
}

func (r *AccessManagementRepository) ListProjectScopes(ctx context.Context) ([]authzapp.ProjectScope, error) {
	var values []authzapp.ProjectScope
	if err := r.db.WithContext(ctx).Raw(`SELECT organization_id, id AS project_id FROM projects WHERE active ORDER BY organization_id, id`).Scan(&values).Error; err != nil {
		return nil, fmt.Errorf("list active Project scopes: %w", err)
	}
	return values, nil
}

// ResolveAccessScope validates one catalog resource and returns its authorization scope.
func (r *AccessManagementRepository) ResolveAccessScope(ctx context.Context, kind string, id uuid.UUID) (authz.Scope, error) {
	var row struct {
		OrganizationID uuid.UUID
		ProjectID      uuid.UUID
		EnvironmentID  uuid.UUID
	}
	switch kind {
	case "project":
		if err := r.db.WithContext(ctx).Raw(`SELECT organization_id, id AS project_id FROM projects WHERE id = ? AND active`, id).Scan(&row).Error; err != nil {
			return authz.Scope{}, err
		}
		return authz.NewProjectScope(row.OrganizationID, row.ProjectID)
	case "environment":
		if err := r.db.WithContext(ctx).Raw(`SELECT organization_id, project_id, id AS environment_id FROM environments WHERE id = ? AND active`, id).Scan(&row).Error; err != nil {
			return authz.Scope{}, err
		}
		return authz.NewEnvironmentScope(row.OrganizationID, row.ProjectID, row.EnvironmentID)
	case "application":
		if err := r.db.WithContext(ctx).Raw(`SELECT organization_id, project_id, environment_id FROM applications WHERE id = ? AND active`, id).Scan(&row).Error; err != nil {
			return authz.Scope{}, err
		}
		return authz.NewApplicationScope(row.OrganizationID, row.ProjectID, row.EnvironmentID, id)
	default:
		return authz.Scope{}, gorm.ErrRecordNotFound
	}
}

// FindMembershipCandidates returns minimal identities and excludes disabled or existing members.
func (r *AccessManagementRepository) FindMembershipCandidates(ctx context.Context, groupID uuid.UUID, query string, after *authzapp.MembershipCandidateCursor, limit int) ([]authzapp.AccessUser, error) {
	afterUsername := ""
	afterUserID := uuid.Nil
	hasCursor := after != nil
	if after != nil {
		afterUsername = after.Username
		afterUserID = after.UserID
	}
	values := []authzapp.AccessUser{}
	err := r.db.WithContext(ctx).Raw(`
SELECT users.id, users.username, users.disabled_at
FROM users
WHERE users.disabled_at IS NULL
  AND (? = '' OR lower(users.username) LIKE lower(?) || '%')
  AND (? = false OR lower(users.username) > ? OR (lower(users.username) = ? AND users.id > ?))
  AND NOT EXISTS (
    SELECT 1 FROM authorization_group_memberships memberships
    WHERE memberships.group_id = ? AND memberships.user_id = users.id AND memberships.active
  )
ORDER BY lower(users.username), users.id
LIMIT ?`, query, query, hasCursor, afterUsername, afterUsername, afterUserID, groupID, limit).Scan(&values).Error
	if err != nil {
		return nil, fmt.Errorf("find Group membership candidates: %w", err)
	}
	return values, nil
}

func (r *AccessManagementRepository) LoadAccessSnapshot(ctx context.Context) (authzapp.AccessSnapshot, error) {
	value := authzapp.AccessSnapshot{Users: []authzapp.AccessUser{}, Groups: []authzapp.AccessGroup{}, Roles: []authzapp.AccessRole{}, Permissions: []authzapp.AccessPermission{}, Memberships: []authzapp.AccessMembership{}, Bindings: []authzapp.AccessBinding{}, Denies: []authzapp.AccessDeny{}}
	queries := []struct {
		label  string
		query  string
		target any
	}{
		{"users", `SELECT id, username, disabled_at FROM users ORDER BY lower(username), id`, &value.Users},
		{"groups", `SELECT id, owner_kind, owner_id, name, system_key, oidc_viewer_only, disabled_at FROM authorization_groups ORDER BY owner_kind, lower(name), id`, &value.Groups},
		{"permissions", `SELECT key, platform_only, project_role_delegable FROM authorization_permissions ORDER BY key`, &value.Permissions},
		{"memberships", `SELECT id, group_id, user_id, source, active FROM authorization_group_memberships ORDER BY group_id, user_id, id`, &value.Memberships},
		{"bindings", `SELECT id, group_id, role_id, organization_id, scope_kind, project_id, environment_id, application_id, active
FROM authorization_group_role_bindings
UNION ALL
SELECT id, group_id, role_id, '00000000-0000-0000-0000-000000000000'::uuid AS organization_id,
       'platform' AS scope_kind, '00000000-0000-0000-0000-000000000000'::uuid AS project_id,
       NULL::uuid AS environment_id, NULL::uuid AS application_id, active
FROM authorization_platform_role_bindings
ORDER BY scope_kind, organization_id, project_id, id`, &value.Bindings},
		{"deny policies", `SELECT id, group_id, permission_key AS permission, organization_id, scope_kind, project_id, environment_id, application_id, active FROM authorization_deny_policies ORDER BY organization_id, project_id, scope_kind, id`, &value.Denies},
	}
	for _, query := range queries {
		if err := r.db.WithContext(ctx).Raw(query.query).Scan(query.target).Error; err != nil {
			return authzapp.AccessSnapshot{}, fmt.Errorf("list access management %s: %w", query.label, err)
		}
	}
	type roleRow struct {
		ID        uuid.UUID
		OwnerKind string
		OwnerID   *uuid.UUID
		Name      string
		SystemKey *string
		Active    bool
	}
	var roles []roleRow
	if err := r.db.WithContext(ctx).Raw(`SELECT id, owner_kind, owner_id, name, system_key, active FROM authorization_roles ORDER BY owner_kind, lower(name), id`).Scan(&roles).Error; err != nil {
		return authzapp.AccessSnapshot{}, fmt.Errorf("list access management roles: %w", err)
	}
	for _, role := range roles {
		value.Roles = append(value.Roles, authzapp.AccessRole{ID: role.ID, OwnerKind: role.OwnerKind, OwnerID: role.OwnerID, Name: role.Name, SystemKey: role.SystemKey, Active: role.Active})
	}
	type rolePermission struct {
		RoleID     uuid.UUID
		Permission string
	}
	var permissions []rolePermission
	if err := r.db.WithContext(ctx).Raw(`SELECT role_id, permission_key AS permission FROM authorization_role_permissions ORDER BY role_id, permission_key`).Scan(&permissions).Error; err != nil {
		return authzapp.AccessSnapshot{}, fmt.Errorf("list Role permissions: %w", err)
	}
	byRole := make(map[uuid.UUID][]string)
	for _, permission := range permissions {
		byRole[permission.RoleID] = append(byRole[permission.RoleID], permission.Permission)
	}
	for index := range value.Roles {
		value.Roles[index].Permissions = byRole[value.Roles[index].ID]
		if value.Roles[index].Permissions == nil {
			value.Roles[index].Permissions = []string{}
		}
	}
	return value, nil
}

// CreateGroup persists a Group together with its audit and outbox records.
func (r *AccessManagementRepository) CreateGroup(ctx context.Context, mutation authzapp.AccessMutation, input authzapp.CreateGroupInput) (authzapp.AccessGroup, error) {
	value := authzapp.AccessGroup{ID: uuid.New(), OwnerKind: input.OwnerKind, OwnerID: input.OwnerID, Name: input.Name, OIDCViewerOnly: input.OIDCViewerOnly}
	organizationID, err := r.organizationForOwner(ctx, input.OwnerKind, input.OwnerID)
	if err != nil {
		return authzapp.AccessGroup{}, err
	}
	err = r.mutate(ctx, mutation, organizationID, "authorization_group", value.ID, "authorization.group.created", map[string]any{"ownerKind": input.OwnerKind}, func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Exec(`INSERT INTO authorization_groups (id, owner_kind, owner_id, name, oidc_viewer_only) VALUES (?, ?, ?, ?, ?)`, value.ID, value.OwnerKind, value.OwnerID, value.Name, value.OIDCViewerOnly).Error
	})
	if err != nil {
		return authzapp.AccessGroup{}, fmt.Errorf("create Group: %w", err)
	}
	return value, nil
}

// DisableGroup disables a Group and lets the database revoke its active policies atomically.
func (r *AccessManagementRepository) DisableGroup(ctx context.Context, mutation authzapp.AccessMutation, groupID uuid.UUID) error {
	organizationID, err := r.organizationForGroup(ctx, groupID)
	if err != nil {
		return err
	}
	return r.mutateProtected(ctx, mutation, organizationID, "authorization_group", groupID, "authorization.group.disabled", map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Exec(`UPDATE authorization_groups SET disabled_at = now(), updated_at = now() WHERE id = ? AND disabled_at IS NULL`, groupID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// CreateRole persists a Role and its initial permission set in one transaction.
func (r *AccessManagementRepository) CreateRole(ctx context.Context, mutation authzapp.AccessMutation, input authzapp.CreateRoleInput) (authzapp.AccessRole, error) {
	value := authzapp.AccessRole{ID: uuid.New(), OwnerKind: input.OwnerKind, OwnerID: input.OwnerID, Name: input.Name, Active: true, Permissions: append([]string(nil), input.Permissions...)}
	organizationID, err := r.organizationForOwner(ctx, input.OwnerKind, input.OwnerID)
	if err != nil {
		return authzapp.AccessRole{}, err
	}
	err = r.mutate(ctx, mutation, organizationID, "authorization_role", value.ID, "authorization.role.created", map[string]any{"ownerKind": input.OwnerKind, "permissions": input.Permissions}, func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Exec(`INSERT INTO authorization_roles (id, owner_kind, owner_id, name) VALUES (?, ?, ?, ?)`, value.ID, value.OwnerKind, value.OwnerID, value.Name).Error; err != nil {
			return err
		}
		for _, permission := range value.Permissions {
			if err := tx.WithContext(ctx).Exec(`INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES (?, ?)`, value.ID, permission).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return authzapp.AccessRole{}, fmt.Errorf("create Role: %w", err)
	}
	return value, nil
}

// AddMembership creates one active manual Group membership.
func (r *AccessManagementRepository) AddMembership(ctx context.Context, mutation authzapp.AccessMutation, groupID, userID uuid.UUID) (authzapp.AccessMembership, error) {
	value := authzapp.AccessMembership{ID: uuid.New(), GroupID: groupID, UserID: userID, Source: "manual", Active: true}
	organizationID, err := r.organizationForGroup(ctx, groupID)
	if err != nil {
		return authzapp.AccessMembership{}, err
	}
	metadata := map[string]any{"groupId": groupID.String(), "userId": userID.String()}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return authzapp.AccessMembership{}, fmt.Errorf("marshal Group membership event: %w", err)
	}
	now := time.Now().UTC()
	err = database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var existing struct {
			ID     uuid.UUID
			Active bool
		}
		if err := tx.WithContext(ctx).Raw(`
SELECT id, active
FROM authorization_group_memberships
WHERE group_id = ? AND user_id = ? AND source = 'manual'
FOR UPDATE`, groupID, userID).Scan(&existing).Error; err != nil {
			return fmt.Errorf("find existing manual Group membership: %w", err)
		}

		eventType := "authorization.group_membership.created"
		if existing.ID != uuid.Nil {
			if existing.Active {
				return authzapp.ErrAccessManagementConflict
			}
			value.ID = existing.ID
			eventType = "authorization.group_membership.reactivated"
			if err := affectedExactlyOne(tx.WithContext(ctx).Exec(`UPDATE authorization_group_memberships SET active = true, updated_at = ? WHERE id = ? AND NOT active`, now, value.ID)); err != nil {
				return err
			}
		} else if err := tx.WithContext(ctx).Exec(`INSERT INTO authorization_group_memberships (id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, value.ID, groupID, userID).Error; err != nil {
			return err
		}

		event, err := platform.NewEvent(eventType, "authorization_group_membership", value.ID.String(), payload, now)
		if err != nil {
			return fmt.Errorf("create Group membership event: %w", err)
		}
		if err := database.AppendAudit(ctx, tx, database.AuditRecord{OccurredAt: now, ActorID: &mutation.ActorID, OrganizationID: organizationID, Action: eventType, ResourceType: "authorization_group_membership", ResourceID: value.ID.String(), RequestID: mutation.RequestID, Metadata: metadata}); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
	if err != nil {
		return authzapp.AccessMembership{}, fmt.Errorf("add Group membership: %w", classifyAccessMutationError(err))
	}
	return value, nil
}

// CreateBinding creates one active Role binding.
func (r *AccessManagementRepository) CreateBinding(ctx context.Context, mutation authzapp.AccessMutation, input authzapp.CreateBindingInput) (authzapp.AccessBinding, error) {
	value := authzapp.AccessBinding{ID: uuid.New(), GroupID: input.GroupID, RoleID: input.RoleID, OrganizationID: input.OrganizationID, ScopeKind: input.ScopeKind, ProjectID: input.ProjectID, EnvironmentID: input.EnvironmentID, ApplicationID: input.ApplicationID, Active: true}
	if input.ScopeKind == "platform" {
		err := r.mutate(ctx, mutation, nil, "authorization_platform_role_binding", value.ID, "authorization.platform_role_binding.created", map[string]any{"groupId": input.GroupID.String(), "roleId": input.RoleID.String(), "scopeKind": input.ScopeKind}, func(tx *gorm.DB) error {
			return tx.WithContext(ctx).Exec(`INSERT INTO authorization_platform_role_bindings (id, group_id, role_id) VALUES (?, ?, ?)`, value.ID, value.GroupID, value.RoleID).Error
		})
		if err != nil {
			return authzapp.AccessBinding{}, fmt.Errorf("create platform Role binding: %w", err)
		}
		return value, nil
	}
	err := r.mutate(ctx, mutation, &input.OrganizationID, "authorization_group_role_binding", value.ID, "authorization.role_binding.created", map[string]any{"groupId": input.GroupID.String(), "roleId": input.RoleID.String(), "scopeKind": input.ScopeKind}, func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Exec(`INSERT INTO authorization_group_role_bindings (id, group_id, role_id, organization_id, scope_kind, project_id, environment_id, application_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.GroupID, value.RoleID, value.OrganizationID, value.ScopeKind, value.ProjectID, value.EnvironmentID, value.ApplicationID).Error
	})
	if err != nil {
		return authzapp.AccessBinding{}, fmt.Errorf("create Role binding: %w", err)
	}
	return value, nil
}

// CreateDeny creates one active explicit deny policy.
func (r *AccessManagementRepository) CreateDeny(ctx context.Context, mutation authzapp.AccessMutation, input authzapp.CreateDenyInput) (authzapp.AccessDeny, error) {
	value := authzapp.AccessDeny{ID: uuid.New(), GroupID: input.GroupID, Permission: input.Permission, OrganizationID: input.OrganizationID, ScopeKind: input.ScopeKind, ProjectID: input.ProjectID, EnvironmentID: input.EnvironmentID, ApplicationID: input.ApplicationID, Active: true}
	err := r.mutate(ctx, mutation, &input.OrganizationID, "authorization_deny_policy", value.ID, "authorization.deny_policy.created", map[string]any{"groupId": input.GroupID.String(), "permission": input.Permission, "scopeKind": input.ScopeKind}, func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Exec(`INSERT INTO authorization_deny_policies (id, group_id, permission_key, organization_id, scope_kind, project_id, environment_id, application_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.GroupID, value.Permission, value.OrganizationID, value.ScopeKind, value.ProjectID, value.EnvironmentID, value.ApplicationID).Error
	})
	if err != nil {
		return authzapp.AccessDeny{}, fmt.Errorf("create deny policy: %w", err)
	}
	return value, nil
}

// DisableUser locally disables an account and revokes every active browser session atomically.
func (r *AccessManagementRepository) DisableUser(ctx context.Context, mutation authzapp.AccessMutation, userID uuid.UUID) error {
	return r.mutateProtected(ctx, mutation, nil, "user", userID, "identity.user.disabled", map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Exec(`UPDATE users SET disabled_at = now(), updated_at = now() WHERE id = ? AND disabled_at IS NULL`, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.WithContext(ctx).Exec(`UPDATE sessions SET revoked_at = now() WHERE user_id = ? AND revoked_at IS NULL`, userID).Error
	})
}

// RevokeMembership deactivates one manual membership without deleting history.
func (r *AccessManagementRepository) RevokeMembership(ctx context.Context, mutation authzapp.AccessMutation, membershipID uuid.UUID) error {
	organizationID, err := r.organizationForMembership(ctx, membershipID)
	if err != nil {
		return err
	}
	return r.mutateProtected(ctx, mutation, organizationID, "authorization_group_membership", membershipID, "authorization.group_membership.revoked", map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Exec(`UPDATE authorization_group_memberships SET active = false, updated_at = now() WHERE id = ? AND source = 'manual' AND active`, membershipID)
		return affectedExactlyOne(result)
	})
}

// DisableRole deactivates one custom Role without deleting its permission history.
func (r *AccessManagementRepository) DisableRole(ctx context.Context, mutation authzapp.AccessMutation, roleID uuid.UUID) error {
	organizationID, err := r.organizationForRole(ctx, roleID)
	if err != nil {
		return err
	}
	return r.mutateProtected(ctx, mutation, organizationID, "authorization_role", roleID, "authorization.role.disabled", map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Exec(`UPDATE authorization_roles SET active = false, updated_at = now() WHERE id = ? AND system_key IS NULL AND active`, roleID)
		return affectedExactlyOne(result)
	})
}

// RevokeBinding deactivates one Project-descendant Role binding.
func (r *AccessManagementRepository) RevokeBinding(ctx context.Context, mutation authzapp.AccessMutation, bindingID uuid.UUID) error {
	organizationID, platformBinding, err := r.bindingLocation(ctx, bindingID)
	if err != nil {
		return err
	}
	resourceType, eventType, table := "authorization_group_role_binding", "authorization.role_binding.revoked", "authorization_group_role_bindings"
	if platformBinding {
		resourceType, eventType, table = "authorization_platform_role_binding", "authorization.platform_role_binding.revoked", "authorization_platform_role_bindings"
	}
	return r.mutateProtected(ctx, mutation, organizationID, resourceType, bindingID, eventType, map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Table(table).Where("id = ? AND active", bindingID).Updates(map[string]any{"active": false, "updated_at": time.Now().UTC()})
		return affectedExactlyOne(result)
	})
}

// RevokeDeny deactivates one explicit deny policy.
func (r *AccessManagementRepository) RevokeDeny(ctx context.Context, mutation authzapp.AccessMutation, denyID uuid.UUID) error {
	organizationID, err := r.organizationForDeny(ctx, denyID)
	if err != nil {
		return err
	}
	return r.mutate(ctx, mutation, organizationID, "authorization_deny_policy", denyID, "authorization.deny_policy.revoked", map[string]any{}, func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Exec(`UPDATE authorization_deny_policies SET active = false, updated_at = now() WHERE id = ? AND active`, denyID)
		return affectedExactlyOne(result)
	})
}

func (r *AccessManagementRepository) mutate(ctx context.Context, mutation authzapp.AccessMutation, organizationID *uuid.UUID, resourceType string, resourceID uuid.UUID, eventType string, metadata map[string]any, work func(*gorm.DB) error) error {
	return r.mutateWithGuard(ctx, mutation, organizationID, resourceType, resourceID, eventType, metadata, false, work)
}

func (r *AccessManagementRepository) mutateProtected(ctx context.Context, mutation authzapp.AccessMutation, organizationID *uuid.UUID, resourceType string, resourceID uuid.UUID, eventType string, metadata map[string]any, work func(*gorm.DB) error) error {
	return r.mutateWithGuard(ctx, mutation, organizationID, resourceType, resourceID, eventType, metadata, true, work)
}

func (r *AccessManagementRepository) mutateWithGuard(ctx context.Context, mutation authzapp.AccessMutation, organizationID *uuid.UUID, resourceType string, resourceID uuid.UUID, eventType string, metadata map[string]any, protectContinuity bool, work func(*gorm.DB) error) error {
	now := time.Now().UTC()
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal access management event: %w", err)
	}
	event, err := platform.NewEvent(eventType, resourceType, resourceID.String(), payload, now)
	if err != nil {
		return fmt.Errorf("create access management event: %w", err)
	}
	err = database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if protectContinuity {
			if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(?)`, int64(5931311082169046361)).Error; err != nil {
				return fmt.Errorf("lock platform management continuity: %w", err)
			}
		}
		if err := work(tx); err != nil {
			return err
		}
		if protectContinuity {
			var paths int64
			err := tx.WithContext(ctx).Raw(`
SELECT count(DISTINCT users.id)
FROM users
JOIN authorization_group_memberships memberships ON memberships.user_id = users.id AND memberships.active
JOIN authorization_groups groups ON groups.id = memberships.group_id AND groups.disabled_at IS NULL
JOIN authorization_platform_role_bindings bindings ON bindings.group_id = groups.id AND bindings.active
JOIN authorization_roles roles ON roles.id = bindings.role_id AND roles.active
JOIN authorization_role_permissions permissions ON permissions.role_id = roles.id AND permissions.permission_key = 'platform.manage'
WHERE users.disabled_at IS NULL`).Scan(&paths).Error
			if err != nil {
				return fmt.Errorf("count effective platform managers: %w", err)
			}
			if paths == 0 {
				return authzapp.ErrLastPlatformManager
			}
		}
		if err := database.AppendAudit(ctx, tx, database.AuditRecord{OccurredAt: now, ActorID: &mutation.ActorID, OrganizationID: organizationID, Action: eventType, ResourceType: resourceType, ResourceID: resourceID.String(), RequestID: mutation.RequestID, Metadata: metadata}); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
	return classifyAccessMutationError(err)
}

func classifyAccessMutationError(err error) error {
	if err == nil || errors.Is(err, authzapp.ErrAccessManagementConflict) || errors.Is(err, authzapp.ErrLastPlatformManager) {
		return err
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "23503", "23505", "23514":
			return fmt.Errorf("%w: %v", authzapp.ErrAccessManagementConflict, err)
		}
	}
	return err
}

func affectedExactlyOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return authzapp.ErrAccessManagementConflict
	}
	return nil
}

func (r *AccessManagementRepository) organizationForOwner(ctx context.Context, ownerKind string, ownerID *uuid.UUID) (*uuid.UUID, error) {
	if ownerKind == "platform" {
		return nil, nil
	}
	if ownerID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if ownerKind == "organization" {
		var count int64
		if err := r.db.WithContext(ctx).Table("organizations").Where("id = ? AND active", *ownerID).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("verify Organization owner: %w", err)
		}
		if count != 1 {
			return nil, gorm.ErrRecordNotFound
		}
		value := *ownerID
		return &value, nil
	}
	var project struct {
		OrganizationID uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT organization_id FROM projects WHERE id = ? AND active`, *ownerID).Scan(&project).Error; err != nil {
		return nil, fmt.Errorf("resolve Project owner: %w", err)
	}
	if project.OrganizationID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &project.OrganizationID, nil
}

func (r *AccessManagementRepository) organizationForGroup(ctx context.Context, groupID uuid.UUID) (*uuid.UUID, error) {
	var owner struct {
		Kind string
		ID   *uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT owner_kind AS kind, owner_id AS id FROM authorization_groups WHERE id = ?`, groupID).Scan(&owner).Error; err != nil {
		return nil, fmt.Errorf("resolve Group owner: %w", err)
	}
	if owner.Kind == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return r.organizationForOwner(ctx, owner.Kind, owner.ID)
}

func (r *AccessManagementRepository) organizationForMembership(ctx context.Context, membershipID uuid.UUID) (*uuid.UUID, error) {
	var membership struct {
		GroupID uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT group_id FROM authorization_group_memberships WHERE id = ?`, membershipID).Scan(&membership).Error; err != nil {
		return nil, fmt.Errorf("resolve membership Group: %w", err)
	}
	if membership.GroupID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.organizationForGroup(ctx, membership.GroupID)
}

func (r *AccessManagementRepository) organizationForRole(ctx context.Context, roleID uuid.UUID) (*uuid.UUID, error) {
	var owner struct {
		Kind string
		ID   *uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT owner_kind AS kind, owner_id AS id FROM authorization_roles WHERE id = ?`, roleID).Scan(&owner).Error; err != nil || owner.Kind == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return r.organizationForOwner(ctx, owner.Kind, owner.ID)
}

func (r *AccessManagementRepository) bindingLocation(ctx context.Context, bindingID uuid.UUID) (*uuid.UUID, bool, error) {
	var binding struct {
		OrganizationID uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT organization_id FROM authorization_group_role_bindings WHERE id = ?`, bindingID).Scan(&binding).Error; err != nil {
		return nil, false, err
	}
	if binding.OrganizationID != uuid.Nil {
		return &binding.OrganizationID, false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Table("authorization_platform_role_bindings").Where("id = ?", bindingID).Count(&count).Error; err != nil || count != 1 {
		return nil, false, gorm.ErrRecordNotFound
	}
	return nil, true, nil
}

func (r *AccessManagementRepository) organizationForDeny(ctx context.Context, denyID uuid.UUID) (*uuid.UUID, error) {
	var deny struct {
		OrganizationID uuid.UUID
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT organization_id FROM authorization_deny_policies WHERE id = ?`, denyID).Scan(&deny).Error; err != nil {
		return nil, fmt.Errorf("resolve deny Organization: %w", err)
	}
	if deny.OrganizationID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &deny.OrganizationID, nil
}
