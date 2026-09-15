package infrastructure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
)

func TestRBACScopeAndDenyPrecedence(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	devID, productionID := uuid.New(), uuid.New()
	userID, applicationID := uuid.New(), uuid.New()
	project := mustProjectScope(t, organizationID, projectID)
	production := mustEnvironmentScope(t, organizationID, projectID, productionID)
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), organizationID.String(), project.Path(), "resource.view", "allow"},
		{userID.String(), organizationID.String(), project.Path(), "deployment.execute", "allow"},
		{userID.String(), organizationID.String(), production.Path(), "deployment.execute", "deny"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	view, _ := authz.NewPermission("resource.view")
	deploy, _ := authz.NewPermission("deployment.execute")
	devApplication := mustApplicationScope(t, organizationID, projectID, devID, applicationID)
	productionApplication := mustApplicationScope(t, organizationID, projectID, productionID, applicationID)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{UserID: userID, Permission: view, Scope: devApplication})
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = engine.Authorize(context.Background(), authz.AuthorizationRequest{UserID: userID, Permission: deploy, Scope: devApplication})
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = engine.Authorize(context.Background(), authz.AuthorizationRequest{UserID: userID, Permission: deploy, Scope: productionApplication})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPolicyEngineDefaultsToDenyAndHonorsLocalDisable(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	userID := uuid.New()
	scope := mustProjectScope(t, organizationID, projectID)
	permission, _ := authz.NewPermission("resource.view")
	source := &memoryPolicySource{revision: 1, policies: [][]string{{userID.String(), organizationID.String(), scope.Path(), string(permission), "allow"}}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{UserID: uuid.New(), Permission: permission, Scope: scope})
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = engine.Authorize(context.Background(), authz.AuthorizationRequest{UserID: userID, Disabled: true, Permission: permission, Scope: scope})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPlatformPolicyAppliesToProjectDescendants(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	permission, _ := authz.NewPermission("deployment_plan.manage")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), "platform", "/platform", string(permission), "allow"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	for name, scope := range map[string]authz.Scope{
		"project":     mustProjectScope(t, organizationID, projectID),
		"environment": mustEnvironmentScope(t, organizationID, projectID, uuid.New()),
	} {
		t.Run(name, func(t *testing.T) {
			allowed, authorizeErr := engine.Authorize(context.Background(), authz.AuthorizationRequest{
				UserID: userID, Permission: permission, Scope: scope,
			})
			require.NoError(t, authorizeErr)
			require.True(t, allowed)
		})
	}
}

func TestPlatformDeploymentRequestViewAppliesToEnvironmentDescendants(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	permission, _ := authz.NewPermission("deployment_request.view")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), "platform", "/platform", string(permission), "allow"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	for name, scope := range map[string]authz.Scope{
		"project":     mustProjectScope(t, organizationID, projectID),
		"environment": mustEnvironmentScope(t, organizationID, projectID, uuid.New()),
	} {
		t.Run(name, func(t *testing.T) {
			allowed, authorizeErr := engine.Authorize(context.Background(), authz.AuthorizationRequest{
				UserID: userID, Permission: permission, Scope: scope,
			})
			require.NoError(t, authorizeErr)
			require.True(t, allowed)
		})
	}
}

