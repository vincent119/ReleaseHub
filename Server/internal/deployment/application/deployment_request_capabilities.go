package application

import (
	"context"
	"fmt"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var deploymentRequestCapabilityKeys = []string{
	"deployment_request.update",
	"deployment_request.review",
	"deployment_request.reassign",
	"deployment_request.deploy",
	"deployment_request.retry",
	"deployment_request.terminate",
	"deployment_request.unlock",
	"deployment_history.view",
}

func (s *DeploymentRequestService) withCapabilities(ctx context.Context, principal RequestPrincipal, detail deploydomain.DeploymentRequestDetail) (deploydomain.DeploymentRequestDetail, error) {
	scope := requestScope(detail.Summary)
	for _, key := range deploymentRequestCapabilityKeys {
		allowed, err := s.allowsCapability(ctx, principal, scope, key)
		if err != nil {
			return deploydomain.DeploymentRequestDetail{}, err
		}
		if allowed {
			detail.Capabilities = append(detail.Capabilities, key)
		}
	}
	return detail, nil
}

func (s *DeploymentRequestService) allowsCapability(ctx context.Context, principal RequestPrincipal, scope authz.Scope, key string) (bool, error) {
	permission, err := authz.NewPermission(key)
	if err != nil {
		return false, err
	}
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return false, fmt.Errorf("authorize deployment request capability: %w", err)
	}
	return allowed, nil
}
