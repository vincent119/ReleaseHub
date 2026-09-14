// Package domain contains the framework-independent ReleaseHub resource catalog.
package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
)

// ErrDefaultOrganizationProtected rejects permanent removal of the configured tenant root.
var ErrDefaultOrganizationProtected = errors.New("default Organization is protected")

// EnvironmentType classifies an Environment independently from its display name.
type EnvironmentType string

const (
	EnvironmentDevelopment EnvironmentType = "Development"
	EnvironmentTesting     EnvironmentType = "Testing"
	EnvironmentStaging     EnvironmentType = "Staging"
	EnvironmentProduction  EnvironmentType = "Production"
)

// Organization is the permanent tenant boundary, including in single-tenant mode.
type Organization struct {
	ID      uuid.UUID
	Name    string
	Active  bool
	Version uint64
}

// Project is a ReleaseHub logical grouping and is not an Argo CD Project.
type Project struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Active         bool
	Version        uint64
}

// Environment has an arbitrary name and one fixed classification.
type Environment struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	Type           EnvironmentType
	Active         bool
	Version        uint64
}

// GitOpsSource identifies the source consumed by one Argo CD Application.
type GitOpsSource struct {
	RepositoryURL  string
	TargetRevision string
	Path           string
}

// ArgoApplicationIdentity is unique within the configured Argo CD instance.
type ArgoApplicationIdentity struct {
	Namespace string
	Name      string
}

// Application maps one ReleaseHub resource to exactly one Argo CD Application.
type Application struct {
	ID                   uuid.UUID
	OrganizationID       uuid.UUID
	ProjectID            uuid.UUID
	EnvironmentID        uuid.UUID
	Name                 string
	Argo                 ArgoApplicationIdentity
	ArgoProject          string
	DestinationServer    string
	DestinationNamespace string
	Source               GitOpsSource
	Active               bool
	Version              uint64
}

// EnvironmentLabelMapping maps arbitrary Argo CD metadata to one Environment.
type EnvironmentLabelMapping struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	LabelKey       string
	LabelValue     string
}

// NewOrganization creates a validated active Organization.
func NewOrganization(name string) (Organization, error) {
	name, err := validateName("organization name", name)
	if err != nil {
		return Organization{}, err
	}
	return Organization{ID: uuid.New(), Name: name, Active: true, Version: 1}, nil
}

// Rename returns a new Organization version without changing its stable identity or lifecycle.
func (o Organization) Rename(name string) (Organization, error) {
	if o.ID == uuid.Nil || !o.Active || o.Version == 0 {
		return Organization{}, fmt.Errorf("active versioned Organization is required")
	}
	name, err := validateName("organization name", name)
	if err != nil {
		return Organization{}, err
	}
	o.Name = name
	o.Version++
	return o, nil
}

// IsDefault reports whether this Organization is the configured permanent tenant root.
func (o Organization) IsDefault(defaultOrganizationID uuid.UUID) bool {
	return defaultOrganizationID != uuid.Nil && o.ID == defaultOrganizationID
}

// ValidateDeletion enforces the permanent default Organization boundary for future lifecycle commands.
func (o Organization) ValidateDeletion(defaultOrganizationID uuid.UUID) error {
	if o.IsDefault(defaultOrganizationID) {
		return ErrDefaultOrganizationProtected
	}
	return nil
}

// NewProject creates a validated active Project inside an Organization.
func NewProject(organizationID uuid.UUID, name string) (Project, error) {
	if organizationID == uuid.Nil {
		return Project{}, fmt.Errorf("organization ID is required")
	}
	name, err := validateName("project name", name)
	if err != nil {
		return Project{}, err
	}
	return Project{ID: uuid.New(), OrganizationID: organizationID, Name: name, Active: true, Version: 1}, nil
}

// NewEnvironment creates an Environment with a custom name and fixed type.
func NewEnvironment(organizationID, projectID uuid.UUID, name string, environmentType EnvironmentType) (Environment, error) {
	if organizationID == uuid.Nil || projectID == uuid.Nil {
		return Environment{}, fmt.Errorf("organization and project IDs are required")
	}
	name, err := validateName("environment name", name)
	if err != nil {
		return Environment{}, err
	}
	if !environmentType.Valid() {
		return Environment{}, fmt.Errorf("unsupported environment type %q", environmentType)
	}
	return Environment{ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, Name: name, Type: environmentType, Active: true, Version: 1}, nil
}

