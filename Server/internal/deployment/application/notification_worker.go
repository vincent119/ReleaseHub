package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const notificationCleanupInterval = time.Hour

// NotificationProjectionStore projects one event and expires retained rows.
type NotificationProjectionStore interface {
	ProjectNext(context.Context, time.Time, time.Duration) (bool, error)
	DeleteExpired(context.Context, time.Time, int) (int, error)
}

// NotificationWorkerOptions configures projection and retention polling.
type NotificationWorkerOptions struct {
	Store        NotificationProjectionStore
	PollInterval time.Duration
	Retention    time.Duration
}

// NotificationWorker projects Outbox events and enforces notification retention.
type NotificationWorker struct {
	store        NotificationProjectionStore
	pollInterval time.Duration
	retention    time.Duration
}

// NewNotificationWorker creates the cancellable notification worker.
func NewNotificationWorker(options NotificationWorkerOptions) (*NotificationWorker, error) {
	if options.Store == nil || options.PollInterval <= 0 || options.Retention <= 0 {
		return nil, errors.New("notification worker configuration is invalid")
	}
	return &NotificationWorker{store: options.Store, pollInterval: options.PollInterval, retention: options.Retention}, nil
}

// Run projects immediately and continues until cancellation.
func (w *NotificationWorker) Run(ctx context.Context) error {
	project := time.NewTicker(w.pollInterval)
	cleanup := time.NewTicker(notificationCleanupInterval)
	defer project.Stop()
	defer cleanup.Stop()
	if err := w.process(ctx); err != nil {
		return err
	}
	if err := w.expire(ctx); err != nil {
		return err
	}
	return w.runTicks(ctx, project.C, cleanup.C)
}

func (w *NotificationWorker) runTicks(ctx context.Context, project, cleanup <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-project:
			if err := w.process(ctx); err != nil {
				return err
			}
		case <-cleanup:
			if err := w.expire(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *NotificationWorker) process(ctx context.Context) error {
	for range 100 {
		projected, err := w.store.ProjectNext(ctx, time.Now().UTC(), w.retention)
		if err != nil {
			return fmt.Errorf("project notification: %w", err)
		}
		if !projected {
			return nil
		}
	}
	return nil
}

func (w *NotificationWorker) expire(ctx context.Context) error {
	for {
		count, err := w.store.DeleteExpired(ctx, time.Now().UTC(), 500)
		if err != nil {
			return fmt.Errorf("expire notifications: %w", err)
		}
		if count < 500 {
			return nil
		}
	}
}
