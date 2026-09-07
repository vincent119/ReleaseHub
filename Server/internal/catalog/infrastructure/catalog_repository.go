// Package infrastructure contains PostgreSQL adapters for the resource catalog.
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// CatalogRepository persists catalog resources with audit and outbox records.
type CatalogRepository struct{ db *gorm.DB }

var _ application.Reader = (*CatalogRepository)(nil)
var _ application.Writer = (*CatalogRepository)(nil)

// NewCatalogRepository creates a PostgreSQL catalog adapter.
func NewCatalogRepository(db *gorm.DB) (*CatalogRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &CatalogRepository{db: db}, nil
}

func (r *CatalogRepository) CreateOrganization(ctx context.Context, mutation application.Mutation, value catalog.Organization) error {
	model := organizationModel{ID: value.ID, Name: value.Name, Active: value.Active, Version: value.Version}
	return r.create(ctx, mutation, &value.ID, "organization", value.ID, "catalog.organization.created", model)
}

func (r *CatalogRepository) CreateProject(ctx context.Context, mutation application.Mutation, value catalog.Project) error {
	model := projectModel{ID: value.ID, OrganizationID: value.OrganizationID, Name: value.Name, Active: value.Active, Version: value.Version}
	return r.create(ctx, mutation, &value.OrganizationID, "project", value.ID, "catalog.project.created", model)
}

func (r *CatalogRepository) CreateEnvironment(ctx context.Context, mutation application.Mutation, value catalog.Environment) error {
	model := environmentModel{ID: value.ID, OrganizationID: value.OrganizationID, ProjectID: value.ProjectID, Name: value.Name, EnvironmentType: string(value.Type), Active: value.Active, Version: value.Version}
	return r.create(ctx, mutation, &value.OrganizationID, "environment", value.ID, "catalog.environment.created", model)
}

func (r *CatalogRepository) CreateApplication(ctx context.Context, mutation application.Mutation, value catalog.Application) error {
	model := applicationModel{
		ID: value.ID, OrganizationID: value.OrganizationID, ProjectID: value.ProjectID, EnvironmentID: value.EnvironmentID,
		Name: value.Name, ArgoCDNamespace: value.Argo.Namespace, ArgoCDApplicationName: value.Argo.Name,
		ArgoCDProject: value.ArgoProject, DestinationServer: value.DestinationServer, DestinationNamespace: value.DestinationNamespace,
		SourceRepositoryURL: value.Source.RepositoryURL, SourceTargetRevision: value.Source.TargetRevision, SourcePath: value.Source.Path,
		Active: value.Active, Version: value.Version,
	}
	return r.create(ctx, mutation, &value.OrganizationID, "application", value.ID, "catalog.application.created", model)
}

func (r *CatalogRepository) CreateEnvironmentLabelMapping(ctx context.Context, mutation application.Mutation, value catalog.EnvironmentLabelMapping) error {
	model := environmentLabelMappingModel{ID: value.ID, OrganizationID: value.OrganizationID, ProjectID: value.ProjectID, EnvironmentID: value.EnvironmentID, LabelKey: value.LabelKey, LabelValue: value.LabelValue}
	return r.create(ctx, mutation, &value.OrganizationID, "environment_label_mapping", value.ID, "catalog.environment_label_mapping.created", model)
}

func (r *CatalogRepository) FindApplication(ctx context.Context, applicationID uuid.UUID) (catalog.Application, error) {
	var model applicationModel
	if err := r.db.WithContext(ctx).Where("id = ? AND active", applicationID).Take(&model).Error; err != nil {
		return catalog.Application{}, fmt.Errorf("find Application: %w", err)
	}
	return model.domain(), nil
}

// ListActiveOrganizations returns active tenant boundaries for authorization-aware tree assembly.
func (r *CatalogRepository) ListActiveOrganizations(ctx context.Context) ([]catalog.Organization, error) {
	var models []organizationModel
	if err := r.db.WithContext(ctx).Where("active").Order("lower(name), id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list active Organizations: %w", err)
	}
	result := make([]catalog.Organization, 0, len(models))
	for _, model := range models {
		result = append(result, catalog.Organization{ID: model.ID, Name: model.Name, Active: model.Active, Version: model.Version})
	}
	return result, nil
}

// ListActiveProjects returns active Projects before application-layer authorization filtering.
func (r *CatalogRepository) ListActiveProjects(ctx context.Context) ([]catalog.Project, error) {
	var models []projectModel
	if err := r.db.WithContext(ctx).Where("active").Order("organization_id, lower(name), id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list active Projects: %w", err)
	}
	result := make([]catalog.Project, 0, len(models))
	for _, model := range models {
		result = append(result, catalog.Project{ID: model.ID, OrganizationID: model.OrganizationID, Name: model.Name, Active: model.Active, Version: model.Version})
	}
	return result, nil
}

// ListActiveEnvironments returns active Environments before application-layer authorization filtering.
func (r *CatalogRepository) ListActiveEnvironments(ctx context.Context) ([]catalog.Environment, error) {
	var models []environmentModel
	if err := r.db.WithContext(ctx).Where("active").Order("organization_id, project_id, lower(name), id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list active Environments: %w", err)
	}
	result := make([]catalog.Environment, 0, len(models))
	for _, model := range models {
		result = append(result, model.domain())
	}
	return result, nil
}

