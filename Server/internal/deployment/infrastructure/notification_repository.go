package infrastructure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// NotificationRepository stores recipient projections and read receipts.
type NotificationRepository struct{ db *gorm.DB }

type notificationCursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	ID         uuid.UUID `json:"id"`
}

var _ deployapp.NotificationRepository = (*NotificationRepository)(nil)

// NewNotificationRepository creates the PostgreSQL notification repository.
func NewNotificationRepository(db *gorm.DB) (*NotificationRepository, error) {
	if db == nil {
		return nil, errors.New("notification database is required")
	}
	return &NotificationRepository{db: db}, nil
}

// List returns one recipient-owned keyset page inside the retention window.
func (r *NotificationRepository) List(ctx context.Context, recipientID uuid.UUID, cursor string, limit int) ([]deploydomain.Notification, string, bool, error) {
	position, err := decodeNotificationCursor(cursor)
	if err != nil {
		return nil, "", false, deployapp.ErrNotificationInvalid
	}
	rows, err := r.listRows(ctx, recipientID, position, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	return notificationPage(rows, limit)
}

// Load returns one unexpired notification owned by the recipient.
func (r *NotificationRepository) Load(ctx context.Context, recipientID, notificationID uuid.UUID) (deploydomain.Notification, error) {
	var row notificationRow
	result := r.listQuery(ctx, recipientID).Where("notification.id = ?", notificationID).Limit(1).Scan(&row)
	if result.Error != nil {
		return deploydomain.Notification{}, fmt.Errorf("load notification: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return deploydomain.Notification{}, deployapp.ErrNotificationNotFound
	}
	return notificationFromRow(row), nil
}

// ListUnread returns retained unread notifications for authorization filtering.
func (r *NotificationRepository) ListUnread(ctx context.Context, recipientID uuid.UUID) ([]deploydomain.Notification, error) {
	var rows []notificationRow
	err := r.listQuery(ctx, recipientID).Where("receipt.notification_id IS NULL").Order("notification.occurred_at, notification.id").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list unread notifications: %w", err)
	}
	return notificationsFromRows(rows), nil
}

// MarkRead appends one idempotent read receipt.
func (r *NotificationRepository) MarkRead(ctx context.Context, recipientID, notificationID uuid.UUID) error {
	return r.MarkManyRead(ctx, recipientID, []uuid.UUID{notificationID})
}

// MarkManyRead appends read receipts only for notifications owned by the user.
func (r *NotificationRepository) MarkManyRead(ctx context.Context, recipientID uuid.UUID, notificationIDs []uuid.UUID) error {
	if len(notificationIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return insertNotificationReads(ctx, tx, recipientID, notificationIDs)
	})
}

// EventsAfter returns durable non-sensitive signals for SSE replay.
func (r *NotificationRepository) EventsAfter(ctx context.Context, recipientID, lastEventID uuid.UUID, limit int) ([]deploydomain.NotificationEvent, error) {
	rows, err := r.eventRows(ctx, recipientID, lastEventID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]deploydomain.NotificationEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, deploydomain.NotificationEvent{EventID: row.EventID, EventType: row.EventType})
	}
	return result, nil
}

func (r *NotificationRepository) listRows(ctx context.Context, recipientID uuid.UUID, cursor *notificationCursor, limit int) ([]notificationRow, error) {
	query := r.listQuery(ctx, recipientID)
	if cursor != nil {
		query = query.Where("(notification.occurred_at, notification.id) < (?, ?)", cursor.OccurredAt, cursor.ID)
	}
	var rows []notificationRow
	err := query.Order("notification.occurred_at DESC, notification.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return rows, nil
}

func (r *NotificationRepository) listQuery(ctx context.Context, recipientID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).Table("deployment_notifications AS notification").
		Select("notification.*, receipt.notification_id IS NOT NULL AS read").
		Joins("LEFT JOIN deployment_notification_reads receipt ON receipt.notification_id = notification.id AND receipt.user_id = ?", recipientID).
		Where("notification.recipient_id = ? AND notification.expires_at > ?", recipientID, time.Now().UTC())
}

func notificationPage(rows []notificationRow, limit int) ([]deploydomain.Notification, string, bool, error) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	next := ""
	if hasMore {
		next = encodeNotificationCursor(rows[len(rows)-1])
	}
	return notificationsFromRows(rows), next, hasMore, nil
}

func notificationsFromRows(rows []notificationRow) []deploydomain.Notification {
	result := make([]deploydomain.Notification, 0, len(rows))
	for _, row := range rows {
		result = append(result, notificationFromRow(row))
	}
	return result
}

func notificationFromRow(row notificationRow) deploydomain.Notification {
	result := deploydomain.Notification{ID: row.ID, RecipientID: row.RecipientID,
		OrganizationID: row.OrganizationID, ProjectID: row.ProjectID, EnvironmentID: row.EnvironmentID,
		EventType: row.EventType, ResourceType: row.ResourceType, ResourceID: row.ResourceID,
		EventID: row.EventID, OccurredAt: row.OccurredAt, ExpiresAt: row.ExpiresAt, Read: row.Read}
	if row.ApplicationID != nil {
		result.ApplicationID = *row.ApplicationID
	}
	return result
}

func insertNotificationReads(ctx context.Context, tx *gorm.DB, recipientID uuid.UUID, ids []uuid.UUID) error {
	var owned []uuid.UUID
	if err := tx.WithContext(ctx).Table("deployment_notifications").Where("recipient_id = ? AND id IN ? AND expires_at > ?", recipientID, ids, time.Now().UTC()).Pluck("id", &owned).Error; err != nil {
		return fmt.Errorf("resolve owned notifications: %w", err)
	}
	reads := make([]deploymentNotificationReadModel, 0, len(owned))
	for _, id := range owned {
		reads = append(reads, deploymentNotificationReadModel{NotificationID: id, UserID: recipientID, ReadAt: time.Now().UTC()})
	}
	if len(reads) == 0 {
		return deployapp.ErrNotificationNotFound
	}
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&reads).Error
	if err != nil {
		return fmt.Errorf("mark notifications read: %w", err)
	}
	return nil
}

func encodeNotificationCursor(row notificationRow) string {
	value, _ := json.Marshal(notificationCursor{OccurredAt: row.OccurredAt.UTC(), ID: row.ID})
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeNotificationCursor(value string) (*notificationCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor notificationCursor
	err = json.Unmarshal(decoded, &cursor)
	if err != nil || cursor.OccurredAt.IsZero() || cursor.ID == uuid.Nil {
		return nil, errors.New("invalid notification cursor")
	}
	return &cursor, nil
}
