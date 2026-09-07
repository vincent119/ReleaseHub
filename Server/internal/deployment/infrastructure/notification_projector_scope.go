package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func queryNotificationScope(ctx context.Context, tx *gorm.DB, aggregateType string, resourceID uuid.UUID) (notificationScopeRow, error) {
	query, arguments := notificationScopeQuery(aggregateType, resourceID)
	if query == "" {
		return notificationScopeRow{}, fmt.Errorf("unsupported notification aggregate %q", aggregateType)
	}
	var scope notificationScopeRow
	err := tx.WithContext(ctx).Raw(query, arguments...).Scan(&scope).Error
	if err != nil || scope.OrganizationID == uuid.Nil || scope.EnvironmentID == uuid.Nil {
		return notificationScopeRow{}, fmt.Errorf("resolve notification scope: %w", scopeLookupError(err))
	}
	return scope, nil
}

func notificationScopeQuery(aggregateType string, resourceID uuid.UUID) (string, []any) {
	switch aggregateType {
	case "deployment_request":
		return `SELECT organization_id, project_id, environment_id
			FROM deployment_requests WHERE id = ?`, []any{resourceID}
	case "deployment_request_version":
		return requestVersionScopeQuery("version.id = ?"), []any{resourceID}
	case "deployment_execution":
		return requestVersionScopeQuery("execution.id = ?", executionScopeJoin()), []any{resourceID}
	case "deployment_review_task":
		return requestVersionScopeQuery("task.id = ?", reviewTaskScopeJoin()), []any{resourceID}
	case "application_onboarding":
		return `SELECT organization_id, project_id, environment_id, id AS application_id
			FROM applications WHERE id = ?`, []any{resourceID}
	default:
		return "", nil
	}
}

func requestVersionScopeQuery(predicate string, prefix ...string) string {
	start := "FROM deployment_request_versions version"
	if len(prefix) > 0 {
		start = prefix[0]
	}
	return `SELECT request.organization_id, request.project_id, request.environment_id ` + start + `
		JOIN deployment_requests request ON request.id = version.request_id WHERE ` + predicate
}

func executionScopeJoin() string {
	return `FROM deployment_executions execution
		JOIN deployment_request_versions version ON version.id = execution.request_version_id`
}

func reviewTaskScopeJoin() string {
	return `FROM deployment_review_tasks task
		JOIN deployment_request_versions version ON version.id = task.request_version_id`
}

func scopeLookupError(err error) error {
	if err != nil {
		return err
	}
	return gorm.ErrRecordNotFound
}
