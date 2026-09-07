package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NotificationProjector consumes deployment Outbox events into recipient rows.
type NotificationProjector struct{ db *gorm.DB }

type notificationOutboxRow struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       datatypes.JSON
	OccurredAt    time.Time
}

type notificationScopeRow struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	ApplicationID  *uuid.UUID
}

type notificationProjection struct {
	ctx       context.Context
	tx        *gorm.DB
	event     notificationOutboxRow
	now       time.Time
	retention time.Duration
}

var notificationEventTypes = []string{
	"deployment.request.created", "deployment.request.version.created",
	"deployment.request.forward_rollback_classified", "deployment.review.decided",
	"deployment.review.reassigned", "deployment.workflow.advanced",
	"deployment.workflow.result_blocked", "deployment.execution.completed",
	"deployment.execution.blocked",
	"deployment.execution.retry", "deployment.execution.terminate", "deployment.execution.unlock",
	"argocd.application.onboarding_state_changed",
}

// NewNotificationProjector creates the transactional Outbox projector.
func NewNotificationProjector(db *gorm.DB) (*NotificationProjector, error) {
	if db == nil {
		return nil, errors.New("notification projector database is required")
	}
	return &NotificationProjector{db: db}, nil
}

// ProjectNext claims and projects one supported Outbox event atomically.
func (p *NotificationProjector) ProjectNext(ctx context.Context, now time.Time, retention time.Duration) (bool, error) {
	projected := false
	var eventID uuid.UUID
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		event, found, err := claimNotificationEvent(ctx, tx)
		if err != nil || !found {
			return err
		}
		eventID = event.ID
		projected = true
		return projectNotificationEvent(notificationProjection{
			ctx: ctx, tx: tx, event: event, now: now, retention: retention,
		})
	})
	if err != nil && eventID != uuid.Nil {
		err = errors.Join(err, p.recordFailure(ctx, eventID, err))
	}
	return projected, err
}

func (p *NotificationProjector) recordFailure(ctx context.Context, eventID uuid.UUID, cause error) error {
	result := p.db.WithContext(ctx).Table("outbox_events").Where("id = ? AND published_at IS NULL", eventID).
		Updates(map[string]any{"retry_count": gorm.Expr("retry_count + 1"), "last_error": cause.Error()})
	if result.Error != nil {
		return fmt.Errorf("record notification projection failure: %w", result.Error)
	}
	return nil
}

// DeleteExpired removes notifications without modifying immutable Audit records.
func (p *NotificationProjector) DeleteExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	result := p.db.WithContext(ctx).Exec(`DELETE FROM deployment_notifications WHERE id IN (
		SELECT id FROM deployment_notifications WHERE expires_at <= ? ORDER BY expires_at LIMIT ?
	)`, now.UTC(), limit)
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired notifications: %w", result.Error)
	}
	return int(result.RowsAffected), nil
}

func claimNotificationEvent(ctx context.Context, tx *gorm.DB) (notificationOutboxRow, bool, error) {
	var event notificationOutboxRow
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Table("outbox_events").Where("published_at IS NULL AND event_type IN ?", notificationEventTypes).
		Order("occurred_at, id").Limit(1).Take(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notificationOutboxRow{}, false, nil
	}
	if err != nil {
		return notificationOutboxRow{}, false, fmt.Errorf("claim notification event: %w", err)
	}
	return event, true, nil
}

func projectNotificationEvent(value notificationProjection) error {
	if !value.event.OccurredAt.Add(value.retention).After(value.now) {
		return markNotificationEventPublished(value.ctx, value.tx, value.event.ID, value.now)
	}
	if !isNotifiableEvent(value.event) {
		return markNotificationEventPublished(value.ctx, value.tx, value.event.ID, value.now)
	}
	scope, err := resolveNotificationScope(value.ctx, value.tx, value.event)
	if err != nil {
		return err
	}
	recipients, err := notificationRecipients(value.ctx, value.tx, scope, isConfigurationDrift(value.event))
	if err != nil {
		return err
	}
	if err := insertProjectedNotifications(value, scope, recipients); err != nil {
		return err
	}
	return markNotificationEventPublished(value.ctx, value.tx, value.event.ID, value.now)
}

func isNotifiableEvent(event notificationOutboxRow) bool {
	return event.EventType != "argocd.application.onboarding_state_changed" || isConfigurationDrift(event)
}

func isConfigurationDrift(event notificationOutboxRow) bool {
	if event.EventType != "argocd.application.onboarding_state_changed" {
		return false
	}
	var payload struct {
		State string `json:"state"`
	}
	return json.Unmarshal(event.Payload, &payload) == nil && payload.State == "ConfigurationDrift"
}

func resolveNotificationScope(ctx context.Context, tx *gorm.DB, event notificationOutboxRow) (notificationScopeRow, error) {
	resourceID, err := uuid.Parse(event.AggregateID)
	if err != nil {
		return notificationScopeRow{}, fmt.Errorf("parse notification resource ID: %w", err)
	}
	scope, err := queryNotificationScope(ctx, tx, event.AggregateType, resourceID)
	if err != nil {
		return notificationScopeRow{}, err
	}
	return scope, nil
}

func insertProjectedNotifications(value notificationProjection, scope notificationScopeRow, recipients []uuid.UUID) error {
	values := make([]deploymentNotificationModel, 0, len(recipients))
	for _, recipientID := range recipients {
		values = append(values, projectedNotification(value.event, scope, recipientID, value.retention))
	}
	if len(values) == 0 {
		return nil
	}
	err := value.tx.WithContext(value.ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&values).Error
	if err != nil {
		return fmt.Errorf("insert projected notifications: %w", err)
	}
	return nil
}

func projectedNotification(event notificationOutboxRow, scope notificationScopeRow, recipientID uuid.UUID, retention time.Duration) deploymentNotificationModel {
	resourceID, _ := uuid.Parse(event.AggregateID)
	return deploymentNotificationModel{
		ID: uuid.New(), RecipientID: recipientID, OrganizationID: scope.OrganizationID,
		ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, ApplicationID: scope.ApplicationID,
		EventType: event.EventType, ResourceType: event.AggregateType, ResourceID: resourceID,
		EventID: event.EventID, OccurredAt: event.OccurredAt.UTC(),
		ExpiresAt: event.OccurredAt.Add(retention).UTC(), CreatedAt: time.Now().UTC(),
	}
}

func markNotificationEventPublished(ctx context.Context, tx *gorm.DB, eventID uuid.UUID, now time.Time) error {
	result := tx.WithContext(ctx).Table("outbox_events").Where("id = ? AND published_at IS NULL", eventID).
		Updates(map[string]any{"published_at": now.UTC(), "last_error": nil})
	if result.Error != nil {
		return fmt.Errorf("mark notification event published: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("notification event was not marked published")
	}
	return nil
}