func TestPlatformDeploymentRequestViewHonorsDescendantDeny(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	scope := mustEnvironmentScope(t, organizationID, projectID, uuid.New())
	permission, _ := authz.NewPermission("deployment_request.view")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), "platform", "/platform", string(permission), "allow"},
		{userID.String(), organizationID.String(), scope.Path(), string(permission), "deny"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{
		UserID: userID, Permission: permission, Scope: scope,
	})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPlatformPolicyHonorsProjectDeny(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	scope := mustProjectScope(t, organizationID, projectID)
	permission, _ := authz.NewPermission("deployment_plan.manage")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), "platform", "/platform", string(permission), "allow"},
		{userID.String(), organizationID.String(), scope.Path(), string(permission), "deny"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{
		UserID: userID, Permission: permission, Scope: scope,
	})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestScopedPolicyDoesNotApplyOutsideItsProject(t *testing.T) {
	organizationID, userID := uuid.New(), uuid.New()
	grantedScope := mustProjectScope(t, organizationID, uuid.New())
	otherScope := mustProjectScope(t, organizationID, uuid.New())
	permission, _ := authz.NewPermission("deployment_plan.manage")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), organizationID.String(), grantedScope.Path(), string(permission), "allow"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{
		UserID: userID, Permission: permission, Scope: otherScope,
	})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPlatformPolicyDoesNotExpandUnrelatedPermissions(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	scope := mustProjectScope(t, organizationID, projectID)
	permission, _ := authz.NewPermission("resource.view")
	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{userID.String(), "platform", "/platform", string(permission), "allow"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	allowed, err := engine.Authorize(context.Background(), authz.AuthorizationRequest{
		UserID: userID, Permission: permission, Scope: scope,
	})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestWorkerSensitiveAuthorizationReloadsPolicyRevision(t *testing.T) {
	organizationID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	scope := mustProjectScope(t, organizationID, projectID)
	permission, _ := authz.NewPermission("resource.view")
	source := &memoryPolicySource{}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)
	request := authz.AuthorizationRequest{UserID: userID, Permission: permission, Scope: scope}
	allowed, err := engine.Authorize(context.Background(), request)
	require.NoError(t, err)
	require.False(t, allowed)

	source.policies = [][]string{{userID.String(), organizationID.String(), scope.Path(), string(permission), "allow"}}
	source.revision = 2
	allowed, err = engine.AuthorizeFresh(context.Background(), request)
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, uint64(2), engine.Revision())
}

type memoryPolicySource struct {
	policies [][]string
	revision uint64
}

func (s *memoryPolicySource) LoadPolicy(target model.Model) error {
	return s.load(target)
}
func (s *memoryPolicySource) LoadPolicyCtx(_ context.Context, target model.Model) error {
	return s.load(target)
}
func (s *memoryPolicySource) load(target model.Model) error {
	for _, policy := range s.policies {
		if err := persist.LoadPolicyArray(append([]string{"p"}, policy...), target); err != nil {
			return err
		}
	}
	return nil
}
func (s *memoryPolicySource) CurrentRevision(context.Context) (uint64, error) {
	return s.revision, nil
}
func (*memoryPolicySource) SavePolicy(model.Model) error { return errors.New("read only") }
func (*memoryPolicySource) AddPolicy(string, string, []string) error {
	return errors.New("read only")
}
func (*memoryPolicySource) RemovePolicy(string, string, []string) error {
	return errors.New("read only")
}
func (*memoryPolicySource) RemoveFilteredPolicy(string, string, int, ...string) error {
	return errors.New("read only")
}
func (*memoryPolicySource) SavePolicyCtx(context.Context, model.Model) error {
	return errors.New("read only")
}
func (*memoryPolicySource) AddPolicyCtx(context.Context, string, string, []string) error {
	return errors.New("read only")
}
func (*memoryPolicySource) RemovePolicyCtx(context.Context, string, string, []string) error {
	return errors.New("read only")
}
func (*memoryPolicySource) RemoveFilteredPolicyCtx(context.Context, string, string, int, ...string) error {
	return errors.New("read only")
}

func mustProjectScope(t *testing.T, organizationID, projectID uuid.UUID) authz.Scope {
	t.Helper()
	scope, err := authz.NewProjectScope(organizationID, projectID)
	require.NoError(t, err)
	return scope
}

func mustEnvironmentScope(t *testing.T, organizationID, projectID, environmentID uuid.UUID) authz.Scope {
	t.Helper()
	scope, err := authz.NewEnvironmentScope(organizationID, projectID, environmentID)
	require.NoError(t, err)
	return scope
}

func mustApplicationScope(t *testing.T, organizationID, projectID, environmentID, applicationID uuid.UUID) authz.Scope {
	t.Helper()
	scope, err := authz.NewApplicationScope(organizationID, projectID, environmentID, applicationID)
	require.NoError(t, err)
	return scope
}
