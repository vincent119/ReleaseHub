package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidApplicationLock = errors.New("invalid application lock request")
	ErrApplicationLocked      = errors.New("application operation is locked")
	ErrStaleApplicationLock   = errors.New("stale application lock lease")
)

type ApplicationLockRequest struct {
	ExecutionID    uuid.UUID
	ApplicationIDs []uuid.UUID
	OwnerToken     uuid.UUID
	Now            time.Time
	LeaseDuration  time.Duration
}

type ApplicationLockLease struct {
	ApplicationID uuid.UUID
	ExecutionID   uuid.UUID
	OwnerToken    uuid.UUID
	FencingToken  uint64
	HeartbeatAt   time.Time
	ExpiresAt     time.Time
}

type ApplicationLockSet struct {
	Leases []ApplicationLockLease
}

func (request ApplicationLockRequest) Valid() bool {
	return request.ExecutionID != uuid.Nil && request.OwnerToken != uuid.Nil &&
		!request.Now.IsZero() && request.LeaseDuration > 0 && validApplicationIDs(request.ApplicationIDs)
}

func validApplicationIDs(values []uuid.UUID) bool {
	if len(values) == 0 {
		return false
	}
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(seen) == len(values)
}
