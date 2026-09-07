package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/distribution/reference"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type actualStateArgo interface {
	GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error)
	GetApplicationImages(context.Context, argodomain.ApplicationIdentity, string) ([]string, error)
}

// ArgoActualStateReader obtains fresh revision and digest evidence before manual unlock.
type ArgoActualStateReader struct{ argo actualStateArgo }

// NewArgoActualStateReader creates the fail-closed actual-state adapter.
func NewArgoActualStateReader(argo actualStateArgo) (*ArgoActualStateReader, error) {
	if argo == nil {
		return nil, errors.New("Argo CD actual-state client is required")
	}
	return &ArgoActualStateReader{argo: argo}, nil
}

// ReadActualState resolves only digest-bearing live image references.
func (r *ArgoActualStateReader) ReadActualState(ctx context.Context, node deployapp.DeploymentExecutionNode) (deployapp.ActualState, error) {
	application, err := r.argo.GetApplication(ctx, node.Identity, node.ArgoProject)
	if err != nil {
		return deployapp.ActualState{}, fmt.Errorf("read actual Application revision: %w", err)
	}
	values, err := r.argo.GetApplicationImages(ctx, node.Identity, node.ArgoProject)
	if err != nil {
		return deployapp.ActualState{}, err
	}
	images, err := actualImageSnapshots(values)
	if err != nil {
		return deployapp.ActualState{}, err
	}
	return deployapp.ActualState{ApplicationID: node.ApplicationID,
		Revision: application.ResolvedRevision, Images: images}, nil
}

func actualImageSnapshots(values []string) ([]deploydomain.DeploymentRequestImageSnapshot, error) {
	result := make([]deploydomain.DeploymentRequestImageSnapshot, 0, len(values))
	for _, value := range values {
		image, err := actualImageSnapshot(value)
		if err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	slices.SortFunc(result, func(left, right deploydomain.DeploymentRequestImageSnapshot) int {
		return strings.Compare(left.ImageReference, right.ImageReference)
	})
	return result, nil
}

func actualImageSnapshot(value string) (deploydomain.DeploymentRequestImageSnapshot, error) {
	parsed, err := reference.ParseAnyReference(strings.TrimSpace(value))
	if err != nil {
		return deploydomain.DeploymentRequestImageSnapshot{}, fmt.Errorf("parse actual image reference: %w", err)
	}
	named, namedOK := parsed.(reference.Named)
	digested, digestOK := parsed.(reference.Digested)
	if !namedOK || !digestOK || digested.Digest().Algorithm().String() != "sha256" {
		return deploydomain.DeploymentRequestImageSnapshot{}, errors.New("actual image digest is unavailable")
	}
	tag := ""
	if tagged, ok := parsed.(reference.Tagged); ok {
		tag = tagged.Tag()
	}
	return deploydomain.DeploymentRequestImageSnapshot{
		ImageReference: strings.TrimSpace(value), Registry: reference.Domain(named),
		Repository: reference.Path(named), Tag: tag, Digest: digested.Digest().String(),
	}, nil
}
