package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
)

func TestAuthorizationCapabilityMatrix(t *testing.T) {
	organizationID, grantedProjectID := uuid.New(), uuid.New()
	localUserID, oidcUserID := uuid.New(), uuid.New()
	grantedScope := mustProjectScope(t, organizationID, grantedProjectID)
	otherScope := mustProjectScope(t, organizationID, uuid.New())
	permission, err := authz.NewPermission("deployment_request.view")
	require.NoError(t, err)

	source := &memoryPolicySource{revision: 1, policies: [][]string{
		{localUserID.String(), organizationID.String(), grantedScope.Path(), string(permission), "allow"},
		{oidcUserID.String(), organizationID.String(), grantedScope.Path(), string(permission), "allow"},
		{localUserID.String(), organizationID.String(), otherScope.Path(), string(permission), "deny"},
	}}
	engine, err := authzinfra.NewPolicyEngine(source)
	require.NoError(t, err)

	tests := []struct {
		name     string
		userID   uuid.UUID
		disabled bool
		scope    authz.Scope
		allowed  bool
	}{
		{name: "AUTHZ-IDENTITY-RESOURCE-PARITY/local", userID: localUserID, scope: grantedScope, allowed: true},
		{name: "AUTHZ-IDENTITY-RESOURCE-PARITY/oidc", userID: oidcUserID, scope: grantedScope, allowed: true},
		{name: "AUTHZ-POLICY-DISABLED-DENY", userID: localUserID, disabled: true, scope: grantedScope},
		{name: "AUTHZ-POLICY-EXPLICIT-DENY", userID: localUserID, scope: otherScope},
		{name: "AUTHZ-POLICY-DEFAULT-DENY", userID: uuid.New(), scope: grantedScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, authorizeErr := engine.Authorize(context.Background(), authz.AuthorizationRequest{
				UserID: test.userID, Disabled: test.disabled, Permission: permission, Scope: test.scope,
			})
			require.NoError(t, authorizeErr)
			require.Equal(t, test.allowed, allowed)
		})
	}
}
