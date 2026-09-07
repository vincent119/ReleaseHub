package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

type deploymentBindingModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	Version           uint64
	Active            bool
	CreatedBy         uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (deploymentBindingModel) TableName() string { return "deployment_bindings" }

// DeploymentBindingRepository stores the current version-pinned Environment binding.
type DeploymentBindingRepository struct{ db *gorm.DB }

var _ deployapp.DeploymentBindingRepository = (*DeploymentBindingRepository)(nil)

// NewDeploymentBindingRepository creates the PostgreSQL binding repository.
func NewDeploymentBindingRepository(db *gorm.DB) (*DeploymentBindingRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentBindingRepository{db: db}, nil
}

// ResolveEnvironment verifies the complete hierarchy and returns its canonical scope.
func (r *DeploymentBindingRepository) ResolveEnvironment(ctx context.Context, input deployapp.BindingScopeInput) (authz.Scope, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("environments").Where(
		"id = ? AND project_id = ? AND organization_id = ?", input.EnvironmentID, input.ProjectID, input.OrganizationID,
	).Count(&count).Error
	if err != nil {
		return authz.Scope{}, fmt.Errorf("resolve deployment binding Environment: %w", err)
	}
	if count != 1 {
		return authz.Scope{}, deployapp.ErrBindingNotFound
	}
	scope, err := authz.NewEnvironmentScope(input.OrganizationID, input.ProjectID, input.EnvironmentID)
	if err != nil {
		return authz.Scope{}, deployapp.ErrBindingNotFound
	}
	return scope, nil
}

// Get returns the active binding or nil when the Environment is unbound.
func (r *DeploymentBindingRepository) Get(ctx context.Context, scope authz.Scope) (*deploydomain.DeploymentBinding, error) {
	model, err := readActiveBinding(r.db.WithContext(ctx), scope, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deployment binding: %w", err)
	}
	value := bindingFromModel(model)
	return &value, nil
}

// Bind atomically creates or switches a binding after published-version checks.
func (r *DeploymentBindingRepository) Bind(ctx context.Context, mutation deployapp.BindingMutation, scope authz.Scope, input deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	var result deploydomain.DeploymentBinding
	err := database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := lockBindingEnvironment(tx, scope); err != nil {
			return err
		}
		if err := validateBindingVersions(tx, scope.ProjectID, input); err != nil {
			return err
		}
		value, err := applyBindingChange(tx, mutation, scope, input)
		if err != nil {
			return err
		}
		result = value
		return appendBindingChange(ctx, tx, mutation, value)
	})
	return result, err
}

func applyBindingChange(tx *gorm.DB, mutation deployapp.BindingMutation, scope authz.Scope, input deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	model, err := readActiveBinding(tx, scope, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return createBinding(tx, mutation, scope, input)
	}
	if err != nil {
		return deploydomain.DeploymentBinding{}, fmt.Errorf("lock deployment binding: %w", err)
	}
	return updateBinding(tx, model, mutation, input)
}

func createBinding(tx *gorm.DB, mutation deployapp.BindingMutation, scope authz.Scope, input deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	if input.ExpectedVersion != 0 {
		return deploydomain.DeploymentBinding{}, deployapp.ErrBindingConflict
	}
	value, err := deploydomain.NewDeploymentBinding(deploydomain.DeploymentBinding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID,
		EnvironmentID: scope.EnvironmentID, WorkflowVersionID: input.WorkflowVersionID,
		PlanVersionID: input.PlanVersionID, CreatedBy: mutation.ActorID, CreatedAt: mutation.OccurredAt,
	})
	if err != nil {
		return deploydomain.DeploymentBinding{}, deployapp.ErrBindingConflict
	}
	model := bindingToModel(value)
	if err := tx.Create(&model).Error; err != nil {
		return deploydomain.DeploymentBinding{}, bindingWriteError(err)
	}
	return value, nil
}

func updateBinding(tx *gorm.DB, model deploymentBindingModel, mutation deployapp.BindingMutation, input deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	value, err := bindingFromModel(model).Rebind(input.WorkflowVersionID, input.PlanVersionID, input.ExpectedVersion, mutation.OccurredAt)
	if err != nil {
		return deploydomain.DeploymentBinding{}, deployapp.ErrBindingConflict
	}
	result := tx.Model(&deploymentBindingModel{}).Where("id = ? AND version = ?", value.ID, input.ExpectedVersion).Updates(map[string]any{
		"workflow_version_id": value.WorkflowVersionID, "plan_version_id": value.PlanVersionID,
		"version": value.Version, "updated_at": value.UpdatedAt,
	})
	if result.Error != nil {
		return deploydomain.DeploymentBinding{}, bindingWriteError(result.Error)
	}
	if result.RowsAffected != 1 {
		return deploydomain.DeploymentBinding{}, deployapp.ErrBindingConflict
	}
	return value, nil
}

