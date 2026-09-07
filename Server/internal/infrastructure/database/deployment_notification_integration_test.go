//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func TestNotificationProjectionReplayReadAndRetention(t *testing.T) {
	ctx := context.Background()
	db := workflowTestDatabase(t, ctx)
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	requestVersionID, _ := seedDeploymentRequest(t, db, ids)
	requestID := deploymentRequestID(t, db, requestVersionID)
	recipients := seedNotificationRecipients(t, db, ids)
	projector, _ := deployinfra.NewNotificationProjector(db)
	repository, _ := deployinfra.NewNotificationRepository(db)
	firstEvent := appendNotificationEvent(t, db, requestID, "deployment.request.created", time.Now().UTC())

	projected, err := projector.ProjectNext(ctx, time.Now().UTC(), 7*24*time.Hour)
	if err != nil || !projected {
		t.Fatalf("project first notification = %v, %v", projected, err)
	}
	assertCountWhere(t, db, "deployment_notifications", "event_id = ?", firstEvent, int64(len(recipients)))
	verifyNotificationCurrentPermission(t, db, repository, recipients[0])
	verifyNotificationRead(t, repository, recipients[0])
	verifyNotificationReplay(t, db, projector, repository, recipients[0], requestID, firstEvent)
	verifyConfigurationDriftBroadcast(t, db, projector, repository, ids.applicationID)
	verifyNotificationRetention(t, db, projector)
	verifyNotificationProjectionFailure(t, db, projector)
}

