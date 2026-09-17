// Package infrastructure implements Audit Trail read adapters.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

// PostgresRepository reads immutable Audit Trail records.
type PostgresRepository struct{ db *gorm.DB }

type auditEventRow struct {
	ID               uuid.UUID
	OccurredAt       time.Time
	ActorID          *uuid.UUID
	ActorDisplayName *string
	OrganizationID   *uuid.UUID
	ProjectID        *uuid.UUID
	EnvironmentID    *uuid.UUID
	ApplicationID    *uuid.UUID
	ScopeResolution  string
	Action           string
	ResourceType     string
	ResourceID       string
	RequestID        string
	HasMetadata      bool
	Metadata         []byte
}

var _ auditapp.Repository = (*PostgresRepository)(nil)

// NewPostgresRepository creates the PostgreSQL Audit Trail reader.
func NewPostgresRepository(db *gorm.DB) (*PostgresRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &PostgresRepository{db: db}, nil
}

// ResolveScope validates a public scope identifier and returns its complete ancestry.
func (r *PostgresRepository) ResolveScope(ctx context.Context, kind authz.ScopeKind, id *uuid.UUID) (authz.Scope, error) {
	if kind == authz.ScopePlatform {
		if id != nil {
			return authz.Scope{}, auditapp.ErrNotFound
		}
		return authz.NewPlatformScope(), nil
	}
	if id == nil || *id == uuid.Nil {
		return authz.Scope{}, auditapp.ErrNotFound
	}
	var row struct {
		OrganizationID uuid.UUID
		ProjectID      uuid.UUID
		EnvironmentID  uuid.UUID
		ApplicationID  uuid.UUID
	}
	var statement string
	switch kind {
	case authz.ScopeProject:
		statement = `SELECT organization_id, id AS project_id FROM projects WHERE id = ? AND active`
	case authz.ScopeEnvironment:
		statement = `SELECT organization_id, project_id, id AS environment_id FROM environments WHERE id = ? AND active`
	case authz.ScopeApplication:
		statement = `SELECT organization_id, project_id, environment_id, id AS application_id FROM applications WHERE id = ? AND active`
	default:
		return authz.Scope{}, auditapp.ErrNotFound
	}
	result := r.db.WithContext(ctx).Raw(statement, *id).Scan(&row)
	if result.Error != nil {
		return authz.Scope{}, fmt.Errorf("resolve audit scope: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return authz.Scope{}, auditapp.ErrNotFound
	}
	switch kind {
	case authz.ScopeProject:
		return authz.NewProjectScope(row.OrganizationID, row.ProjectID)
	case authz.ScopeEnvironment:
		return authz.NewEnvironmentScope(row.OrganizationID, row.ProjectID, row.EnvironmentID)
	default:
		return authz.NewApplicationScope(row.OrganizationID, row.ProjectID, row.EnvironmentID, row.ApplicationID)
	}
}

// List returns one keyset page and never selects the metadata payload.
func (r *PostgresRepository) List(ctx context.Context, query auditapp.ListQuery) (auditapp.StoredPage, error) {
	where, args, err := auditPredicates(query.Filter, query.Position, query.Visibility)
	if err != nil {
		return auditapp.StoredPage{}, err
	}
	statement := `SELECT id, occurred_at, actor_id, actor_display_name, organization_id, project_id,
environment_id, application_id, scope_resolution, action, resource_type, resource_id, request_id,
(metadata <> '{}'::jsonb) AS has_metadata
FROM audit_logs WHERE ` + strings.Join(where, " AND ") + `
ORDER BY occurred_at DESC, id DESC LIMIT ?`
	args = append(args, query.Filter.Limit+1)
	var rows []auditEventRow
	if err := r.db.WithContext(ctx).Raw(statement, args...).Scan(&rows).Error; err != nil {
		return auditapp.StoredPage{}, fmt.Errorf("query audit events: %w", err)
	}
	hasMore := len(rows) > query.Filter.Limit
	if hasMore {
		rows = rows[:query.Filter.Limit]
	}
	items := make([]auditdomain.EventSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.summary())
	}
	return auditapp.StoredPage{Items: items, HasMore: hasMore}, nil
}

