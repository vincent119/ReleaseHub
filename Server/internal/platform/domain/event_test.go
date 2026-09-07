package domain_test

import (
	"testing"
	"time"

	"github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func TestEventCopiesPayloadAndNormalizesOccurrenceTime(t *testing.T) {
	payload := []byte(`{"result":"managed"}`)
	occurredAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	event, err := domain.NewEvent("ApplicationOnboarded", "application", "app-1", payload, occurredAt)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	payload[0] = 'x'
	returned := event.Payload()
	returned[0] = 'y'

	if string(event.Payload()) != `{"result":"managed"}` {
		t.Fatalf("event payload must be immutable, got %s", event.Payload())
	}
	if event.OccurredAt().Location() != time.UTC {
		t.Fatalf("event occurrence time must be UTC, got %s", event.OccurredAt().Location())
	}
}
