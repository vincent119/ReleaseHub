package infrastructure

import (
	"time"

	"github.com/google/uuid"
)

type deploymentNotificationModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	RecipientID    uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ApplicationID  *uuid.UUID
	EventType      string
	ResourceType   string
	ResourceID     uuid.UUID
	EventID        uuid.UUID
	OccurredAt     time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

func (deploymentNotificationModel) TableName() string { return "deployment_notifications" }

type deploymentNotificationReadModel struct {
	NotificationID uuid.UUID `gorm:"primaryKey"`
	UserID         uuid.UUID `gorm:"primaryKey"`
	ReadAt         time.Time
}

func (deploymentNotificationReadModel) TableName() string { return "deployment_notification_reads" }

type notificationRow struct {
	ID             uuid.UUID
	RecipientID    uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ApplicationID  *uuid.UUID
	EventType      string
	ResourceType   string
	ResourceID     uuid.UUID
	EventID        uuid.UUID
	OccurredAt     time.Time
	ExpiresAt      time.Time
	Read           bool
}

type notificationEventRow struct {
	EventID    uuid.UUID
	EventType  string
	OccurredAt time.Time
}
