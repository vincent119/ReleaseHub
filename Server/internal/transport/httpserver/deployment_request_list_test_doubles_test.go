package httpserver_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type requestListHTTPRepository struct {
	detail deploydomain.DeploymentRequestDetail
	filter deployapp.DeploymentRequestListFilter
	items  []deploydomain.DeploymentRequestSummary
	calls  int
}

func (r *requestListHTTPRepository) List(_ context.Context, _ authz.Scope, filter deployapp.DeploymentRequestListFilter) ([]deploydomain.DeploymentRequestSummary, error) {
	r.calls++
	r.filter = filter
	return r.items, nil
}

func (r *requestListHTTPRepository) Load(context.Context, uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	return r.detail, nil
}

func (r *requestListHTTPRepository) SupersedeMetadata(context.Context, deployapp.RequestMutation, deployapp.MetadataVersionChange) (deploydomain.DeploymentRequestDetail, error) {
	return r.detail, nil
}

type requestListHTTPAuthorizer struct{ allowed bool }

func (a requestListHTTPAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return a.allowed, nil
}

type requestListHTTPScheduleReader struct{}

func (requestListHTTPScheduleReader) Get(context.Context, uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	return nil, nil
}

func newRequestListHTTPService(t *testing.T, detail deploydomain.DeploymentRequestDetail, allowed bool) (*deployapp.DeploymentRequestService, *requestListHTTPRepository) {
	t.Helper()
	repo := &requestListHTTPRepository{detail: detail}
	service, err := deployapp.NewDeploymentRequestService(repo, requestListHTTPScheduleReader{},
		requestListHTTPAuthorizer{allowed: allowed}, deployapp.SystemWorkflowClock{})
	require.NoError(t, err)
	return service, repo
}
