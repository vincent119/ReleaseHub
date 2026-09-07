package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
)

func TestOIDCGroupServiceNormalizesProviderGroupsWithoutGrantingUnknownValues(t *testing.T) {
	repository := &recordingOIDCGroupRepository{}
	service, err := application.NewOIDCGroupService(repository)
	require.NoError(t, err)
	userID := uuid.New()
	require.NoError(t, service.SyncOIDCGroups(context.Background(), userID, " https://issuer.example ", []string{"viewer", "unknown", "viewer", " "}))
	require.Equal(t, userID, repository.userID)
	require.Equal(t, "https://issuer.example", repository.issuer)
	require.Equal(t, []string{"unknown", "viewer"}, repository.groups)
}

type recordingOIDCGroupRepository struct {
	userID uuid.UUID
	issuer string
	groups []string
}

func (r *recordingOIDCGroupRepository) SyncOIDCGroups(_ context.Context, userID uuid.UUID, issuer string, groups []string) error {
	r.userID, r.issuer, r.groups = userID, issuer, append([]string(nil), groups...)
	return nil
}
