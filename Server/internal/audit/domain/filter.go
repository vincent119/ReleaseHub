package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

const (
	DefaultLimit      = 20
	MaximumLimit      = 100
	DefaultQueryRange = 24 * time.Hour
	MaximumQueryRange = 90 * 24 * time.Hour
)

var ErrInvalidFilter = errors.New("audit query filter is invalid")

// QueryFilter defines one bounded, stable Audit Trail query.
type QueryFilter struct {
	Scope        authz.Scope
	OccurredFrom time.Time
	OccurredTo   time.Time
	Action       string
	ResourceType string
	Actor        string
	RequestID    string
	Limit        int
}

// Normalize applies defaults and validates all query bounds.
func (f QueryFilter) Normalize(now time.Time) (QueryFilter, error) {
	now = now.UTC()
	if f.OccurredTo.IsZero() {
		f.OccurredTo = now
	} else {
		f.OccurredTo = f.OccurredTo.UTC()
	}
	if f.OccurredFrom.IsZero() {
		f.OccurredFrom = f.OccurredTo.Add(-DefaultQueryRange)
	} else {
		f.OccurredFrom = f.OccurredFrom.UTC()
	}
	if !f.OccurredFrom.Before(f.OccurredTo) || f.OccurredTo.Sub(f.OccurredFrom) > MaximumQueryRange {
		return QueryFilter{}, ErrInvalidFilter
	}
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit < 1 || f.Limit > MaximumLimit {
		return QueryFilter{}, ErrInvalidFilter
	}
	if err := validateScope(f.Scope); err != nil {
		return QueryFilter{}, ErrInvalidFilter
	}
	f.Action = strings.TrimSpace(f.Action)
	f.ResourceType = strings.TrimSpace(f.ResourceType)
	f.Actor = strings.TrimSpace(f.Actor)
	f.RequestID = strings.TrimSpace(f.RequestID)
	for _, value := range []string{f.Action, f.ResourceType, f.Actor, f.RequestID} {
		if len(value) > 255 {
			return QueryFilter{}, ErrInvalidFilter
		}
	}
	return f, nil
}

// Fingerprint binds an opaque cursor to all normalized filters except page size.
func (f QueryFilter) Fingerprint() string {
	value, _ := json.Marshal(struct {
		ScopePath    string    `json:"scopePath"`
		OccurredFrom time.Time `json:"occurredFrom"`
		OccurredTo   time.Time `json:"occurredTo"`
		Action       string    `json:"action"`
		ResourceType string    `json:"resourceType"`
		Actor        string    `json:"actor"`
		RequestID    string    `json:"requestId"`
	}{f.Scope.Path(), f.OccurredFrom, f.OccurredTo, f.Action, f.ResourceType, strings.ToLower(f.Actor), f.RequestID})
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validateScope(scope authz.Scope) error {
	permission, _ := authz.NewPermission("audit.view")
	return (authz.AuthorizationRequest{UserID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Permission: permission, Scope: scope}).Validate()
}
