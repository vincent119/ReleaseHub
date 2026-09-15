// Package application coordinates resource catalog use cases.
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

// ErrResourceNotFound intentionally represents both missing and unauthorized resources.
var ErrResourceNotFound = errors.New("resource not found")

// ErrInvalidResource identifies malformed catalog mutation input.
var ErrInvalidResource = errors.New("invalid resource mutation")

// ErrResourceConflict identifies a stale or conflicting catalog mutation.
var ErrResourceConflict = errors.New("resource mutation conflict")

// Mutation identifies the actor and request attached to an audited catalog change.
type Mutation struct {
	ActorID   *uuid.UUID
	RequestID string
}

// Writer is the transactional persistence boundary used by authorized command handlers.
type Writer interface {
	CreateOrganization(context.Context, Mutation, domain.Organization) error
	RenameOrganization(context.Context, Mutation, domain.Organization, domain.Organization) error
	DeleteOrganization(context.Context, Mutation, domain.Organization, domain.Organization, uuid.UUID) error
	CreateProject(context.Context, Mutation, domain.Project) error
	CreateEnvironment(context.Context, Mutation, domain.Environment) error
	CreateApplication(context.Context, Mutation, domain.Application) error
	CreateEnvironmentLabelMapping(context.Context, Mutation, domain.EnvironmentLabelMapping) error
}

// Reader is the persistence boundary for catalog queries.
type Reader interface {
	FindOrganization(context.Context, uuid.UUID) (domain.Organization, error)
	FindApplication(context.Context, uuid.UUID) (domain.Application, error)
	ListActiveOrganizations(context.Context) ([]domain.Organization, error)
	ListActiveProjects(context.Context) ([]domain.Project, error)
	ListOrganizationProjectCounts(context.Context) (map[uuid.UUID]uint64, error)
	ListActiveEnvironments(context.Context) ([]domain.Environment, error)
	ListActiveApplications(context.Context) ([]domain.Application, error)
	ListApplications(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.Application, error)
	ResolveEnvironmentLabel(context.Context, uuid.UUID, uuid.UUID, string, string) (domain.Environment, error)
}

// Repository combines the catalog query and transactional command boundaries.
type Repository interface {
	Reader
	Writer
}

// ListVisibleApplications returns only active Applications that the principal may view at Application scope.
func (s *Service) ListVisibleApplications(ctx context.Context, principal Principal) ([]domain.Application, error) {
	if principal.Disabled {
		return []domain.Application{}, nil
	}
	values, err := s.repository.ListActiveApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active Applications: %w", err)
	}
	result := make([]domain.Application, 0, len(values))
	for _, value := range values {
		scope, err := authz.NewApplicationScope(value.OrganizationID, value.ProjectID, value.EnvironmentID, value.ID)
		if err != nil {
			return nil, fmt.Errorf("build Application authorization scope: %w", err)
		}
		allowed, err := s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: false, Permission: s.view, Scope: scope})
		if err != nil {
			return nil, fmt.Errorf("authorize visible Application query: %w", err)
		}
		if allowed {
			result = append(result, value)
		}
	}
	return result, nil
}

