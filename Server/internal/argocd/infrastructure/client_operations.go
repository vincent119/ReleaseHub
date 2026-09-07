package infrastructure

import (
	"context"
	"fmt"
	"slices"

	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

// GetApplicationImages reads image references reported by Argo CD's live resource tree.
func (c *Client) GetApplicationImages(ctx context.Context, identity argodomain.ApplicationIdentity, project string) ([]string, error) {
	if c.manager == nil {
		return nil, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	query := &applicationpkg.ResourcesQuery{ApplicationName: &identity.Name, AppNamespace: &identity.Namespace, Project: &project}
	tree, err := c.manager.ResourceTree(requestCtx, query)
	if err != nil {
		return nil, fmt.Errorf("get Argo CD Application resource tree: %w", err)
	}
	images := make([]string, 0)
	for _, node := range tree.Nodes {
		images = append(images, node.Images...)
	}
	slices.Sort(images)
	return slices.Compact(images), nil
}

// TerminateApplication stops the currently running Argo CD operation.
func (c *Client) TerminateApplication(ctx context.Context, identity argodomain.ApplicationIdentity, project string) error {
	if c.manager == nil {
		return fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	_, err := c.manager.TerminateOperation(requestCtx, &applicationpkg.OperationTerminateRequest{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project,
	})
	if err != nil {
		return fmt.Errorf("terminate Argo CD Application operation: %w", err)
	}
	return nil
}
