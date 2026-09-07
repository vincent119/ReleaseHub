package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	ErrNotificationNotFound  = errors.New("notification was not found")
	ErrNotificationForbidden = errors.New("notification operation is not allowed")
	ErrNotificationInvalid   = errors.New("notification request is invalid")
)

// NotificationQuery selects one recipient-owned cursor page.
type NotificationQuery struct {
	Cursor string
	Limit  int
}

// NotificationView is the current-permission projection returned to delivery.
type NotificationView struct {
	Notification deploydomain.Notification
	Restricted   bool
}

// NotificationPage is one stable cursor page.
type NotificationPage struct {
	Items      []NotificationView
	NextCursor string
	HasMore    bool
}

// NotificationRepository is the consumer-owned notification persistence port.
type NotificationRepository interface {
	List(context.Context, uuid.UUID, string, int) ([]deploydomain.Notification, string, bool, error)
	Load(context.Context, uuid.UUID, uuid.UUID) (deploydomain.Notification, error)
	ListUnread(context.Context, uuid.UUID) ([]deploydomain.Notification, error)
	MarkRead(context.Context, uuid.UUID, uuid.UUID) error
	MarkManyRead(context.Context, uuid.UUID, []uuid.UUID) error
	EventsAfter(context.Context, uuid.UUID, uuid.UUID, int) ([]deploydomain.NotificationEvent, error)
}

// NotificationAuthorizer refreshes policy once and evaluates each projected item.
type NotificationAuthorizer interface {
	ReloadIfStale(context.Context) error
	Authorize(context.Context, authz.AuthorizationRequest) (bool, error)
}

// NotificationService reads and acknowledges notifications using current policy.
type NotificationService struct {
	repository NotificationRepository
	authorizer NotificationAuthorizer
	view       authz.Permission
	markRead   authz.Permission
}

// NewNotificationService creates the notification use case.
func NewNotificationService(repository NotificationRepository, authorizer NotificationAuthorizer) (*NotificationService, error) {
	if repository == nil || authorizer == nil {
		return nil, errors.New("notification dependencies are required")
	}
	view, _ := authz.NewPermission("notification.view")
	markRead, _ := authz.NewPermission("notification.mark_read")
	return &NotificationService{repository: repository, authorizer: authorizer, view: view, markRead: markRead}, nil
}

// List returns notifications with sensitive scope fields projected at read time.
func (s *NotificationService) List(ctx context.Context, principal RequestPrincipal, query NotificationQuery) (NotificationPage, error) {
	if err := validateNotificationQuery(principal, query); err != nil {
		return NotificationPage{}, err
	}
	if err := s.refreshPolicy(ctx); err != nil {
		return NotificationPage{}, err
	}
	items, cursor, more, err := s.repository.List(ctx, principal.UserID, strings.TrimSpace(query.Cursor), query.Limit)
	if err != nil {
		return NotificationPage{}, err
	}
	views, err := s.project(ctx, principal, items)
	return NotificationPage{Items: views, NextCursor: cursor, HasMore: more}, err
}

// MarkRead acknowledges one owned notification after current authorization.
func (s *NotificationService) MarkRead(ctx context.Context, principal RequestPrincipal, notificationID uuid.UUID) error {
	if principal.UserID == uuid.Nil || notificationID == uuid.Nil {
		return ErrNotificationInvalid
	}
	value, err := s.repository.Load(ctx, principal.UserID, notificationID)
	if err != nil {
		return err
	}
	if err := s.refreshPolicy(ctx); err != nil {
		return err
	}
	if err := s.require(ctx, principal, value, s.markRead); err != nil {
		return err
	}
	return s.repository.MarkRead(ctx, principal.UserID, notificationID)
}

// MarkAllRead acknowledges every currently authorized unread notification.
func (s *NotificationService) MarkAllRead(ctx context.Context, principal RequestPrincipal) error {
	if principal.UserID == uuid.Nil {
		return ErrNotificationInvalid
	}
	items, err := s.repository.ListUnread(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if err := s.refreshPolicy(ctx); err != nil {
		return err
	}
	ids, err := s.authorizedIDs(ctx, principal, items)
	if err != nil || len(ids) == 0 {
		return err
	}
	return s.repository.MarkManyRead(ctx, principal.UserID, ids)
}

// EventsAfter returns non-sensitive durable signals for SSE replay.
func (s *NotificationService) EventsAfter(ctx context.Context, principal RequestPrincipal, lastEventID uuid.UUID, limit int) ([]deploydomain.NotificationEvent, error) {
	if principal.UserID == uuid.Nil || principal.Disabled || limit < 1 || limit > 100 {
		return nil, ErrNotificationInvalid
	}
	return s.repository.EventsAfter(ctx, principal.UserID, lastEventID, limit)
}

func validateNotificationQuery(principal RequestPrincipal, query NotificationQuery) error {
	if principal.UserID == uuid.Nil || principal.Disabled || query.Limit < 1 || query.Limit > 100 {
		return ErrNotificationInvalid
	}
	return nil
}

func (s *NotificationService) project(ctx context.Context, principal RequestPrincipal, items []deploydomain.Notification) ([]NotificationView, error) {
	views := make([]NotificationView, 0, len(items))
	for _, item := range items {
		allowed, err := s.allowed(ctx, principal, item, s.view)
		if err != nil {
			return nil, err
		}
		views = append(views, NotificationView{Notification: item, Restricted: !allowed})
	}
	return views, nil
}

func (s *NotificationService) authorizedIDs(ctx context.Context, principal RequestPrincipal, items []deploydomain.Notification) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		allowed, err := s.allowed(ctx, principal, item, s.markRead)
		if err != nil {
			return nil, err
		}
		if allowed {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func (s *NotificationService) require(ctx context.Context, principal RequestPrincipal, item deploydomain.Notification, permission authz.Permission) error {
	allowed, err := s.allowed(ctx, principal, item, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrNotificationForbidden
	}
	return nil
}

func (s *NotificationService) allowed(ctx context.Context, principal RequestPrincipal, item deploydomain.Notification, permission authz.Permission) (bool, error) {
	scope, err := notificationScope(item)
	if err != nil {
		return false, err
	}
	allowed, err := s.authorizer.Authorize(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return false, fmt.Errorf("authorize notification: %w", err)
	}
	return allowed, nil
}

func (s *NotificationService) refreshPolicy(ctx context.Context) error {
	if err := s.authorizer.ReloadIfStale(ctx); err != nil {
		return fmt.Errorf("refresh notification policy: %w", err)
	}
	return nil
}

func notificationScope(value deploydomain.Notification) (authz.Scope, error) {
	if value.ApplicationID != uuid.Nil {
		return authz.NewApplicationScope(value.OrganizationID, value.ProjectID, value.EnvironmentID, value.ApplicationID)
	}
	return authz.NewEnvironmentScope(value.OrganizationID, value.ProjectID, value.EnvironmentID)
}
