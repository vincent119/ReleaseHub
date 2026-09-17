package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AuditScopeResolved   = "resolved"
	AuditScopeUnresolved = "unresolved"

	maxAuditMetadataBytes = 16 * 1024
	maxAuditMetadataDepth = 8
	maxAuditMetadataKeys  = 100
	maxAuditArrayItems    = 100
	maxAuditStringBytes   = 2 * 1024
)

var forbiddenAuditMetadataKeys = []string{
	"password",
	"passwd",
	"token",
	"secret",
	"authorization",
	"cookie",
	"credential",
	"privatekey",
	"clientsecret",
	"accesskey",
	"session",
}

func prepareAuditRecord(ctx context.Context, tx *gorm.DB, record *AuditRecord) ([]byte, error) {
	if tx == nil {
		return nil, errors.New("audit transaction is required")
	}
	if record == nil {
		return nil, errors.New("audit record is required")
	}
	if record.OccurredAt.IsZero() {
		return nil, errors.New("audit occurrence time is required")
	}
	record.Action = strings.TrimSpace(record.Action)
	record.ResourceType = strings.TrimSpace(record.ResourceType)
	record.ResourceID = strings.TrimSpace(record.ResourceID)
	record.RequestID = strings.TrimSpace(record.RequestID)
	if err := validateAuditText(record.Action, "action", 255, true); err != nil {
		return nil, err
	}
	if err := validateAuditText(record.ResourceType, "resource type", 255, true); err != nil {
		return nil, err
	}
	if err := validateAuditText(record.ResourceID, "resource ID", 512, true); err != nil {
		return nil, err
	}
	if err := validateAuditText(record.RequestID, "request ID", 255, false); err != nil {
		return nil, err
	}
	if err := populateAuditActor(ctx, tx, record); err != nil {
		return nil, err
	}
	if err := populateAuditScope(ctx, tx, record); err != nil {
		return nil, err
	}
	if err := validateAuditScope(*record); err != nil {
		return nil, err
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	if err := validateAuditMetadata(record.Metadata, 1); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal audit metadata: %w", err)
	}
	if len(metadata) > maxAuditMetadataBytes {
		return nil, fmt.Errorf("audit metadata exceeds %d bytes", maxAuditMetadataBytes)
	}
	return metadata, nil
}

func validateAuditText(value, field string, maximum int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("audit %s is required", field)
	}
	if len(value) > maximum {
		return fmt.Errorf("audit %s exceeds %d bytes", field, maximum)
	}
	return nil
}

func populateAuditActor(ctx context.Context, tx *gorm.DB, record *AuditRecord) error {
	record.ActorDisplayName = strings.TrimSpace(record.ActorDisplayName)
	if record.ActorID == nil {
		if record.ActorDisplayName != "" {
			return errors.New("audit actor display name requires an actor ID")
		}
		return nil
	}
	if record.ActorDisplayName == "" {
		var actor struct {
			Username string
		}
		result := tx.WithContext(ctx).Raw(`SELECT username FROM users WHERE id = ?`, *record.ActorID).Scan(&actor)
		if result.Error != nil {
			return fmt.Errorf("load audit actor snapshot: %w", result.Error)
		}
		if result.RowsAffected != 1 || strings.TrimSpace(actor.Username) == "" {
			return errors.New("audit actor does not exist")
		}
		record.ActorDisplayName = strings.TrimSpace(actor.Username)
	}
	return validateAuditText(record.ActorDisplayName, "actor display name", 255, true)
}

func populateAuditScope(ctx context.Context, tx *gorm.DB, record *AuditRecord) error {
	record.ScopeResolution = strings.TrimSpace(record.ScopeResolution)
	if record.ScopeResolution != "" || hasAuditScope(*record) {
		if record.ScopeResolution == "" {
			record.ScopeResolution = AuditScopeResolved
		}
		return nil
	}
	resourceID, err := uuid.Parse(record.ResourceID)
	if err != nil {
		if isPlatformAuditResource(record.ResourceType) {
			record.ScopeResolution = AuditScopeResolved
		} else {
			record.ScopeResolution = AuditScopeUnresolved
		}
		return nil
	}
	scope, found, err := resolveAuditScope(ctx, tx, record.ResourceType, resourceID)
	if err != nil {
		return err
	}
	if found {
		record.OrganizationID = scope.OrganizationID
		record.ProjectID = scope.ProjectID
		record.EnvironmentID = scope.EnvironmentID
		record.ApplicationID = scope.ApplicationID
		record.ScopeResolution = AuditScopeResolved
		return nil
	}
	if isPlatformAuditResource(record.ResourceType) {
		record.ScopeResolution = AuditScopeResolved
	} else {
		record.ScopeResolution = AuditScopeUnresolved
	}
	return nil
}

type auditScopeRow struct {
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
	ApplicationID  *uuid.UUID
}