// Authorizer evaluates a complete tenant-aware permission request.
type Authorizer interface {
	Authorize(context.Context, authz.AuthorizationRequest) (bool, error)
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// Principal is the authenticated local user used for catalog queries.
type Principal struct {
	UserID   uuid.UUID
	Disabled bool
}

// Service provides tenant-isolated catalog queries.
type Service struct {
	repository            Repository
	authorizer            Authorizer
	view                  authz.Permission
	platformManage        authz.Permission
	projectManage         authz.Permission
	defaultOrganizationID uuid.UUID
}

// NewService creates a catalog application service.
func NewService(repository Repository, authorizer Authorizer, defaultOrganizationID uuid.UUID) (*Service, error) {
	if repository == nil || authorizer == nil || defaultOrganizationID == uuid.Nil {
		return nil, fmt.Errorf("catalog repository, authorizer, and default Organization ID are required")
	}
	view, err := authz.NewPermission("resource.view")
	if err != nil {
		return nil, err
	}
	platformManage, err := authz.NewPermission("platform.manage")
	if err != nil {
		return nil, err
	}
	projectManage, err := authz.NewPermission("project.manage")
	if err != nil {
		return nil, err
	}
	return &Service{
		repository: repository, authorizer: authorizer, view: view,
		platformManage: platformManage, projectManage: projectManage,
		defaultOrganizationID: defaultOrganizationID,
	}, nil
}

// CreateOrganization creates a tenant boundary after platform authorization.
func (s *Service) CreateOrganization(ctx context.Context, principal Principal, mutation Mutation, name string) (domain.Organization, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return domain.Organization{}, fmt.Errorf("authorize Organization creation: %w", err)
	}
	if !allowed {
		return domain.Organization{}, ErrResourceNotFound
	}
	value, err := domain.NewOrganization(name)
	if err != nil {
		return domain.Organization{}, err
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.CreateOrganization(ctx, mutation, value); err != nil {
		return domain.Organization{}, fmt.Errorf("create Organization: %w", err)
	}
	return value, nil
}

// RenameOrganization changes only the display name of a platform-managed tenant boundary.
func (s *Service) RenameOrganization(ctx context.Context, principal Principal, mutation Mutation, organizationID uuid.UUID, name string, expectedVersion uint64) (domain.Organization, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return domain.Organization{}, fmt.Errorf("authorize Organization rename: %w", err)
	}
	if !allowed {
		return domain.Organization{}, ErrResourceNotFound
	}
	if organizationID == uuid.Nil || expectedVersion == 0 {
		return domain.Organization{}, ErrInvalidResource
	}
	current, err := s.repository.FindOrganization(ctx, organizationID)
	if err != nil {
		return domain.Organization{}, ErrResourceNotFound
	}
	if current.Version != expectedVersion {
		return domain.Organization{}, ErrResourceConflict
	}
	renamed, err := current.Rename(name)
	if err != nil {
		return domain.Organization{}, fmt.Errorf("%w: %v", ErrInvalidResource, err)
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.RenameOrganization(ctx, mutation, current, renamed); err != nil {
		return domain.Organization{}, fmt.Errorf("rename Organization: %w", err)
	}
	return renamed, nil
}

// DeleteOrganization deactivates an empty non-default tenant boundary after fresh platform authorization.
func (s *Service) DeleteOrganization(ctx context.Context, principal Principal, mutation Mutation, organizationID uuid.UUID, expectedVersion uint64) error {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return fmt.Errorf("authorize Organization deletion: %w", err)
	}
	if !allowed {
		return ErrResourceNotFound
	}
	if organizationID == uuid.Nil || expectedVersion == 0 {
		return ErrInvalidResource
	}
	current, err := s.repository.FindOrganization(ctx, organizationID)
	if err != nil {
		return ErrResourceNotFound
	}
	if current.Version != expectedVersion {
		return ErrResourceConflict
	}
	deactivated, err := current.Deactivate(s.defaultOrganizationID)
	if errors.Is(err, domain.ErrDefaultOrganizationProtected) {
		return ErrResourceConflict
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResource, err)
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.DeleteOrganization(ctx, mutation, current, deactivated, s.defaultOrganizationID); err != nil {
		return fmt.Errorf("delete Organization: %w", err)
	}
	return nil
}

// CreateProject creates a Project after platform authorization.
func (s *Service) CreateProject(ctx context.Context, principal Principal, mutation Mutation, organizationID uuid.UUID, name string) (domain.Project, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return domain.Project{}, fmt.Errorf("authorize Project creation: %w", err)
	}
	if !allowed {
		return domain.Project{}, ErrResourceNotFound
	}
	value, err := domain.NewProject(organizationID, name)
	if err != nil {
		return domain.Project{}, err
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.CreateProject(ctx, mutation, value); err != nil {
		return domain.Project{}, fmt.Errorf("create Project: %w", err)
	}
	return value, nil
}