// NewApplication creates a validated Application mapping without assuming a Git repository topology.
func NewApplication(organizationID, projectID, environmentID uuid.UUID, name string, argo ArgoApplicationIdentity, argoProject, destinationServer, destinationNamespace string, source GitOpsSource) (Application, error) {
	if organizationID == uuid.Nil || projectID == uuid.Nil || environmentID == uuid.Nil {
		return Application{}, fmt.Errorf("organization, project, and environment IDs are required")
	}
	name, err := validateName("application name", name)
	if err != nil {
		return Application{}, err
	}
	if err := argo.Validate(); err != nil {
		return Application{}, err
	}
	if err := source.Validate(); err != nil {
		return Application{}, err
	}
	for _, field := range []struct{ name, value string }{
		{"Argo CD project", argoProject},
		{"destination server", destinationServer},
		{"destination namespace", destinationNamespace},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Application{}, fmt.Errorf("%s is required", field.name)
		}
	}
	return Application{
		ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID,
		Name: name, Argo: ArgoApplicationIdentity{Namespace: strings.TrimSpace(argo.Namespace), Name: strings.TrimSpace(argo.Name)}, ArgoProject: strings.TrimSpace(argoProject),
		DestinationServer: strings.TrimSpace(destinationServer), DestinationNamespace: strings.TrimSpace(destinationNamespace),
		Source: source.Normalized(), Active: true, Version: 1,
	}, nil
}

// NewEnvironmentLabelMapping creates an explicit metadata-to-Environment mapping.
func NewEnvironmentLabelMapping(organizationID, projectID, environmentID uuid.UUID, key, value string) (EnvironmentLabelMapping, error) {
	if organizationID == uuid.Nil || projectID == uuid.Nil || environmentID == uuid.Nil {
		return EnvironmentLabelMapping{}, fmt.Errorf("organization, project, and environment IDs are required")
	}
	key, err := validateBounded("label key", key, 1, 317)
	if err != nil {
		return EnvironmentLabelMapping{}, err
	}
	value, err = validateBounded("label value", value, 0, 63)
	if err != nil {
		return EnvironmentLabelMapping{}, err
	}
	return EnvironmentLabelMapping{ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID, LabelKey: key, LabelValue: value}, nil
}

// Valid reports whether the Environment type belongs to the fixed platform set.
func (t EnvironmentType) Valid() bool {
	switch t {
	case EnvironmentDevelopment, EnvironmentTesting, EnvironmentStaging, EnvironmentProduction:
		return true
	default:
		return false
	}
}

// Validate checks the Argo CD identity without treating its name as a ReleaseHub resource key.
func (i ArgoApplicationIdentity) Validate() error {
	if strings.TrimSpace(i.Namespace) == "" || strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("Argo CD namespace and application name are required")
	}
	return nil
}

// Validate checks that a GitOps source can identify one Application source tree.
func (s GitOpsSource) Validate() error {
	if strings.TrimSpace(s.RepositoryURL) == "" || strings.TrimSpace(s.TargetRevision) == "" {
		return fmt.Errorf("GitOps repository URL and target revision are required")
	}
	cleaned := path.Clean(strings.TrimSpace(s.Path))
	if cleaned == "" || cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("GitOps source path must be a non-root relative path")
	}
	return nil
}

// Normalized trims source values and cleans the repository-relative path.
func (s GitOpsSource) Normalized() GitOpsSource {
	return GitOpsSource{RepositoryURL: strings.TrimSpace(s.RepositoryURL), TargetRevision: strings.TrimSpace(s.TargetRevision), Path: path.Clean(strings.TrimSpace(s.Path))}
}

func validateName(field, value string) (string, error) {
	return validateBounded(field, value, 1, 128)
}

func validateBounded(field, value string, minimum, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < minimum || len(value) > maximum {
		return "", fmt.Errorf("%s must contain between %d and %d characters", field, minimum, maximum)
	}
	return value, nil
}
