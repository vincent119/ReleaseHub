package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestScopeInheritanceUsesCompleteResourceIdentity(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	environmentID, applicationID := uuid.New(), uuid.New()
	project, err := authz.NewProjectScope(organizationID, projectID)
	require.NoError(t, err)
	environment, err := authz.NewEnvironmentScope(organizationID, projectID, environmentID)
	require.NoError(t, err)
	application, err := authz.NewApplicationScope(organizationID, projectID, environmentID, applicationID)
	require.NoError(t, err)

	require.True(t, project.Contains(environment))
	require.True(t, project.Contains(application))
	require.True(t, environment.Contains(application))
	require.False(t, environment.Contains(mustApplicationScope(t, organizationID, projectID, uuid.New(), applicationID)))
	require.False(t, project.Contains(mustProjectScope(t, organizationID, uuid.New())))
	require.Contains(t, application.Path(), applicationID.String())
}

func TestScopeRejectsIncompleteHierarchy(t *testing.T) {
	_, err := authz.NewApplicationScope(uuid.New(), uuid.New(), uuid.Nil, uuid.New())
	require.ErrorContains(t, err, "requires environment and application")
}

func TestPermissionRequiresNamespacedKey(t *testing.T) {
	permission, err := authz.NewPermission("deployment.execute")
	require.NoError(t, err)
	require.Equal(t, authz.Permission("deployment.execute"), permission)
	_, err = authz.NewPermission("execute")
	require.Error(t, err)
}

func TestPlatformScopeUsesGlobalPolicyPath(t *testing.T) {
	scope := authz.NewPlatformScope()
	require.Equal(t, authz.ScopePlatform, scope.Kind)
	require.Equal(t, "/platform", scope.Path())
	require.Equal(t, "platform", scope.Tenant())
	require.True(t, scope.Contains(authz.NewPlatformScope()))
}

func mustProjectScope(t *testing.T, organizationID, projectID uuid.UUID) authz.Scope {
	t.Helper()
	scope, err := authz.NewProjectScope(organizationID, projectID)
	require.NoError(t, err)
	return scope
}

func mustApplicationScope(t *testing.T, organizationID, projectID, environmentID, applicationID uuid.UUID) authz.Scope {
	t.Helper()
	scope, err := authz.NewApplicationScope(organizationID, projectID, environmentID, applicationID)
	require.NoError(t, err)
	return scope
}
