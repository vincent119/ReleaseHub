package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *NotificationRepository) eventRows(ctx context.Context, recipientID, lastEventID uuid.UUID, limit int) ([]notificationEventRow, error) {
	anchor, found, err := r.eventAnchor(ctx, recipientID, lastEventID)
	if err != nil {
		return nil, err
	}
	if !found {
		return r.latestEventRows(ctx, recipientID, limit)
	}
	var rows []notificationEventRow
	err = r.db.WithContext(ctx).Table("deployment_notifications").
		Select("event_id, event_type, occurred_at").
		Where("recipient_id = ? AND expires_at > ? AND (occurred_at, id) > (?, ?)",
			recipientID, time.Now().UTC(), anchor.OccurredAt, anchor.ID).
		Order("occurred_at, id").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("replay notification events: %w", err)
	}
	return rows, nil
}

func (r *NotificationRepository) eventAnchor(ctx context.Context, recipientID, eventID uuid.UUID) (notificationCursor, bool, error) {
	if eventID == uuid.Nil {
		return notificationCursor{}, false, nil
	}
	var anchor notificationCursor
	result := r.db.WithContext(ctx).Table("deployment_notifications").
		Select("occurred_at, id").Where("recipient_id = ? AND event_id = ?", recipientID, eventID).Take(&anchor)
	if result.Error == nil {
		return anchor, true, nil
	}
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return notificationCursor{}, false, fmt.Errorf("load notification event anchor: %w", result.Error)
	}
	return notificationCursor{}, false, nil
}

func (r *NotificationRepository) latestEventRows(ctx context.Context, recipientID uuid.UUID, limit int) ([]notificationEventRow, error) {
	var rows []notificationEventRow
	err := r.db.WithContext(ctx).Raw(`SELECT event_id, event_type, occurred_at FROM (
		SELECT id, event_id, event_type, occurred_at FROM deployment_notifications
		WHERE recipient_id = ? AND expires_at > ? ORDER BY occurred_at DESC, id DESC LIMIT ?
	) recent ORDER BY occurred_at, id`, recipientID, time.Now().UTC(), limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list latest notification events: %w", err)
	}
	return rows, nil
}
