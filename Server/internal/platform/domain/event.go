// Package domain contains framework-independent shared platform concepts.
package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Event is an immutable domain event stored in the transactional outbox.
type Event struct {
	id            uuid.UUID
	eventType     string
	aggregateType string
	aggregateID   string
	payload       []byte
	occurredAt    time.Time
}

// NewEvent creates an immutable event with a generated id and UTC occurrence time.
func NewEvent(eventType, aggregateType, aggregateID string, payload []byte, occurredAt time.Time) (Event, error) {
	for _, value := range []struct {
		name  string
		value string
	}{
		{"event type", eventType},
		{"aggregate type", aggregateType},
		{"aggregate ID", aggregateID},
	} {
		if strings.TrimSpace(value.value) == "" {
			return Event{}, fmt.Errorf("%s is required", value.name)
		}
	}
	if len(payload) == 0 {
		return Event{}, errors.New("event payload is required")
	}
	if occurredAt.IsZero() {
		return Event{}, errors.New("event occurrence time is required")
	}
	return Event{
		id:            uuid.New(),
		eventType:     eventType,
		aggregateType: aggregateType,
		aggregateID:   aggregateID,
		payload:       slices.Clone(payload),
		occurredAt:    occurredAt.UTC(),
	}, nil
}

func (e Event) ID() uuid.UUID         { return e.id }
func (e Event) Type() string          { return e.eventType }
func (e Event) AggregateType() string { return e.aggregateType }
func (e Event) AggregateID() string   { return e.aggregateID }
func (e Event) Payload() []byte       { return slices.Clone(e.payload) }
func (e Event) OccurredAt() time.Time { return e.occurredAt }
