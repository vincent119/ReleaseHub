package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type ApplicationLocks struct {
	db *gorm.DB
}

func NewApplicationLocks(db *gorm.DB) (*ApplicationLocks, error) {
	if db == nil {
		return nil, errors.New("application locks database is required")
	}
	return &ApplicationLocks{db: db}, nil
}

func (locks *ApplicationLocks) Acquire(ctx context.Context, request deploydomain.ApplicationLockRequest) (deploydomain.ApplicationLockSet, error) {
	if !request.Valid() {
		return deploydomain.ApplicationLockSet{}, deploydomain.ErrInvalidApplicationLock
	}
	applicationIDs := sortedApplicationIDs(request.ApplicationIDs)
	var leases []deploydomain.ApplicationLockLease
	err := locks.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var acquireError error
		leases, acquireError = acquireApplicationLocks(tx, request, applicationIDs)
		return acquireError
	})
	return deploydomain.ApplicationLockSet{Leases: leases}, lockTransactionError(err)
}

func acquireApplicationLocks(tx *gorm.DB, request deploydomain.ApplicationLockRequest, applicationIDs []uuid.UUID) ([]deploydomain.ApplicationLockLease, error) {
	leases := make([]deploydomain.ApplicationLockLease, 0, len(applicationIDs))
	for _, applicationID := range applicationIDs {
		lease, err := acquireApplicationLock(tx, request, applicationID)
		if err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, nil
}

func acquireApplicationLock(tx *gorm.DB, request deploydomain.ApplicationLockRequest, applicationID uuid.UUID) (deploydomain.ApplicationLockLease, error) {
	var model applicationLockModel
	args := []any{applicationID, request.ExecutionID, request.OwnerToken, request.Now.UTC(), request.Now.UTC(), request.Now.Add(-request.LeaseDuration).UTC()}
	if err := tx.Raw(acquireApplicationLockSQL(), args...).Scan(&model).Error; err != nil {
		return deploydomain.ApplicationLockLease{}, fmt.Errorf("acquire application lock: %w", err)
	}
	if model.ApplicationID == uuid.Nil {
		return deploydomain.ApplicationLockLease{}, deploydomain.ErrApplicationLocked
	}
	return applicationLockFromModel(model, request.LeaseDuration), nil
}

func sortedApplicationIDs(values []uuid.UUID) []uuid.UUID {
	result := slices.Clone(values)
	slices.SortFunc(result, func(left, right uuid.UUID) int { return bytes.Compare(left[:], right[:]) })
	return result
}

func lockTransactionError(err error) error {
	if err == nil || errors.Is(err, deploydomain.ErrApplicationLocked) {
		return err
	}
	return fmt.Errorf("acquire application lock set: %w", err)
}

func acquireApplicationLockSQL() string {
	return `INSERT INTO deployment_application_locks AS current (
		application_id, execution_id, owner_token, fencing_token, acquired_at, heartbeat_at
	) VALUES (?, ?, ?, 1, ?, ?)
	ON CONFLICT (application_id) DO UPDATE SET
		execution_id = EXCLUDED.execution_id, owner_token = EXCLUDED.owner_token,
		fencing_token = CASE WHEN current.execution_id = EXCLUDED.execution_id AND current.owner_token = EXCLUDED.owner_token
			THEN current.fencing_token ELSE current.fencing_token + 1 END,
		acquired_at = CASE WHEN current.execution_id = EXCLUDED.execution_id AND current.owner_token = EXCLUDED.owner_token
			THEN current.acquired_at ELSE EXCLUDED.acquired_at END,
		heartbeat_at = EXCLUDED.heartbeat_at
	WHERE current.execution_id = EXCLUDED.execution_id
		OR current.heartbeat_at <= ? RETURNING current.*`
}

var _ deployapp.ApplicationLocks = (*ApplicationLocks)(nil)
