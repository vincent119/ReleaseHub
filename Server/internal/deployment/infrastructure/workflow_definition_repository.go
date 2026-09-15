package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// WorkflowDefinitionRepository persists workflow definitions in PostgreSQL.
type WorkflowDefinitionRepository struct{ db *gorm.DB }

type workflowChange struct {
	mutation   deployapp.WorkflowMutation
	workflowID uuid.UUID
	action     string
	metadata   map[string]any
}

var _ deployapp.WorkflowDefinitionRepository = (*WorkflowDefinitionRepository)(nil)

// NewWorkflowDefinitionRepository creates the PostgreSQL workflow repository.
func NewWorkflowDefinitionRepository(db *gorm.DB) (*WorkflowDefinitionRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &WorkflowDefinitionRepository{db: db}, nil
}

// List returns all centrally managed workflow definitions and versions.
func (r *WorkflowDefinitionRepository) List(ctx context.Context) ([]deploydomain.ReleaseWorkflow, error) {
	var workflows []releaseWorkflowModel
	if err := r.db.WithContext(ctx).Order("lower(name), id").Find(&workflows).Error; err != nil {
		return nil, fmt.Errorf("list release workflows: %w", err)
	}
	versions, err := r.loadVersions(ctx, nil)
	if err != nil {
		return nil, err
	}
	return joinWorkflows(workflows, versions), nil
}

// ListReviewOptions returns minimal identities referenced by Workflow Review policies.
func (r *WorkflowDefinitionRepository) ListReviewOptions(ctx context.Context) (deployapp.WorkflowReviewOptions, error) {
	value := deployapp.WorkflowReviewOptions{
		Users: []deployapp.WorkflowReviewUserOption{},
		Roles: []deployapp.WorkflowReviewRoleOption{},
	}
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, username, disabled_at IS NULL AS assignable
FROM users
ORDER BY lower(username), id`).Scan(&value.Users).Error; err != nil {
		return deployapp.WorkflowReviewOptions{}, fmt.Errorf("list Workflow Review users: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, name, owner_kind, owner_id, active AS assignable
FROM authorization_roles
ORDER BY owner_kind, lower(name), id`).Scan(&value.Roles).Error; err != nil {
		return deployapp.WorkflowReviewOptions{}, fmt.Errorf("list Workflow Review Roles: %w", err)
	}
	return value, nil
}

// Load returns one workflow and all immutable versions.
func (r *WorkflowDefinitionRepository) Load(ctx context.Context, workflowID uuid.UUID) (deploydomain.ReleaseWorkflow, error) {
	var workflow releaseWorkflowModel
	if err := r.db.WithContext(ctx).First(&workflow, "id = ?", workflowID).Error; err != nil {
		return deploydomain.ReleaseWorkflow{}, workflowReadError(err)
	}
	versions, err := r.loadVersions(ctx, &workflowID)
	if err != nil {
		return deploydomain.ReleaseWorkflow{}, err
	}
	return workflowFromModel(workflow, versions), nil
}

