package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestNotificationRechecksCurrentPermission(t *testing.T) {
	value := notificationFixture()
	repository := &notificationRepositoryStub{items: []deploydomain.Notification{value}}
	authorizer := &notificationAuthorizerStub{allowed: false}
	service, err := NewNotificationService(repository, authorizer)
	require.NoError(t, err)
	page, err := service.List(context.Background(), RequestPrincipal{UserID: value.RecipientID}, NotificationQuery{Limit: 20})
	require.NoError(t, err)
	require.True(t, page.Items[0].Restricted)
	require.Equal(t, authz.Permission("notification.view"), authorizer.permission)
}

func TestNotificationMarkReadRequiresCurrentPermission(t *testing.T) {
	value := notificationFixture()
	repository := &notificationRepositoryStub{loaded: value}
	service, err := NewNotificationService(repository, &notificationAuthorizerStub{allowed: false})
	require.NoError(t, err)
	err = service.MarkRead(context.Background(), RequestPrincipal{UserID: value.RecipientID}, value.ID)
	require.ErrorIs(t, err, ErrNotificationForbidden)
	require.False(t, repository.marked)
}

type notificationAuthorizerStub struct {
	allowed    bool
	permission authz.Permission
}

func (*notificationAuthorizerStub) ReloadIfStale(context.Context) error { return nil }

func (s *notificationAuthorizerStub) Authorize(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	s.permission = request.Permission
	return s.allowed, nil
}

type notificationRepositoryStub struct {
	items  []deploydomain.Notification
	loaded deploydomain.Notification
	marked bool
}

func (s *notificationRepositoryStub) List(context.Context, uuid.UUID, string, int) ([]deploydomain.Notification, string, bool, error) {
	return s.items, "", false, nil
}

func (s *notificationRepositoryStub) Load(context.Context, uuid.UUID, uuid.UUID) (deploydomain.Notification, error) {
	return s.loaded, nil
}

func (s *notificationRepositoryStub) ListUnread(context.Context, uuid.UUID) ([]deploydomain.Notification, error) {
	return s.items, nil
}

func (s *notificationRepositoryStub) MarkRead(context.Context, uuid.UUID, uuid.UUID) error {
	s.marked = true
	return nil
}

func (s *notificationRepositoryStub) MarkManyRead(context.Context, uuid.UUID, []uuid.UUID) error {
	s.marked = true
	return nil
}

func (s *notificationRepositoryStub) EventsAfter(context.Context, uuid.UUID, uuid.UUID, int) ([]deploydomain.NotificationEvent, error) {
	return nil, nil
}

func notificationFixture() deploydomain.Notification {
	now := time.Now().UTC()
	return deploydomain.Notification{
		ID: uuid.New(), RecipientID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(),
		EnvironmentID: uuid.New(), EventType: "deployment.request.created", ResourceType: "deployment_request",
		ResourceID: uuid.New(), EventID: uuid.New(), OccurredAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
}
