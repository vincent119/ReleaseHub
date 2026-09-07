package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const policyRevisionChannel = "releasehub_policy_revision"

// PolicyRevisionReloader repairs a process-local policy cache after notification.
type PolicyRevisionReloader interface {
	ReloadIfStale(context.Context) error
}

// PolicyRevisionListener owns one dedicated PostgreSQL LISTEN connection.
type PolicyRevisionListener struct {
	databaseURL string
	reloader    PolicyRevisionReloader
	ready       chan struct{}
	readyOnce   sync.Once
}

// NewPolicyRevisionListener creates a listener without opening a connection.
func NewPolicyRevisionListener(databaseURL string, reloader PolicyRevisionReloader) (*PolicyRevisionListener, error) {
	if databaseURL == "" || reloader == nil {
		return nil, fmt.Errorf("database URL and policy reloader are required")
	}
	return &PolicyRevisionListener{databaseURL: databaseURL, reloader: reloader, ready: make(chan struct{})}, nil
}

// Ready closes after LISTEN is active and the initial revision has been checked.
func (l *PolicyRevisionListener) Ready() <-chan struct{} { return l.ready }

// Run reloads policy after every committed revision notification.
func (l *PolicyRevisionListener) Run(ctx context.Context) error {
	connection, err := pgx.Connect(ctx, l.databaseURL)
	if err != nil {
		return fmt.Errorf("connect policy revision listener: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = connection.Close(closeCtx)
	}()
	if _, err := connection.Exec(ctx, "LISTEN "+policyRevisionChannel); err != nil {
		return fmt.Errorf("listen for policy revisions: %w", err)
	}
	if err := l.reloader.ReloadIfStale(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("load initial policy revision: %w", err)
	}
	l.readyOnce.Do(func() { close(l.ready) })
	for {
		if _, err := connection.WaitForNotification(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("wait for policy revision: %w", err)
		}
		if err := l.reloader.ReloadIfStale(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("reload notified policy revision: %w", err)
		}
	}
}
