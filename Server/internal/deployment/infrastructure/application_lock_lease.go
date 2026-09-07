package infrastructure

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (locks *ApplicationLocks) Heartbeat(ctx context.Context, set deploydomain.ApplicationLockSet, now time.Time) (deploydomain.ApplicationLockSet, error) {
	duration, err := lockLeaseDuration(set, now)
	if err != nil {
		return deploydomain.ApplicationLockSet{}, err
	}
	err = locks.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return heartbeatApplicationLocks(tx, set.Leases, now.UTC(), duration)
	})
	if err != nil {
		return deploydomain.ApplicationLockSet{}, fmt.Errorf("heartbeat application lock set: %w", err)
	}
	for index := range set.Leases {
		set.Leases[index].HeartbeatAt = now.UTC()
		set.Leases[index].ExpiresAt = now.Add(duration).UTC()
	}
	return set, nil
}

func (locks *ApplicationLocks) Release(ctx context.Context, set deploydomain.ApplicationLockSet) error {
	if len(set.Leases) == 0 {
		return deploydomain.ErrInvalidApplicationLock
	}
	err := locks.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return releaseApplicationLocks(tx, set.Leases)
	})
	if err != nil {
		return fmt.Errorf("release application lock set: %w", err)
	}
	return nil
}

func heartbeatApplicationLocks(tx *gorm.DB, leases []deploydomain.ApplicationLockLease, now time.Time, duration time.Duration) error {
	for _, lease := range leases {
		result := lockLeaseQuery(tx, lease).Where("heartbeat_at > ?", now.Add(-duration)).Update("heartbeat_at", now)
		if err := exactLockMutation(result, "heartbeat"); err != nil {
			return err
		}
	}
	return nil
}

func lockLeaseDuration(set deploydomain.ApplicationLockSet, now time.Time) (time.Duration, error) {
	if len(set.Leases) == 0 || now.IsZero() {
		return 0, deploydomain.ErrInvalidApplicationLock
	}
	duration := set.Leases[0].ExpiresAt.Sub(set.Leases[0].HeartbeatAt)
	if duration <= 0 {
		return 0, deploydomain.ErrInvalidApplicationLock
	}
	if !now.Before(set.Leases[0].ExpiresAt) {
		return 0, deploydomain.ErrStaleApplicationLock
	}
	return duration, nil
}

func releaseApplicationLocks(tx *gorm.DB, leases []deploydomain.ApplicationLockLease) error {
	for _, lease := range leases {
		result := lockLeaseQuery(tx, lease).Delete(&applicationLockModel{})
		if err := exactLockMutation(result, "release"); err != nil {
			return err
		}
	}
	return nil
}

func lockLeaseQuery(tx *gorm.DB, lease deploydomain.ApplicationLockLease) *gorm.DB {
	return tx.Model(&applicationLockModel{}).Where(
		"application_id = ? AND execution_id = ? AND owner_token = ? AND fencing_token = ?",
		lease.ApplicationID, lease.ExecutionID, lease.OwnerToken, lease.FencingToken,
	)
}

func exactLockMutation(result *gorm.DB, action string) error {
	if result.Error != nil {
		return fmt.Errorf("%s application lock: %w", action, result.Error)
	}
	if result.RowsAffected != 1 {
		return deploydomain.ErrStaleApplicationLock
	}
	return nil
}
