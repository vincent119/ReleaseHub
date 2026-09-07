package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNotificationRequiresRecipientScopeAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	value := Notification{
		ID: uuid.New(), RecipientID: uuid.New(), OrganizationID: uuid.New(),
		ProjectID: uuid.New(), EnvironmentID: uuid.New(), EventType: "deployment.request.created",
		ResourceType: "deployment_request", ResourceID: uuid.New(), EventID: uuid.New(),
		OccurredAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	require.True(t, value.Valid())
	value.ExpiresAt = now
	require.False(t, value.Valid())
}