func readActiveBinding(db *gorm.DB, scope authz.Scope, lock bool) (deploymentBindingModel, error) {
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var model deploymentBindingModel
	err := db.Where("organization_id = ? AND project_id = ? AND environment_id = ? AND active", scope.OrganizationID, scope.ProjectID, scope.EnvironmentID).First(&model).Error
	return model, err
}

func lockBindingEnvironment(tx *gorm.DB, scope authz.Scope) error {
	var row struct{ ID uuid.UUID }
	err := tx.Table("environments").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(
		&row, "id = ? AND project_id = ? AND organization_id = ?", scope.EnvironmentID, scope.ProjectID, scope.OrganizationID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deployapp.ErrBindingNotFound
	}
	return err
}

func validateBindingVersions(tx *gorm.DB, projectID uuid.UUID, input deployapp.BindDefinitionsInput) error {
	workflows, err := countPublishedWorkflow(tx, input.WorkflowVersionID)
	if err != nil {
		return err
	}
	plans, err := countPublishedPlan(tx, projectID, input.PlanVersionID)
	if err != nil {
		return err
	}
	if workflows != 1 || plans != 1 {
		return deployapp.ErrBindingConflict
	}
	return nil
}

func countPublishedWorkflow(tx *gorm.DB, versionID uuid.UUID) (int64, error) {
	var count int64
	err := tx.Table("release_workflow_versions v").Joins("JOIN release_workflows w ON w.id = v.workflow_id").Where(
		"v.id = ? AND v.lifecycle = 'Published' AND w.active", versionID,
	).Count(&count).Error
	return count, err
}

func countPublishedPlan(tx *gorm.DB, projectID, versionID uuid.UUID) (int64, error) {
	var count int64
	err := tx.Table("deployment_plan_versions v").Joins("JOIN deployment_plans p ON p.id = v.plan_id").Where(
		"v.id = ? AND v.lifecycle = 'Published' AND p.active AND (p.owner_kind = 'platform' OR p.owner_project_id = ?)", versionID, projectID,
	).Count(&count).Error
	return count, err
}

func appendBindingChange(ctx context.Context, tx *gorm.DB, mutation deployapp.BindingMutation, value deploydomain.DeploymentBinding) error {
	metadata := map[string]any{"environmentId": value.EnvironmentID, "workflowVersionId": value.WorkflowVersionID, "planVersionId": value.PlanVersionID, "version": value.Version}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal deployment binding event: %w", err)
	}
	event, err := platform.NewEvent("deployment_binding.changed", "deployment_binding", value.ID.String(), payload, mutation.OccurredAt)
	if err != nil {
		return fmt.Errorf("create deployment binding event: %w", err)
	}
	actorID := mutation.ActorID
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: mutation.OccurredAt, ActorID: &actorID, Action: "deployment_binding.changed",
		ResourceType: "deployment_binding", ResourceID: value.ID.String(), RequestID: mutation.RequestID, Metadata: metadata,
	}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func bindingToModel(value deploydomain.DeploymentBinding) deploymentBindingModel {
	return deploymentBindingModel{
		ID: value.ID, OrganizationID: value.OrganizationID, ProjectID: value.ProjectID,
		EnvironmentID: value.EnvironmentID, WorkflowVersionID: value.WorkflowVersionID,
		PlanVersionID: value.PlanVersionID, Version: value.Version, Active: true,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func bindingFromModel(model deploymentBindingModel) deploydomain.DeploymentBinding {
	return deploydomain.DeploymentBinding{
		ID: model.ID, OrganizationID: model.OrganizationID, ProjectID: model.ProjectID,
		EnvironmentID: model.EnvironmentID, WorkflowVersionID: model.WorkflowVersionID,
		PlanVersionID: model.PlanVersionID, Version: model.Version, CreatedBy: model.CreatedBy,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}

func bindingWriteError(err error) error {
	return fmt.Errorf("write deployment binding: %w", err)
}
