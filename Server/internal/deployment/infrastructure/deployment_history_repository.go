package infrastructure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// DeploymentHistoryRepository reads successful immutable deployment evidence.
type DeploymentHistoryRepository struct{ db *gorm.DB }

type deploymentHistoryRow struct {
	RequestID        uuid.UUID
	RequestVersionID uuid.UUID
	ExecutionID      uuid.UUID
	Classification   string
	CompletedAt      time.Time
}

type deploymentHistoryCursor struct {
	CompletedAt time.Time `json:"completedAt"`
	ExecutionID uuid.UUID `json:"executionId"`
}

const deploymentHistoryQuery = `
SELECT request.id AS request_id, version.id AS request_version_id,
       execution.id AS execution_id, request.classification, execution.completed_at
FROM deployment_executions execution
JOIN deployment_request_versions version ON version.id = execution.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE request.organization_id = ? AND request.project_id = ? AND request.environment_id = ?
  AND execution.status = 'Succeeded' AND execution.completed_at IS NOT NULL
  AND (?::timestamptz IS NULL OR (execution.completed_at, execution.id) < (?, ?))
ORDER BY execution.completed_at DESC, execution.id DESC
LIMIT ?`

var _ deployapp.DeploymentHistoryRepository = (*DeploymentHistoryRepository)(nil)

// NewDeploymentHistoryRepository creates the PostgreSQL history reader.
func NewDeploymentHistoryRepository(db *gorm.DB) (*DeploymentHistoryRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentHistoryRepository{db: db}, nil
}

// ResolveScope validates the requested Project and Environment relationship.
func (r *DeploymentHistoryRepository) ResolveScope(ctx context.Context, projectID, environmentID uuid.UUID) (authz.Scope, error) {
	var value struct{ OrganizationID uuid.UUID }
	err := r.db.WithContext(ctx).Table("environments").Select("organization_id").
		Where("id = ? AND project_id = ? AND active", environmentID, projectID).Scan(&value).Error
	if err != nil || value.OrganizationID == uuid.Nil {
		return authz.Scope{}, deployapp.ErrHistoryNotFound
	}
	scope, err := authz.NewEnvironmentScope(value.OrganizationID, projectID, environmentID)
	if err != nil {
		return authz.Scope{}, deployapp.ErrHistoryNotFound
	}
	return scope, nil
}

// List returns a keyset page containing successful execution snapshots only.
func (r *DeploymentHistoryRepository) List(ctx context.Context, scope authz.Scope, cursor string, limit int) (deployapp.DeploymentHistoryPage, error) {
	position, err := decodeDeploymentHistoryCursor(cursor)
	if err != nil {
		return deployapp.DeploymentHistoryPage{}, deployapp.ErrHistoryInvalid
	}
	rows, err := r.listHistoryRows(ctx, scope, position, limit+1)
	if err != nil {
		return deployapp.DeploymentHistoryPage{}, err
	}
	return r.historyPage(ctx, rows, limit)
}

func (r *DeploymentHistoryRepository) listHistoryRows(ctx context.Context, scope authz.Scope, cursor *deploymentHistoryCursor, limit int) ([]deploymentHistoryRow, error) {
	var completedAt *time.Time
	var executionID uuid.UUID
	if cursor != nil {
		completedAt, executionID = &cursor.CompletedAt, cursor.ExecutionID
	}
	var rows []deploymentHistoryRow
	err := r.db.WithContext(ctx).Raw(deploymentHistoryQuery, scope.OrganizationID, scope.ProjectID,
		scope.EnvironmentID, completedAt, completedAt, executionID, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list successful deployment history: %w", err)
	}
	return rows, nil
}

func (r *DeploymentHistoryRepository) historyPage(ctx context.Context, rows []deploymentHistoryRow, limit int) (deployapp.DeploymentHistoryPage, error) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items, err := loadDeploymentHistoryItems(ctx, r.db, rows)
	if err != nil {
		return deployapp.DeploymentHistoryPage{}, err
	}
	next := ""
	if hasMore {
		next = encodeDeploymentHistoryCursor(rows[len(rows)-1])
	}
	return deployapp.DeploymentHistoryPage{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

func encodeDeploymentHistoryCursor(row deploymentHistoryRow) string {
	value, _ := json.Marshal(deploymentHistoryCursor{CompletedAt: row.CompletedAt.UTC(), ExecutionID: row.ExecutionID})
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeDeploymentHistoryCursor(value string) (*deploymentHistoryCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor deploymentHistoryCursor
	err = json.Unmarshal(decoded, &cursor)
	if err != nil || cursor.CompletedAt.IsZero() || cursor.ExecutionID == uuid.Nil {
		return nil, errors.New("invalid deployment history cursor")
	}
	return &cursor, nil
}

func loadDeploymentHistoryItems(ctx context.Context, db *gorm.DB, rows []deploymentHistoryRow) ([]deploydomain.DeploymentHistoryItem, error) {
	items := make([]deploydomain.DeploymentHistoryItem, 0, len(rows))
	for _, row := range rows {
		applications, err := loadSuccessfulHistoryApplications(ctx, db, row)
		if err != nil {
			return nil, err
		}
		items = append(items, deploydomain.DeploymentHistoryItem{
			RequestID: row.RequestID, RequestVersionID: row.RequestVersionID,
			Classification: row.Classification, CompletedAt: row.CompletedAt, Applications: applications,
		})
	}
	return items, nil
}