// ListActiveApplications returns catalog entries before the application service applies caller-specific authorization.
func (r *CatalogRepository) ListActiveApplications(ctx context.Context) ([]catalog.Application, error) {
	var models []applicationModel
	if err := r.db.WithContext(ctx).Where("active").Order("organization_id, project_id, environment_id, lower(name), id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list active Applications: %w", err)
	}
	result := make([]catalog.Application, 0, len(models))
	for _, model := range models {
		result = append(result, model.domain())
	}
	return result, nil
}

func (r *CatalogRepository) ListApplications(ctx context.Context, organizationID, projectID, environmentID uuid.UUID) ([]catalog.Application, error) {
	var models []applicationModel
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND project_id = ? AND environment_id = ? AND active", organizationID, projectID, environmentID).
		Order("lower(name), id").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list Applications: %w", err)
	}
	result := make([]catalog.Application, 0, len(models))
	for _, model := range models {
		result = append(result, model.domain())
	}
	return result, nil
}

func (r *CatalogRepository) ResolveEnvironmentLabel(ctx context.Context, organizationID, projectID uuid.UUID, key, value string) (catalog.Environment, error) {
	var model environmentModel
	err := r.db.WithContext(ctx).Table("environments").Select("environments.*").
		Joins("JOIN environment_label_mappings mapping ON mapping.environment_id = environments.id AND mapping.project_id = environments.project_id AND mapping.organization_id = environments.organization_id").
		Where("mapping.organization_id = ? AND mapping.project_id = ? AND mapping.label_key = ? AND mapping.label_value = ? AND environments.active", organizationID, projectID, key, value).
		Take(&model).Error
	if err != nil {
		return catalog.Environment{}, fmt.Errorf("resolve Environment label mapping: %w", err)
	}
	return model.domain(), nil
}

func (r *CatalogRepository) create(ctx context.Context, mutation application.Mutation, organizationID *uuid.UUID, resourceType string, resourceID uuid.UUID, eventType string, model any) error {
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]string{"resourceId": resourceID.String()})
	if err != nil {
		return fmt.Errorf("marshal catalog event: %w", err)
	}
	event, err := platform.NewEvent(eventType, resourceType, resourceID.String(), payload, now)
	if err != nil {
		return fmt.Errorf("create catalog event: %w", err)
	}
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(model).Error; err != nil {
			return fmt.Errorf("create %s: %w", resourceType, err)
		}
		if err := database.AppendAudit(ctx, tx, database.AuditRecord{
			OccurredAt: now, ActorID: mutation.ActorID, OrganizationID: organizationID,
			Action: resourceType + ".create", ResourceType: resourceType, ResourceID: resourceID.String(),
			RequestID: mutation.RequestID, Metadata: map[string]any{},
		}); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
}

type organizationModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name    string
	Active  bool
	Version uint64
}

func (organizationModel) TableName() string { return "organizations" }

type projectModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Name           string
	Active         bool
	Version        uint64
}

func (projectModel) TableName() string { return "projects" }

type environmentModel struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID  uuid.UUID `gorm:"type:uuid"`
	ProjectID       uuid.UUID `gorm:"type:uuid"`
	Name            string
	EnvironmentType string
	Active          bool
	Version         uint64
}

func (environmentModel) TableName() string { return "environments" }
func (m environmentModel) domain() catalog.Environment {
	return catalog.Environment{ID: m.ID, OrganizationID: m.OrganizationID, ProjectID: m.ProjectID, Name: m.Name, Type: catalog.EnvironmentType(m.EnvironmentType), Active: m.Active, Version: m.Version}
}

type applicationModel struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID        uuid.UUID `gorm:"type:uuid"`
	ProjectID             uuid.UUID `gorm:"type:uuid"`
	EnvironmentID         uuid.UUID `gorm:"type:uuid"`
	Name                  string
	ArgoCDNamespace       string `gorm:"column:argocd_namespace"`
	ArgoCDApplicationName string `gorm:"column:argocd_application_name"`
	ArgoCDProject         string `gorm:"column:argocd_project"`
	DestinationServer     string
	DestinationNamespace  string
	SourceRepositoryURL   string `gorm:"column:source_repo_url"`
	SourceTargetRevision  string
	SourcePath            string
	Active                bool
	Version               uint64
}

func (applicationModel) TableName() string { return "applications" }
func (m applicationModel) domain() catalog.Application {
	return catalog.Application{
		ID: m.ID, OrganizationID: m.OrganizationID, ProjectID: m.ProjectID, EnvironmentID: m.EnvironmentID, Name: m.Name,
		Argo: catalog.ArgoApplicationIdentity{Namespace: m.ArgoCDNamespace, Name: m.ArgoCDApplicationName}, ArgoProject: m.ArgoCDProject,
		DestinationServer: m.DestinationServer, DestinationNamespace: m.DestinationNamespace,
		Source: catalog.GitOpsSource{RepositoryURL: m.SourceRepositoryURL, TargetRevision: m.SourceTargetRevision, Path: m.SourcePath},
		Active: m.Active, Version: m.Version,
	}
}

type environmentLabelMappingModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	ProjectID      uuid.UUID `gorm:"type:uuid"`
	EnvironmentID  uuid.UUID `gorm:"type:uuid"`
	LabelKey       string
	LabelValue     string
}

func (environmentLabelMappingModel) TableName() string { return "environment_label_mappings" }