// FilterOptions returns bounded, visibility-safe autocomplete values for one allowlisted field.
func (r *PostgresRepository) FilterOptions(ctx context.Context, filter auditdomain.FilterOptionFilter, visibility auditapp.VisibilityConstraint) ([]auditapp.FilterOption, error) {
	where, args, err := visibilityPredicates(visibility)
	if err != nil {
		return nil, err
	}
	where = append(where, "occurred_at >= ?", "occurred_at < ?")
	args = append(args, filter.OccurredFrom, filter.OccurredTo)
	search := containsPattern(filter.Search)
	var statement string
	switch filter.Field {
	case auditdomain.FilterOptionAction:
		if filter.Search != "" {
			where, args = append(where, `action ILIKE ? ESCAPE '\'`), append(args, search)
		}
		statement = `SELECT action AS value, action AS label FROM audit_logs WHERE ` + strings.Join(where, " AND ") + ` GROUP BY action ORDER BY lower(action), action LIMIT ?`
	case auditdomain.FilterOptionResourceType:
		if filter.Search != "" {
			where, args = append(where, `resource_type ILIKE ? ESCAPE '\'`), append(args, search)
		}
		statement = `SELECT resource_type AS value, resource_type AS label FROM audit_logs WHERE ` + strings.Join(where, " AND ") + ` GROUP BY resource_type ORDER BY lower(resource_type), resource_type LIMIT ?`
	case auditdomain.FilterOptionActor:
		if filter.Search != "" {
			where = append(where, `(COALESCE(actor_display_name, '') ILIKE ? ESCAPE '\' OR COALESCE(actor_id::text, 'system') ILIKE ? ESCAPE '\' OR (actor_id IS NULL AND 'ReleaseHub System' ILIKE ? ESCAPE '\'))`)
			args = append(args, search, search, search)
		}
		statement = `SELECT value, label FROM (
SELECT COALESCE(actor_id::text, 'system') AS value,
CASE
  WHEN actor_id IS NULL THEN 'ReleaseHub System'
  WHEN NULLIF(actor_display_name, '') IS NULL THEN actor_id::text
  ELSE actor_display_name || ' · ' || actor_id::text
END AS label
FROM audit_logs WHERE ` + strings.Join(where, " AND ") + `
GROUP BY actor_id, actor_display_name
) options ORDER BY lower(label), value LIMIT ?`
	default:
		return nil, auditdomain.ErrInvalidFilter
	}
	args = append(args, filter.Limit)
	var options []auditapp.FilterOption
	if err := r.db.WithContext(ctx).Raw(statement, args...).Scan(&options).Error; err != nil {
		return nil, fmt.Errorf("query audit filter options: %w", err)
	}
	return options, nil
}

