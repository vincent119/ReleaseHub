package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

const (
	notificationSSEPollInterval = time.Second
	notificationSSEHeartbeat    = 15 * time.Second
)

type deploymentNotificationService interface {
	List(context.Context, deployapp.RequestPrincipal, deployapp.NotificationQuery) (deployapp.NotificationPage, error)
	MarkRead(context.Context, deployapp.RequestPrincipal, uuid.UUID) error
	MarkAllRead(context.Context, deployapp.RequestPrincipal) error
	EventsAfter(context.Context, deployapp.RequestPrincipal, uuid.UUID, int) ([]deploydomain.NotificationEvent, error)
}

// ListNotifications returns retained recipient notifications.
func (h *deploymentHandler) ListNotifications(c *gin.Context, params contract.ListNotificationsParams) {
	principal, ok := h.notificationPrincipal(c, false, "")
	if !ok {
		return
	}
	page, err := h.notifications.List(c.Request.Context(), principal, notificationQuery(params))
	if !respondNotificationError(c, err) {
		return
	}
	meta := responseMeta(c)
	c.JSON(http.StatusOK, contract.NotificationListResponse{Data: notificationItems(page.Items),
		Meta: contract.CursorPageMeta{RequestId: meta.RequestId, Timestamp: meta.Timestamp,
			HasMore: page.HasMore, NextCursor: optionalCursor(page.NextCursor)}})
}

// MarkNotificationRead acknowledges one notification owned by the caller.
func (h *deploymentHandler) MarkNotificationRead(c *gin.Context, notificationID contract.NotificationId, params contract.MarkNotificationReadParams) {
	principal, ok := h.notificationPrincipal(c, true, string(params.XCSRFToken))
	if ok && respondNotificationError(c, h.notifications.MarkRead(c.Request.Context(), principal, notificationID)) {
		c.Status(http.StatusNoContent)
	}
}

// MarkAllNotificationsRead acknowledges currently authorized notifications.
func (h *deploymentHandler) MarkAllNotificationsRead(c *gin.Context, params contract.MarkAllNotificationsReadParams) {
	principal, ok := h.notificationPrincipal(c, true, string(params.XCSRFToken))
	if ok && respondNotificationError(c, h.notifications.MarkAllRead(c.Request.Context(), principal)) {
		c.Status(http.StatusNoContent)
	}
}

// StreamNotificationEvents streams non-sensitive event identifiers with replay.
func (h *deploymentHandler) StreamNotificationEvents(c *gin.Context, params contract.StreamNotificationEventsParams) {
	principal, ok := h.notificationPrincipal(c, false, "")
	if !ok {
		return
	}
	prepareNotificationStream(c)
	h.streamNotificationEvents(c, principal, notificationLastEventID(params))
}

func (h *deploymentHandler) notificationPrincipal(c *gin.Context, mutation bool, csrf string) (deployapp.RequestPrincipal, bool) {
	if h.notifications == nil {
		h.requireDeploymentRead(c)
		return deployapp.RequestPrincipal{}, false
	}
	if mutation {
		_, user, ok := h.authn.authenticateMutation(c, csrf)
		return deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
	}
	_, user, ok := h.authn.authenticate(c)
	return deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
}

func notificationQuery(params contract.ListNotificationsParams) deployapp.NotificationQuery {
	query := deployapp.NotificationQuery{Limit: 20}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
	}
	if params.Limit != nil {
		query.Limit = int(*params.Limit)
	}
	return query
}

func notificationItems(values []deployapp.NotificationView) []contract.Notification {
	result := make([]contract.Notification, 0, len(values))
	for _, value := range values {
		result = append(result, notificationItem(value))
	}
	return result
}

func notificationItem(view deployapp.NotificationView) contract.Notification {
	value := view.Notification
	if view.Restricted {
		return contract.Notification{Id: value.ID, EventType: restrictedNotificationType(value.EventType),
			ResourceType: "platform_event", ResourceId: value.ID, OccurredAt: value.OccurredAt,
			Read: value.Read, Restricted: true}
	}
	return contract.Notification{Id: value.ID, EventType: value.EventType, ResourceType: value.ResourceType,
		ResourceId: value.ResourceID, OccurredAt: value.OccurredAt, Read: value.Read, Restricted: false,
		OrganizationId: uuidPointer(value.OrganizationID), ProjectId: uuidPointer(value.ProjectID),
		EnvironmentId: uuidPointer(value.EnvironmentID), ApplicationId: uuidPointer(value.ApplicationID)}
}

func restrictedNotificationType(eventType string) string {
	if eventType == "argocd.application.onboarding_state_changed" {
		return "platform.configuration_drift"
	}
	return "deployment.notification.restricted"
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func respondNotificationError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrNotificationInvalid) {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Notification request is invalid")
	} else if errors.Is(err, deployapp.ErrNotificationNotFound) || errors.Is(err, deployapp.ErrNotificationForbidden) {
		respondError(c, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "Notification was not found")
	} else {
		respondError(c, http.StatusInternalServerError, "NOTIFICATION_FAILED", "Unable to process notification")
	}
	return false
}

func notificationLastEventID(params contract.StreamNotificationEventsParams) uuid.UUID {
	if params.LastEventID == nil {
		return uuid.Nil
	}
	return *params.LastEventID
}

func prepareNotificationStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Status(http.StatusOK)
	c.Writer.Flush()
}

func (h *deploymentHandler) streamNotificationEvents(c *gin.Context, principal deployapp.RequestPrincipal, last uuid.UUID) {
	poll := time.NewTicker(notificationSSEPollInterval)
	heartbeat := time.NewTicker(notificationSSEHeartbeat)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-poll.C:
			var err error
			if last, err = h.writeNotificationEvents(c, principal, last); err != nil {
				return
			}
		case <-heartbeat.C:
			_, _ = c.Writer.WriteString(": keepalive\n\n")
			c.Writer.Flush()
		}
	}
}

func (h *deploymentHandler) writeNotificationEvents(c *gin.Context, principal deployapp.RequestPrincipal, last uuid.UUID) (uuid.UUID, error) {
	events, err := h.notifications.EventsAfter(c.Request.Context(), principal, last, 100)
	if err != nil {
		return last, err
	}
	for _, event := range events {
		if writeNotificationEvent(c, event) {
			last = event.EventID
		}
	}
	return last, nil
}

func writeNotificationEvent(c *gin.Context, event deploydomain.NotificationEvent) bool {
	payload, _ := json.Marshal(map[string]string{"eventId": event.EventID.String(), "type": event.EventType})
	_, err := fmt.Fprintf(c.Writer, "id: %s\nevent: %s\ndata: %s\n\n", event.EventID, event.EventType, payload)
	if err == nil {
		c.Writer.Flush()
	}
	return err == nil
}
