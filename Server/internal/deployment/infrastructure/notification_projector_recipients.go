package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const notificationRecipientQuery = `
SELECT DISTINCT users.id
FROM users
JOIN authorization_group_memberships membership
  ON membership.user_id = users.id AND membership.active
JOIN authorization_groups groups
  ON groups.id = membership.group_id AND groups.disabled_at IS NULL
JOIN authorization_group_role_bindings binding
  ON binding.group_id = groups.id AND binding.active
JOIN authorization_roles role
  ON role.id = binding.role_id AND role.active
JOIN authorization_role_permissions permission
  ON permission.role_id = role.id AND permission.permission_key = 'notification.view'
WHERE users.disabled_at IS NULL
  AND binding.organization_id = ? AND binding.project_id = ?
  AND (binding.scope_kind = 'project'
    OR (binding.scope_kind = 'environment' AND binding.environment_id = ?)
    OR (binding.scope_kind = 'application' AND binding.environment_id = ? AND binding.application_id = ?))
  AND NOT EXISTS (
    SELECT 1 FROM authorization_group_memberships denied_membership
    JOIN authorization_groups denied_group
      ON denied_group.id = denied_membership.group_id AND denied_group.disabled_at IS NULL
    JOIN authorization_deny_policies denial
      ON denial.group_id = denied_group.id AND denial.active
    WHERE denied_membership.user_id = users.id AND denied_membership.active
      AND denial.permission_key = 'notification.view'
      AND denial.organization_id = ? AND denial.project_id = ?
      AND (denial.scope_kind = 'project'
        OR (denial.scope_kind = 'environment' AND denial.environment_id = ?)
        OR (denial.scope_kind = 'application' AND denial.environment_id = ? AND denial.application_id = ?))
  )
ORDER BY users.id`

func notificationRecipients(ctx context.Context, tx *gorm.DB, scope notificationScopeRow, broadcast bool) ([]uuid.UUID, error) {
	if broadcast {
		return activeNotificationUsers(ctx, tx)
	}
	applicationID := nullableApplicationID(scope.ApplicationID)
	arguments := []any{
		scope.OrganizationID, scope.ProjectID, scope.EnvironmentID, scope.EnvironmentID, applicationID,
		scope.OrganizationID, scope.ProjectID, scope.EnvironmentID, scope.EnvironmentID, applicationID,
	}
	var recipients []uuid.UUID
	if err := tx.WithContext(ctx).Raw(notificationRecipientQuery, arguments...).Scan(&recipients).Error; err != nil {
		return nil, fmt.Errorf("resolve notification recipients: %w", err)
	}
	return recipients, nil
}

func activeNotificationUsers(ctx context.Context, tx *gorm.DB) ([]uuid.UUID, error) {
	var recipients []uuid.UUID
	err := tx.WithContext(ctx).Table("users").Where("disabled_at IS NULL").Order("id").Pluck("id", &recipients).Error
	if err != nil {
		return nil, fmt.Errorf("resolve notification broadcast recipients: %w", err)
	}
	return recipients, nil
}

func nullableApplicationID(value *uuid.UUID) any {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return *value
}