func resolveAuditScope(ctx context.Context, tx *gorm.DB, resourceType string, resourceID uuid.UUID) (auditScopeRow, bool, error) {
	query := ""
	switch resourceType {
	case "organization":
		query = `SELECT id AS organization_id FROM organizations WHERE id = ?`
	case "project":
		query = `SELECT organization_id, id AS project_id FROM projects WHERE id = ?`
	case "environment", "deployment_schedule":
		query = `SELECT organization_id, project_id, id AS environment_id FROM environments WHERE id = ?`
	case "application", "application_onboarding":
		query = `SELECT organization_id, project_id, environment_id, id AS application_id FROM applications WHERE id = ?`
	case "deployment_binding":
		query = `SELECT organization_id, project_id, environment_id FROM deployment_bindings WHERE id = ?`
	case "deployment_request":
		query = `SELECT organization_id, project_id, environment_id FROM deployment_requests WHERE id = ?`
	case "deployment_request_version":
		query = `SELECT request.organization_id, request.project_id, request.environment_id
FROM deployment_request_versions version
JOIN deployment_requests request ON request.id = version.request_id
WHERE version.id = ?`
	case "deployment_review_task":
		query = `SELECT request.organization_id, request.project_id, request.environment_id
FROM deployment_review_tasks review
JOIN deployment_request_versions version ON version.id = review.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE review.id = ?`
	case "deployment_execution":
		query = `SELECT request.organization_id, request.project_id, request.environment_id
FROM deployment_executions execution
JOIN deployment_request_versions version ON version.id = execution.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE execution.id = ?`
	case "deployment_plan":
		query = `SELECT project.organization_id, plan.owner_project_id AS project_id
FROM deployment_plans plan
LEFT JOIN projects project ON project.id = plan.owner_project_id
WHERE plan.id = ?`
	case "authorization_group_role_binding":
		query = `SELECT organization_id, project_id, environment_id, application_id
FROM authorization_group_role_bindings WHERE id = ?`
	case "authorization_deny_policy":
		query = `SELECT organization_id, project_id, environment_id, application_id
FROM authorization_deny_policies WHERE id = ?`
	case "authorization_role":
		query = `SELECT project.organization_id, role.owner_id AS project_id
FROM authorization_roles role
LEFT JOIN projects project ON role.owner_kind = 'project' AND project.id = role.owner_id
WHERE role.id = ?`
	case "authorization_group":
		query = `SELECT
    CASE WHEN auth_group.owner_kind = 'organization' THEN auth_group.owner_id ELSE project.organization_id END AS organization_id,
    CASE WHEN auth_group.owner_kind = 'project' THEN auth_group.owner_id END AS project_id
FROM authorization_groups auth_group
LEFT JOIN projects project ON auth_group.owner_kind = 'project' AND project.id = auth_group.owner_id
WHERE auth_group.id = ?`
	case "authorization_group_membership":
		query = `SELECT
    CASE WHEN auth_group.owner_kind = 'organization' THEN auth_group.owner_id ELSE project.organization_id END AS organization_id,
    CASE WHEN auth_group.owner_kind = 'project' THEN auth_group.owner_id END AS project_id
FROM authorization_group_memberships membership
JOIN authorization_groups auth_group ON auth_group.id = membership.group_id
LEFT JOIN projects project ON auth_group.owner_kind = 'project' AND project.id = auth_group.owner_id
WHERE membership.id = ?`
	case "authorization_platform_role_binding":
		return auditScopeRow{}, true, nil
	default:
		return auditScopeRow{}, false, nil
	}
	var scope auditScopeRow
	result := tx.WithContext(ctx).Raw(query, resourceID).Scan(&scope)
	if result.Error != nil {
		return auditScopeRow{}, false, fmt.Errorf("resolve audit scope: %w", result.Error)
	}
	return scope, result.RowsAffected == 1, nil
}

func hasAuditScope(record AuditRecord) bool {
	return record.OrganizationID != nil || record.ProjectID != nil || record.EnvironmentID != nil || record.ApplicationID != nil
}

func isPlatformAuditResource(resourceType string) bool {
	switch resourceType {
	case "release_workflow", "user", "argocd_application_candidate", "authorization_platform_role_binding":
		return true
	default:
		return false
	}
}

func validateAuditScope(record AuditRecord) error {
	if record.ScopeResolution != AuditScopeResolved && record.ScopeResolution != AuditScopeUnresolved {
		return errors.New("audit scope resolution is invalid")
	}
	if record.ApplicationID != nil && (record.EnvironmentID == nil || record.ProjectID == nil || record.OrganizationID == nil) {
		return errors.New("audit application scope requires its ancestry")
	}
	if record.EnvironmentID != nil && (record.ProjectID == nil || record.OrganizationID == nil) {
		return errors.New("audit environment scope requires its ancestry")
	}
	if record.ProjectID != nil && record.OrganizationID == nil {
		return errors.New("audit project scope requires its organization")
	}
	if record.ScopeResolution == AuditScopeUnresolved && hasAuditScope(record) {
		return errors.New("unresolved audit scope cannot contain ancestry")
	}
	return nil
}

func validateAuditMetadata(value any, depth int) error {
	if depth > maxAuditMetadataDepth {
		return fmt.Errorf("audit metadata exceeds depth %d", maxAuditMetadataDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) > maxAuditMetadataKeys {
			return fmt.Errorf("audit metadata object exceeds %d keys", maxAuditMetadataKeys)
		}
		for key, child := range typed {
			if isForbiddenAuditMetadataKey(key) {
				return fmt.Errorf("audit metadata key %q is forbidden", key)
			}
			if err := validateAuditMetadata(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if len(typed) > maxAuditArrayItems {
			return fmt.Errorf("audit metadata array exceeds %d items", maxAuditArrayItems)
		}
		for _, child := range typed {
			if err := validateAuditMetadata(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > maxAuditStringBytes {
			return fmt.Errorf("audit metadata string exceeds %d bytes", maxAuditStringBytes)
		}
	case nil, bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("audit metadata contains an unsupported value: %w", err)
		}
		var normalized any
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			return fmt.Errorf("normalize audit metadata value: %w", err)
		}
		return validateAuditMetadata(normalized, depth)
	}
	return nil
}

func isForbiddenAuditMetadataKey(key string) bool {
	normalized := strings.Map(func(value rune) rune {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			return unicode.ToLower(value)
		}
		return -1
	}, key)
	for _, forbidden := range forbiddenAuditMetadataKeys {
		if strings.Contains(normalized, forbidden) {
			return true
		}
	}
	return false
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