func verifyConfigurationDriftBroadcast(t *testing.T, db *gorm.DB, projector *deployinfra.NotificationProjector, repository *deployinfra.NotificationRepository, applicationID uuid.UUID) {
	t.Helper()
	userID := uuid.New()
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'notification-outsider')`, userID)
	payload, _ := json.Marshal(map[string]string{"applicationId": applicationID.String(), "state": "ConfigurationDrift"})
	event, _ := platform.NewEvent("argocd.application.onboarding_state_changed", "application_onboarding",
		applicationID.String(), payload, time.Now().UTC().Add(2*time.Second))
	if err := database.AppendOutbox(context.Background(), db, event); err != nil {
		t.Fatalf("append configuration drift event: %v", err)
	}
	if projected, err := projector.ProjectNext(context.Background(), time.Now().UTC(), 7*24*time.Hour); err != nil || !projected {
		t.Fatalf("project configuration drift = %v, %v", projected, err)
	}
	adapter, _ := authzinfra.NewPolicyAdapter(db)
	engine, _ := authzinfra.NewPolicyEngine(adapter)
	service, _ := deployapp.NewNotificationService(repository, engine)
	page, err := service.List(context.Background(), deployapp.RequestPrincipal{UserID: userID}, deployapp.NotificationQuery{Limit: 20})
	if err != nil || len(page.Items) != 1 || !page.Items[0].Restricted {
		t.Fatalf("restricted drift notification = %#v, %v", page, err)
	}
}

func verifyNotificationProjectionFailure(t *testing.T, db *gorm.DB, projector *deployinfra.NotificationProjector) {
	t.Helper()
	eventID := appendNotificationEvent(t, db, uuid.New(), "deployment.request.created", time.Now().UTC())
	if projected, err := projector.ProjectNext(context.Background(), time.Now().UTC(), 7*24*time.Hour); err == nil || !projected {
		t.Fatalf("invalid notification projection = %v, %v", projected, err)
	}
	var retries int
	if err := db.Table("outbox_events").Where("event_id = ?", eventID).Pluck("retry_count", &retries).Error; err != nil || retries != 1 {
		t.Fatalf("notification projection retry count = %d, %v", retries, err)
	}
}

func verifyNotificationCurrentPermission(t *testing.T, db *gorm.DB, repository *deployinfra.NotificationRepository, userID uuid.UUID) {
	t.Helper()
	adapter, err := authzinfra.NewPolicyAdapter(db)
	if err != nil {
		t.Fatalf("create notification policy adapter: %v", err)
	}
	engine, err := authzinfra.NewPolicyEngine(adapter)
	if err != nil {
		t.Fatalf("create notification policy engine: %v", err)
	}
	service, _ := deployapp.NewNotificationService(repository, engine)
	page, err := service.List(context.Background(), deployapp.RequestPrincipal{UserID: userID}, deployapp.NotificationQuery{Limit: 20})
	if err != nil || page.Items[0].Restricted {
		t.Fatalf("authorized notification projection = %#v, %v", page, err)
	}
	execDeploymentSQL(t, db, `UPDATE authorization_group_role_bindings SET active = false
		WHERE group_id IN (SELECT group_id FROM authorization_group_memberships WHERE user_id = ?)`, userID)
	page, err = service.List(context.Background(), deployapp.RequestPrincipal{UserID: userID}, deployapp.NotificationQuery{Limit: 20})
	if err != nil || !page.Items[0].Restricted {
		t.Fatalf("revoked notification projection = %#v, %v", page, err)
	}
	execDeploymentSQL(t, db, `UPDATE authorization_group_role_bindings SET active = true
		WHERE group_id IN (SELECT group_id FROM authorization_group_memberships WHERE user_id = ?)`, userID)
}

func seedNotificationRecipients(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs) []uuid.UUID {
	t.Helper()
	recipients := []uuid.UUID{ids.userID, uuid.New()}
	execDeploymentSQL(t, db, `INSERT INTO users (id, username) VALUES (?, 'notification-user')`, recipients[1])
	for _, userID := range recipients {
		groupID := uuid.New()
		execDeploymentSQL(t, db, `INSERT INTO authorization_groups (id, owner_kind, owner_id, name)
			VALUES (?, 'project', ?, ?)`, groupID, ids.projectID, "notification-group-"+uuid.NewString())
		execDeploymentSQL(t, db, `INSERT INTO authorization_group_memberships
			(id, group_id, user_id, source) VALUES (?, ?, ?, 'manual')`, uuid.New(), groupID, userID)
		execDeploymentSQL(t, db, `INSERT INTO authorization_group_role_bindings
			(id, group_id, role_id, organization_id, scope_kind, project_id, environment_id)
			VALUES (?, ?, '00000000-0000-0000-0000-000000000105', ?, 'environment', ?, ?)`,
			uuid.New(), groupID, ids.organizationID, ids.projectID, ids.environmentID)
	}
	return recipients
}

func deploymentRequestID(t *testing.T, db *gorm.DB, versionID uuid.UUID) uuid.UUID {
	t.Helper()
	var value struct{ RequestID uuid.UUID }
	if err := db.Table("deployment_request_versions").Select("request_id").Where("id = ?", versionID).Scan(&value).Error; err != nil {
		t.Fatalf("load deployment request ID: %v", err)
	}
	return value.RequestID
}

func appendNotificationEvent(t *testing.T, db *gorm.DB, requestID uuid.UUID, eventType string, occurredAt time.Time) uuid.UUID {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"requestId": requestID})
	event, err := platform.NewEvent(eventType, "deployment_request", requestID.String(), payload, occurredAt)
	if err != nil {
		t.Fatalf("create notification event: %v", err)
	}
	if err := database.AppendOutbox(context.Background(), db, event); err != nil {
		t.Fatalf("append notification event: %v", err)
	}
	return event.ID()
}

func verifyNotificationRead(t *testing.T, repository *deployinfra.NotificationRepository, recipientID uuid.UUID) {
	t.Helper()
	page, _, _, err := repository.List(context.Background(), recipientID, "", 20)
	if err != nil || len(page) != 1 || page[0].Read {
		t.Fatalf("notification page before read = %#v, %v", page, err)
	}
	if err := repository.MarkRead(context.Background(), recipientID, page[0].ID); err != nil {
		t.Fatalf("mark notification read for %#v: %v", page[0], err)
	}
	page, _, _, err = repository.List(context.Background(), recipientID, "", 20)
	if err != nil || !page[0].Read {
		t.Fatalf("notification page after read = %#v, %v", page, err)
	}
}

func verifyNotificationReplay(t *testing.T, db *gorm.DB, projector *deployinfra.NotificationProjector, repository *deployinfra.NotificationRepository, recipientID, requestID, firstEvent uuid.UUID) {
	t.Helper()
	secondEvent := appendNotificationEvent(t, db, requestID, "deployment.request.version.created", time.Now().UTC().Add(time.Second))
	if projected, err := projector.ProjectNext(context.Background(), time.Now().UTC(), 7*24*time.Hour); err != nil || !projected {
		t.Fatalf("project replay notification = %v, %v", projected, err)
	}
	events, err := repository.EventsAfter(context.Background(), recipientID, firstEvent, 20)
	if err != nil || len(events) != 1 || events[0].EventID != secondEvent {
		t.Fatalf("notification replay = %#v, %v", events, err)
	}
}

func verifyNotificationRetention(t *testing.T, db *gorm.DB, projector *deployinfra.NotificationProjector) {
	t.Helper()
	auditBefore := tableCount(t, db, "audit_logs")
	count, err := projector.DeleteExpired(context.Background(), time.Now().UTC().Add(8*24*time.Hour), 500)
	if err != nil || count == 0 {
		t.Fatalf("expire notifications = %d, %v", count, err)
	}
	assertCount(t, db, "deployment_notifications", 0)
	if tableCount(t, db, "audit_logs") != auditBefore {
		t.Fatal("notification retention must not delete Audit records")
	}
}
