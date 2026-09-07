package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestNotificationRoutesUseInjectedService(t *testing.T) {
	service := &fakeNotificationService{page: notificationPageFixture()}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Notifications = service
	router := newTestHTTPRouter(t, options)

	response := performNotificationRequest(router, http.MethodGet, "/api/v1/notifications?limit=1", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"eventType":"platform.configuration_drift"`) {
		t.Fatalf("notification list response = %d %s", response.Code, response.Body.String())
	}
	notificationID := service.page.Items[0].Notification.ID
	response = performNotificationRequest(router, http.MethodPost, "/api/v1/notifications/"+notificationID.String()+"/read", "csrf-token")
	if response.Code != http.StatusNoContent || service.markedID != notificationID {
		t.Fatalf("notification read response = %d, marked %s", response.Code, service.markedID)
	}
}

func TestNotificationSSEContainsOnlyEventIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	event := deploydomain.NotificationEvent{EventID: uuid.New(), EventType: "deployment.request.created"}
	service := &fakeNotificationService{events: []deploydomain.NotificationEvent{event}, afterEvents: cancel}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Notifications = service
	router := newTestHTTPRouter(t, options)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/events", nil).WithContext(ctx)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	lastEventID := uuid.New()
	request.Header.Set("Last-Event-ID", lastEventID.String())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(response.Body.String(), event.EventID.String()) {
		t.Fatalf("notification SSE response = %s, %s", response.Header().Get("Content-Type"), response.Body.String())
	}
	if strings.Contains(response.Body.String(), "projectId") {
		t.Fatalf("notification SSE leaked resource data: %s", response.Body.String())
	}
	if service.lastEventID != lastEventID {
		t.Fatalf("Last-Event-ID = %s, want %s", service.lastEventID, lastEventID)
	}
}

type fakeNotificationService struct {
	page        deployapp.NotificationPage
	events      []deploydomain.NotificationEvent
	markedID    uuid.UUID
	lastEventID uuid.UUID
	afterEvents func()
}

func (s *fakeNotificationService) List(context.Context, deployapp.RequestPrincipal, deployapp.NotificationQuery) (deployapp.NotificationPage, error) {
	return s.page, nil
}

func (s *fakeNotificationService) MarkRead(_ context.Context, _ deployapp.RequestPrincipal, id uuid.UUID) error {
	s.markedID = id
	return nil
}

func (*fakeNotificationService) MarkAllRead(context.Context, deployapp.RequestPrincipal) error {
	return nil
}

func (s *fakeNotificationService) EventsAfter(_ context.Context, _ deployapp.RequestPrincipal, lastEventID uuid.UUID, _ int) ([]deploydomain.NotificationEvent, error) {
	s.lastEventID = lastEventID
	if s.afterEvents != nil {
		s.afterEvents()
		s.afterEvents = nil
	}
	return s.events, nil
}

func notificationPageFixture() deployapp.NotificationPage {
	now := time.Now().UTC()
	value := deploydomain.Notification{ID: uuid.New(), RecipientID: uuid.New(), OrganizationID: uuid.New(),
		ProjectID: uuid.New(), EnvironmentID: uuid.New(), EventType: "argocd.application.onboarding_state_changed",
		ResourceType: "application_onboarding", ResourceID: uuid.New(), EventID: uuid.New(),
		OccurredAt: now, ExpiresAt: now.Add(time.Hour)}
	return deployapp.NotificationPage{Items: []deployapp.NotificationView{{Notification: value, Restricted: true}}}
}

func performNotificationRequest(router http.Handler, method, path, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	if csrf != "" {
		request.Header.Set("Origin", "https://releasehub.example")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
