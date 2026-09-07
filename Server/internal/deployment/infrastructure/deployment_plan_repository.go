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

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// DeploymentPlanRepository persists Plan definitions in PostgreSQL.
type DeploymentPlanRepository struct{ db *gorm.DB }

type deploymentPlanChange struct {
	mutation deployapp.PlanMutation
	planID   uuid.UUID
	action   string
	metadata map[string]any
}

var _ deployapp.DeploymentPlanRepository = (*DeploymentPlanRepository)(nil)
var _ deployapp.PlanScopeResolver = (*DeploymentPlanRepository)(nil)

// NewDeploymentPlanRepository creates the PostgreSQL Plan repository.
func NewDeploymentPlanRepository(db *gorm.DB) (*DeploymentPlanRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentPlanRepository{db: db}, nil
}

// ResolveProject returns a canonical Project scope.
func (r *DeploymentPlanRepository) ResolveProject(ctx context.Context, projectID uuid.UUID) (authz.Scope, error) {
	var project struct{ OrganizationID uuid.UUID }
	err := r.db.WithContext(ctx).Table("projects").Select("organization_id").First(&project, "id = ?", projectID).Error
	if err != nil {
		return authz.Scope{}, deploymentPlanReadError(err)
	}
	return authz.NewProjectScope(project.OrganizationID, projectID)
}

// List returns platform Plans and Plans owned by one Project.
func (r *DeploymentPlanRepository) List(ctx context.Context, projectID uuid.UUID) ([]deploydomain.DeploymentPlan, error) {
	var models []deploymentPlanModel
	query := r.db.WithContext(ctx).Where("owner_kind = 'platform' OR owner_project_id = ?", projectID)
	if err := query.Order("lower(name), id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list deployment plans: %w", err)
	}
	versions, err := r.loadVersions(ctx, planIDs(models))
	if err != nil {
		return nil, err
	}
	return joinDeploymentPlans(models, versions), nil
}

// Load returns one Plan and all immutable Versions.
func (r *DeploymentPlanRepository) Load(ctx context.Context, planID uuid.UUID) (deploydomain.DeploymentPlan, error) {
	var model deploymentPlanModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", planID).Error; err != nil {
		return deploydomain.DeploymentPlan{}, deploymentPlanReadError(err)
	}
	versions, err := r.loadVersions(ctx, []uuid.UUID{planID})
	if err != nil {
		return deploydomain.DeploymentPlan{}, err
	}
	return deploymentPlanFromModel(model, versions), nil
}