// Create stores a workflow, first version, audit, and outbox atomically.
func (r *WorkflowDefinitionRepository) Create(ctx context.Context, mutation deployapp.WorkflowMutation, workflow deploydomain.ReleaseWorkflow) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		model := workflowToModel(workflow)
		if err := tx.Create(&model).Error; err != nil {
			return workflowCreateError(err)
		}
		version, err := workflowVersionToModel(workflow.Versions[0])
		if err != nil {
			return err
		}
		if err := tx.Create(&version).Error; err != nil {
			return workflowWriteError("create release workflow version", err)
		}
		return appendWorkflowChange(ctx, tx, workflowChange{
			mutation: mutation, workflowID: workflow.ID, action: "created",
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

// AppendVersion atomically verifies the latest version and single-draft rule.
func (r *WorkflowDefinitionRepository) AppendVersion(ctx context.Context, mutation deployapp.WorkflowMutation, expected uint64, version deploydomain.ReleaseWorkflowVersion) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := lockWorkflow(tx, version.WorkflowID); err != nil {
			return err
		}
		if err := verifyVersionAppend(tx, version.WorkflowID, expected); err != nil {
			return err
		}
		if err := createWorkflowVersion(tx, version); err != nil {
			return err
		}
		return appendWorkflowChange(ctx, tx, workflowChange{
			mutation: mutation, workflowID: version.WorkflowID, action: "version.created",
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

// UpdateLifecycle applies one optimistic lifecycle change with audit and outbox.
func (r *WorkflowDefinitionRepository) UpdateLifecycle(ctx context.Context, mutation deployapp.WorkflowMutation, expected uint64, version deploydomain.ReleaseWorkflowVersion) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := updateWorkflowLifecycle(tx, expected, version); err != nil {
			return err
		}
		event := "version." + string(version.Lifecycle)
		return appendWorkflowChange(ctx, tx, workflowChange{
			mutation: mutation, workflowID: version.WorkflowID, action: event,
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

func createWorkflowVersion(tx *gorm.DB, version deploydomain.ReleaseWorkflowVersion) error {
	model, err := workflowVersionToModel(version)
	if err != nil {
		return err
	}
	if err := tx.Create(&model).Error; err != nil {
		return workflowWriteError("append release workflow version", err)
	}
	return nil
}

func updateWorkflowLifecycle(tx *gorm.DB, expected uint64, version deploydomain.ReleaseWorkflowVersion) error {
	result := tx.Model(&releaseWorkflowVersionModel{}).
		Where("id = ? AND workflow_id = ? AND lock_version = ?", version.ID, version.WorkflowID, expected).
		Updates(workflowLifecycleChanges(version))
	if result.Error != nil {
		return workflowWriteError("update release workflow lifecycle", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowConflict
	}
	return nil
}

func workflowLifecycleChanges(version deploydomain.ReleaseWorkflowVersion) map[string]any {
	return map[string]any{
		"lifecycle": version.Lifecycle, "lock_version": version.LockVersion,
		"published_at": version.PublishedAt, "disabled_at": version.DisabledAt,
		"updated_at": version.UpdatedAt,
	}
}

func (r *WorkflowDefinitionRepository) loadVersions(ctx context.Context, workflowID *uuid.UUID) ([]deploydomain.ReleaseWorkflowVersion, error) {
	query := r.db.WithContext(ctx).Order("workflow_id, version_number")
	if workflowID != nil {
		query = query.Where("workflow_id = ?", *workflowID)
	}
	var models []releaseWorkflowVersionModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list release workflow versions: %w", err)
	}
	return workflowVersionsFromModels(models)
}

func workflowVersionsFromModels(models []releaseWorkflowVersionModel) ([]deploydomain.ReleaseWorkflowVersion, error) {
	result := make([]deploydomain.ReleaseWorkflowVersion, 0, len(models))
	for _, model := range models {
		version, err := workflowVersionFromModel(model)
		if err != nil {
			return nil, err
		}
		result = append(result, version)
	}
	return result, nil
}

func joinWorkflows(models []releaseWorkflowModel, versions []deploydomain.ReleaseWorkflowVersion) []deploydomain.ReleaseWorkflow {
	byWorkflow := make(map[uuid.UUID][]deploydomain.ReleaseWorkflowVersion)
	for _, version := range versions {
		byWorkflow[version.WorkflowID] = append(byWorkflow[version.WorkflowID], version)
	}
	result := make([]deploydomain.ReleaseWorkflow, 0, len(models))
	for _, model := range models {
		result = append(result, workflowFromModel(model, byWorkflow[model.ID]))
	}
	return result
}

func lockWorkflow(tx *gorm.DB, workflowID uuid.UUID) error {
	var model releaseWorkflowModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "id = ?", workflowID).Error; err != nil {
		return workflowReadError(err)
	}
	return nil
}

func verifyVersionAppend(tx *gorm.DB, workflowID uuid.UUID, expected uint64) error {
	var latest, drafts int64
	if err := tx.Model(&releaseWorkflowVersionModel{}).Where("workflow_id = ?", workflowID).Select("COALESCE(MAX(version_number), 0)").Scan(&latest).Error; err != nil {
		return fmt.Errorf("read latest workflow version: %w", err)
	}
	if err := tx.Model(&releaseWorkflowVersionModel{}).Where("workflow_id = ? AND lifecycle = 'Draft'", workflowID).Count(&drafts).Error; err != nil {
		return fmt.Errorf("count workflow drafts: %w", err)
	}
	if uint64(latest) != expected || drafts > 0 {
		return deployapp.ErrWorkflowConflict
	}
	return nil
}

func appendWorkflowChange(ctx context.Context, tx *gorm.DB, change workflowChange) error {
	payload, err := json.Marshal(change.metadata)
	if err != nil {
		return fmt.Errorf("marshal workflow event: %w", err)
	}
	event, err := platform.NewEvent("release_workflow."+change.action, "release_workflow", change.workflowID.String(), payload, change.mutation.OccurredAt)
	if err != nil {
		return fmt.Errorf("create workflow event: %w", err)
	}
	if err := appendWorkflowAudit(ctx, tx, change); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func appendWorkflowAudit(ctx context.Context, tx *gorm.DB, change workflowChange) error {
	actorID := change.mutation.ActorID
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: change.mutation.OccurredAt, ActorID: &actorID,
		Action: "release_workflow." + change.action, ResourceType: "release_workflow",
		ResourceID: change.workflowID.String(), RequestID: change.mutation.RequestID, Metadata: change.metadata,
	})
}

func workflowReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.ErrWorkflowNotFound
	}
	return fmt.Errorf("read release workflow: %w", err)
}

func workflowWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23503" || postgresError.Code == "23505") {
		return deployapp.ErrWorkflowConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func workflowCreateError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "release_workflows_name_idx" {
		return deployapp.ErrWorkflowNameConflict
	}
	return workflowWriteError("create release workflow", err)
}