// CreateEnvironment creates an Environment for a platform or Project manager.
func (s *Service) CreateEnvironment(ctx context.Context, principal Principal, mutation Mutation, organizationID, projectID uuid.UUID, name string, environmentType domain.EnvironmentType) (domain.Environment, error) {
	allowed, err := s.mayManageProject(ctx, principal, organizationID, projectID)
	if err != nil {
		return domain.Environment{}, err
	}
	if !allowed {
		return domain.Environment{}, ErrResourceNotFound
	}
	value, err := domain.NewEnvironment(organizationID, projectID, name, environmentType)
	if err != nil {
		return domain.Environment{}, err
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.CreateEnvironment(ctx, mutation, value); err != nil {
		return domain.Environment{}, fmt.Errorf("create Environment: %w", err)
	}
	return value, nil
}

// CreateEnvironmentLabelMapping creates an explicit Argo CD label mapping for a manageable Project.
func (s *Service) CreateEnvironmentLabelMapping(ctx context.Context, principal Principal, mutation Mutation, organizationID, projectID, environmentID uuid.UUID, key, value string) (domain.EnvironmentLabelMapping, error) {
	allowed, err := s.mayManageProject(ctx, principal, organizationID, projectID)
	if err != nil {
		return domain.EnvironmentLabelMapping{}, err
	}
	if !allowed {
		return domain.EnvironmentLabelMapping{}, ErrResourceNotFound
	}
	mapping, err := domain.NewEnvironmentLabelMapping(organizationID, projectID, environmentID, key, value)
	if err != nil {
		return domain.EnvironmentLabelMapping{}, err
	}
	mutation.ActorID = &principal.UserID
	if err := s.repository.CreateEnvironmentLabelMapping(ctx, mutation, mapping); err != nil {
		return domain.EnvironmentLabelMapping{}, fmt.Errorf("create Environment label mapping: %w", err)
	}
	return mapping, nil
}

func (s *Service) mayManageProject(ctx context.Context, principal Principal, organizationID, projectID uuid.UUID) (bool, error) {
	platformAllowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil || platformAllowed {
		return platformAllowed, err
	}
	scope, err := authz.NewProjectScope(organizationID, projectID)
	if err != nil {
		return false, err
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.projectManage, Scope: scope})
	if err != nil {
		return false, fmt.Errorf("authorize Project management: %w", err)
	}
	return allowed, nil
}

// ResourceTree contains only catalog ancestors and descendants visible to one principal.
type ResourceTree struct {
	Organizations         []OrganizationNode
	CanCreateOrganization bool
}

// OrganizationNode is one visible Organization and its visible Projects.
type OrganizationNode struct {
	Organization     domain.Organization
	Projects         []ProjectNode
	CanCreateProject bool
	CanRename        bool
	CanDelete        bool
	IsDefault        bool
}

// ProjectNode is one visible Project and its visible Environments.
type ProjectNode struct {
	Project      domain.Project
	Environments []EnvironmentNode
	CanManage    bool
}

// EnvironmentNode is one visible Environment and its visible Applications.
type EnvironmentNode struct {
	Environment  domain.Environment
	Applications []domain.Application
}

