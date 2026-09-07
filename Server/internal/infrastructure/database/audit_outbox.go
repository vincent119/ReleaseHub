package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// AuditRecord is the infrastructure input used to persist one immutable audit entry.
type AuditRecord struct {
	OccurredAt     time.Time
	ActorID        *uuid.UUID
	OrganizationID *uuid.UUID
	Action         string
	ResourceType   string
	ResourceID     string
	RequestID      string
	Metadata       map[string]any
}

// AppendAudit writes an audit record into the transaction supplied by the caller.
func AppendAudit(ctx context.Context, tx *gorm.DB, record AuditRecord) error {
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	if err := tx.WithContext(ctx).Create(&auditLogModel{
		OccurredAt: record.OccurredAt.UTC(), ActorID: record.ActorID, OrganizationID: record.OrganizationID,
		Action: record.Action, ResourceType: record.ResourceType, ResourceID: record.ResourceID,
		RequestID: record.RequestID, Metadata: datatypes.JSON(metadata),
	}).Error; err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// AppendOutbox writes an immutable domain event into the caller's transaction.
func AppendOutbox(ctx context.Context, tx *gorm.DB, event domain.Event) error {
	if err := tx.WithContext(ctx).Create(&outboxEventModel{
		EventID: event.ID(), EventType: event.Type(), AggregateType: event.AggregateType(),
		AggregateID: event.AggregateID(), Payload: datatypes.JSON(event.Payload()), OccurredAt: event.OccurredAt(),
	}).Error; err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

type auditLogModel struct {
	ID             uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OccurredAt     time.Time  `gorm:"not null"`
	ActorID        *uuid.UUID `gorm:"type:uuid"`
	OrganizationID *uuid.UUID `gorm:"type:uuid"`
	Action         string     `gorm:"not null"`
	ResourceType   string     `gorm:"not null"`
	ResourceID     string     `gorm:"not null"`
	RequestID      string
	Metadata       datatypes.JSON `gorm:"type:jsonb;not null"`
}

func (auditLogModel) TableName() string { return "audit_logs" }

type outboxEventModel struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	EventID       uuid.UUID      `gorm:"type:uuid;not null"`
	EventType     string         `gorm:"not null"`
	AggregateType string         `gorm:"not null"`
	AggregateID   string         `gorm:"not null"`
	Payload       datatypes.JSON `gorm:"type:jsonb;not null"`
	OccurredAt    time.Time      `gorm:"not null"`
}

func (outboxEventModel) TableName() string { return "outbox_events" }