// Create stores a Plan, first Version, Audit, and Outbox atomically.
func (r *DeploymentPlanRepository) Create(ctx context.Context, mutation deployapp.PlanMutation, plan deploydomain.DeploymentPlan) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		model := deploymentPlanToModel(plan)
		if err := tx.Create(&model).Error; err != nil {
			return deploymentPlanWriteError("create deployment plan", err)
		}
		version, err := deploymentPlanVersionToModel(plan.Versions[0])
		if err != nil {
			return err
		}
		if err := tx.Create(&version).Error; err != nil {
			return deploymentPlanWriteError("create deployment plan version", err)
		}
		return appendDeploymentPlanChange(ctx, tx, deploymentPlanChange{
			mutation: mutation, planID: plan.ID, action: "created",
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

// AppendVersion verifies the latest version and single-Draft rule atomically.
func (r *DeploymentPlanRepository) AppendVersion(ctx context.Context, mutation deployapp.PlanMutation, expected uint64, version deploydomain.DeploymentPlanVersion) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := lockDeploymentPlan(tx, version.PlanID); err != nil {
			return err
		}
		if err := verifyPlanVersionAppend(tx, version.PlanID, expected); err != nil {
			return err
		}
		if err := createDeploymentPlanVersion(tx, version); err != nil {
			return err
		}
		return appendDeploymentPlanChange(ctx, tx, deploymentPlanChange{
			mutation: mutation, planID: version.PlanID, action: "version.created",
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

// UpdateLifecycle applies one optimistic lifecycle transition with Audit and Outbox.
func (r *DeploymentPlanRepository) UpdateLifecycle(ctx context.Context, mutation deployapp.PlanMutation, expected uint64, version deploydomain.DeploymentPlanVersion) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := updateDeploymentPlanLifecycle(tx, expected, version); err != nil {
			return err
		}
		return appendDeploymentPlanChange(ctx, tx, deploymentPlanChange{
			mutation: mutation, planID: version.PlanID,
			action:   "version." + string(version.Lifecycle),
			metadata: map[string]any{"versionId": version.ID},
		})
	})
}

func createDeploymentPlanVersion(tx *gorm.DB, version deploydomain.DeploymentPlanVersion) error {
	model, err := deploymentPlanVersionToModel(version)
	if err != nil {
		return err
	}
	if err := tx.Create(&model).Error; err != nil {
		return deploymentPlanWriteError("append deployment plan version", err)
	}
	return nil
}

func updateDeploymentPlanLifecycle(tx *gorm.DB, expected uint64, version deploydomain.DeploymentPlanVersion) error {
	result := tx.Model(&deploymentPlanVersionModel{}).
		Where("id = ? AND plan_id = ? AND lock_version = ?", version.ID, version.PlanID, expected).
		Updates(deploymentPlanLifecycleChanges(version))
	if result.Error != nil {
		return deploymentPlanWriteError("update deployment plan lifecycle", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrPlanConflict
	}
	return nil
}

func deploymentPlanLifecycleChanges(version deploydomain.DeploymentPlanVersion) map[string]any {
	return map[string]any{
		"lifecycle": version.Lifecycle, "lock_version": version.LockVersion,
		"published_at": version.PublishedAt, "disabled_at": version.DisabledAt,
		"updated_at": version.UpdatedAt,
	}
}

func (r *DeploymentPlanRepository) loadVersions(ctx context.Context, planIDs []uuid.UUID) ([]deploydomain.DeploymentPlanVersion, error) {
	if len(planIDs) == 0 {
		return []deploydomain.DeploymentPlanVersion{}, nil
	}
	var models []deploymentPlanVersionModel
	if err := r.db.WithContext(ctx).Where("plan_id IN ?", planIDs).Order("plan_id, version_number").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list deployment plan versions: %w", err)
	}
	return deploymentPlanVersionsFromModels(models)
}

func deploymentPlanVersionsFromModels(models []deploymentPlanVersionModel) ([]deploydomain.DeploymentPlanVersion, error) {
	result := make([]deploydomain.DeploymentPlanVersion, 0, len(models))
	for _, model := range models {
		version, err := deploymentPlanVersionFromModel(model)
		if err != nil {
			return nil, err
		}
		result = append(result, version)
	}
	return result, nil
}

func joinDeploymentPlans(models []deploymentPlanModel, versions []deploydomain.DeploymentPlanVersion) []deploydomain.DeploymentPlan {
	byPlan := make(map[uuid.UUID][]deploydomain.DeploymentPlanVersion)
	for _, version := range versions {
		byPlan[version.PlanID] = append(byPlan[version.PlanID], version)
	}
	result := make([]deploydomain.DeploymentPlan, 0, len(models))
	for _, model := range models {
		result = append(result, deploymentPlanFromModel(model, byPlan[model.ID]))
	}
	return result
}

func planIDs(models []deploymentPlanModel) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(models))
	for _, model := range models {
		result = append(result, model.ID)
	}
	return result
}

func lockDeploymentPlan(tx *gorm.DB, planID uuid.UUID) error {
	var model deploymentPlanModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "id = ?", planID).Error; err != nil {
		return deploymentPlanReadError(err)
	}
	return nil
}

func verifyPlanVersionAppend(tx *gorm.DB, planID uuid.UUID, expected uint64) error {
	var latest, drafts int64
	if err := tx.Model(&deploymentPlanVersionModel{}).Where("plan_id = ?", planID).Select("COALESCE(MAX(version_number), 0)").Scan(&latest).Error; err != nil {
		return fmt.Errorf("read latest deployment plan version: %w", err)
	}
	if err := tx.Model(&deploymentPlanVersionModel{}).Where("plan_id = ? AND lifecycle = 'Draft'", planID).Count(&drafts).Error; err != nil {
		return fmt.Errorf("count deployment plan drafts: %w", err)
	}
	if uint64(latest) != expected || drafts > 0 {
		return deployapp.ErrPlanConflict
	}
	return nil
}

func appendDeploymentPlanChange(ctx context.Context, tx *gorm.DB, change deploymentPlanChange) error {
	payload, err := json.Marshal(change.metadata)
	if err != nil {
		return fmt.Errorf("marshal deployment plan event: %w", err)
	}
	event, err := platform.NewEvent("deployment_plan."+change.action, "deployment_plan", change.planID.String(), payload, change.mutation.OccurredAt)
	if err != nil {
		return fmt.Errorf("create deployment plan event: %w", err)
	}
	if err := appendDeploymentPlanAudit(ctx, tx, change); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func appendDeploymentPlanAudit(ctx context.Context, tx *gorm.DB, change deploymentPlanChange) error {
	actorID := change.mutation.ActorID
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: change.mutation.OccurredAt, ActorID: &actorID,
		Action: "deployment_plan." + change.action, ResourceType: "deployment_plan",
		ResourceID: change.planID.String(), RequestID: change.mutation.RequestID, Metadata: change.metadata,
	})
}

func deploymentPlanReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.ErrPlanNotFound
	}
	return fmt.Errorf("read deployment plan: %w", err)
}

func deploymentPlanWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return deployapp.ErrPlanConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}
