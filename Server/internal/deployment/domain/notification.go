package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidNotification = errors.New("invalid deployment notification")

// Notification is one recipient-owned projection of a durable domain event.
type Notification struct {
	ID             uuid.UUID
	RecipientID    uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ApplicationID  uuid.UUID
	EventType      string
	ResourceType   string
	ResourceID     uuid.UUID
	EventID        uuid.UUID
	OccurredAt     time.Time
	ExpiresAt      time.Time
	Read           bool
}

// NotificationEvent is the non-sensitive SSE signal for one notification.
type NotificationEvent struct {
	EventID   uuid.UUID
	EventType string
}

// Valid reports whether the notification has a usable recipient and scope.
func (n Notification) Valid() bool {
	return n.ID != uuid.Nil && n.RecipientID != uuid.Nil && n.OrganizationID != uuid.Nil &&
		n.ProjectID != uuid.Nil && n.EnvironmentID != uuid.Nil && n.ResourceID != uuid.Nil &&
		n.EventID != uuid.Nil && n.EventType != "" && n.ResourceType != "" &&
		!n.OccurredAt.IsZero() && n.ExpiresAt.After(n.OccurredAt)
}