// ListResourceTree returns the platform catalog without exposing unauthorized ancestor names.
func (s *Service) ListResourceTree(ctx context.Context, principal Principal) (ResourceTree, error) {
	if principal.Disabled {
		return ResourceTree{Organizations: []OrganizationNode{}}, nil
	}
	platformAllowed, err := s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Permission: s.platformManage, Scope: authz.NewPlatformScope()})
	if err != nil {
		return ResourceTree{}, fmt.Errorf("authorize platform catalog query: %w", err)
	}
	organizations, err := s.repository.ListActiveOrganizations(ctx)
	if err != nil {
		return ResourceTree{}, fmt.Errorf("list active Organizations: %w", err)
	}
	projects, err := s.repository.ListActiveProjects(ctx)
	if err != nil {
		return ResourceTree{}, fmt.Errorf("list active Projects: %w", err)
	}
	environments, err := s.repository.ListActiveEnvironments(ctx)
	if err != nil {
		return ResourceTree{}, fmt.Errorf("list active Environments: %w", err)
	}
	applications, err := s.repository.ListActiveApplications(ctx)
	if err != nil {
		return ResourceTree{}, fmt.Errorf("list active Applications: %w", err)
	}
	projectCounts := map[uuid.UUID]uint64{}
	if platformAllowed {
		projectCounts, err = s.repository.ListOrganizationProjectCounts(ctx)
		if err != nil {
			return ResourceTree{}, fmt.Errorf("list Organization Project counts: %w", err)
		}
	}

	result := ResourceTree{Organizations: make([]OrganizationNode, 0), CanCreateOrganization: platformAllowed}
	for _, organization := range organizations {
		organizationNode := OrganizationNode{
			Organization: organization, Projects: make([]ProjectNode, 0),
			CanCreateProject: platformAllowed, CanRename: platformAllowed,
			IsDefault: organization.IsDefault(s.defaultOrganizationID),
		}
		organizationNode.CanDelete = platformAllowed && !organizationNode.IsDefault && projectCounts[organization.ID] == 0
		for _, project := range projects {
			if project.OrganizationID != organization.ID {
				continue
			}
			projectScope, scopeErr := authz.NewProjectScope(organization.ID, project.ID)
			if scopeErr != nil {
				return ResourceTree{}, scopeErr
			}
			projectAllowed := platformAllowed
			if !projectAllowed {
				projectAllowed, err = s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Permission: s.projectManage, Scope: projectScope})
				if err != nil {
					return ResourceTree{}, fmt.Errorf("authorize Project catalog query: %w", err)
				}
			}
			projectNode := ProjectNode{Project: project, Environments: make([]EnvironmentNode, 0), CanManage: projectAllowed}
			for _, environment := range environments {
				if environment.OrganizationID != organization.ID || environment.ProjectID != project.ID {
					continue
				}
				environmentNode := EnvironmentNode{Environment: environment, Applications: make([]domain.Application, 0)}
				for _, application := range applications {
					if application.EnvironmentID != environment.ID {
						continue
					}
					visible := projectAllowed
					if !visible {
						applicationScope, scopeErr := authz.NewApplicationScope(organization.ID, project.ID, environment.ID, application.ID)
						if scopeErr != nil {
							return ResourceTree{}, scopeErr
						}
						visible, err = s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Permission: s.view, Scope: applicationScope})
						if err != nil {
							return ResourceTree{}, fmt.Errorf("authorize Application catalog tree query: %w", err)
						}
					}
					if visible {
						environmentNode.Applications = append(environmentNode.Applications, application)
					}
				}
				if projectAllowed || len(environmentNode.Applications) > 0 {
					projectNode.Environments = append(projectNode.Environments, environmentNode)
				}
			}
			if projectAllowed || len(projectNode.Environments) > 0 {
				organizationNode.Projects = append(organizationNode.Projects, projectNode)
			}
		}
		if platformAllowed || len(organizationNode.Projects) > 0 {
			result.Organizations = append(result.Organizations, organizationNode)
		}
	}
	return result, nil
}

// FindApplication returns the same result for a missing and an unauthorized resource.
func (s *Service) FindApplication(ctx context.Context, principal Principal, applicationID uuid.UUID) (domain.Application, error) {
	target, err := s.repository.FindApplication(ctx, applicationID)
	if err != nil {
		return domain.Application{}, ErrResourceNotFound
	}
	scope, err := authz.NewApplicationScope(target.OrganizationID, target.ProjectID, target.EnvironmentID, target.ID)
	if err != nil {
		return domain.Application{}, fmt.Errorf("build Application authorization scope: %w", err)
	}
	allowed, err := s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.view, Scope: scope})
	if err != nil {
		return domain.Application{}, fmt.Errorf("authorize Application query: %w", err)
	}
	if !allowed {
		return domain.Application{}, ErrResourceNotFound
	}
	return target, nil
}

// ListApplications authorizes the exact Environment before reading any child records.
func (s *Service) ListApplications(ctx context.Context, principal Principal, organizationID, projectID, environmentID uuid.UUID) ([]domain.Application, error) {
	scope, err := authz.NewEnvironmentScope(organizationID, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.authorizer.Authorize(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.view, Scope: scope})
	if err != nil {
		return nil, fmt.Errorf("authorize Environment query: %w", err)
	}
	if !allowed {
		return []domain.Application{}, nil
	}
	return s.repository.ListApplications(ctx, organizationID, projectID, environmentID)
}