// Locate reads only the authorization fields required for masked detail access.
func (r *PostgresRepository) Locate(ctx context.Context, eventID uuid.UUID) (auditdomain.ScopeAncestry, error) {
	var row auditEventRow
	result := r.db.WithContext(ctx).Raw(`SELECT organization_id, project_id, environment_id, application_id, scope_resolution
FROM audit_logs WHERE id = ?`, eventID).Scan(&row)
	if result.Error != nil {
		return auditdomain.ScopeAncestry{}, fmt.Errorf("locate audit event: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return auditdomain.ScopeAncestry{}, auditapp.ErrNotFound
	}
	return row.ancestry(), nil
}

// Get reads one event only when the SQL visibility predicate permits it.
func (r *PostgresRepository) Get(ctx context.Context, eventID uuid.UUID, visibility auditapp.VisibilityConstraint) (auditdomain.StoredDetail, error) {
	where := []string{"id = ?"}
	args := []any{eventID}
	visibilityWhere, visibilityArgs, err := visibilityPredicates(visibility)
	if err != nil {
		return auditdomain.StoredDetail{}, err
	}
	where = append(where, visibilityWhere...)
	args = append(args, visibilityArgs...)
	var row auditEventRow
	result := r.db.WithContext(ctx).Raw(`SELECT id, occurred_at, actor_id, actor_display_name, organization_id, project_id,
environment_id, application_id, scope_resolution, action, resource_type, resource_id, request_id,
(metadata <> '{}'::jsonb) AS has_metadata, metadata
FROM audit_logs WHERE `+strings.Join(where, " AND "), args...).Scan(&row)
	if result.Error != nil {
		return auditdomain.StoredDetail{}, fmt.Errorf("get audit event: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return auditdomain.StoredDetail{}, auditapp.ErrNotFound
	}
	return auditdomain.StoredDetail{EventSummary: row.summary(), Metadata: row.Metadata}, nil
}

func auditPredicates(filter auditdomain.QueryFilter, position *auditdomain.CursorPosition, visibility auditapp.VisibilityConstraint) ([]string, []any, error) {
	where, args, err := visibilityPredicates(visibility)
	if err != nil {
		return nil, nil, err
	}
	where = append(where, "occurred_at >= ?", "occurred_at < ?")
	args = append(args, filter.OccurredFrom, filter.OccurredTo)
	if filter.Action != "" {
		where, args = append(where, "action = ?"), append(args, filter.Action)
	}
	if filter.ResourceType != "" {
		where, args = append(where, "resource_type = ?"), append(args, filter.ResourceType)
	}
	if filter.RequestID != "" {
		where, args = append(where, "request_id = ?"), append(args, filter.RequestID)
	}
	if filter.Actor != "" {
		if strings.EqualFold(filter.Actor, "system") {
			where = append(where, "actor_id IS NULL")
		} else if actorID, parseErr := uuid.Parse(filter.Actor); parseErr == nil {
			where, args = append(where, "actor_id = ?"), append(args, actorID)
		} else {
			where, args = append(where, "lower(actor_display_name) = ?"), append(args, strings.ToLower(filter.Actor))
		}
	}
	if position != nil {
		where = append(where, "(occurred_at, id) < (?, ?)")
		args = append(args, position.OccurredAt, position.EventID)
	}
	return where, args, nil
}

func containsPattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return "%" + value + "%"
}

func visibilityPredicates(visibility auditapp.VisibilityConstraint) ([]string, []any, error) {
	where := []string{"TRUE"}
	args := make([]any, 0, 8)
	switch visibility.Root.Kind {
	case authz.ScopePlatform:
	case authz.ScopeProject:
		where = append(where, "scope_resolution = 'resolved'", "organization_id = ?", "project_id = ?")
		args = append(args, visibility.Root.OrganizationID, visibility.Root.ProjectID)
	case authz.ScopeEnvironment:
		where = append(where, "scope_resolution = 'resolved'", "organization_id = ?", "project_id = ?", "environment_id = ?")
		args = append(args, visibility.Root.OrganizationID, visibility.Root.ProjectID, visibility.Root.EnvironmentID)
	case authz.ScopeApplication:
		where = append(where, "scope_resolution = 'resolved'", "organization_id = ?", "project_id = ?", "environment_id = ?", "application_id = ?")
		args = append(args, visibility.Root.OrganizationID, visibility.Root.ProjectID, visibility.Root.EnvironmentID, visibility.Root.ApplicationID)
	default:
		return nil, nil, auditdomain.ErrInvalidFilter
	}
	if visibility.Root.Kind == authz.ScopePlatform {
		return where, args, nil
	}
	for _, denied := range visibility.Denied {
		clause, values, err := deniedPredicate(denied)
		if err != nil {
			return nil, nil, err
		}
		where = append(where, "NOT ("+clause+")")
		args = append(args, values...)
	}
	return where, args, nil
}

func deniedPredicate(scope authz.Scope) (string, []any, error) {
	switch scope.Kind {
	case authz.ScopeProject:
		return "organization_id = ? AND project_id = ?", []any{scope.OrganizationID, scope.ProjectID}, nil
	case authz.ScopeEnvironment:
		return "organization_id = ? AND project_id = ? AND environment_id IS NOT DISTINCT FROM ?", []any{scope.OrganizationID, scope.ProjectID, scope.EnvironmentID}, nil
	case authz.ScopeApplication:
		return "organization_id = ? AND project_id = ? AND environment_id IS NOT DISTINCT FROM ? AND application_id IS NOT DISTINCT FROM ?", []any{scope.OrganizationID, scope.ProjectID, scope.EnvironmentID, scope.ApplicationID}, nil
	default:
		return "", nil, auditdomain.ErrInvalidFilter
	}
}

func (r auditEventRow) ancestry() auditdomain.ScopeAncestry {
	return auditdomain.ScopeAncestry{OrganizationID: r.OrganizationID, ProjectID: r.ProjectID, EnvironmentID: r.EnvironmentID, ApplicationID: r.ApplicationID, Resolution: auditdomain.ScopeResolution(r.ScopeResolution)}
}

func (r auditEventRow) summary() auditdomain.EventSummary {
	displayName := ""
	if r.ActorDisplayName != nil {
		displayName = *r.ActorDisplayName
	}
	return auditdomain.EventSummary{ID: r.ID, OccurredAt: r.OccurredAt, ActorID: r.ActorID, ActorDisplayName: displayName,
		Action: r.Action, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Scope: r.ancestry(), RequestID: r.RequestID, HasMetadata: r.HasMetadata}
}
