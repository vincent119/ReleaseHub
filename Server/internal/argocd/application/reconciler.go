// Package application coordinates read-only Argo CD reconciliation.
package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

// Client exposes only the Argo CD read contract required by discovery.
type Client interface {
	ListApplications(context.Context) ([]argodomain.Application, error)
}

// Store persists one complete Argo CD observation atomically.
type Store interface {
	Reconcile(context.Context, []argodomain.Application, time.Time) (Result, error)
}

// Observer records low-cardinality reconciliation telemetry.
type Observer interface {
	ObserveReconciliation(status string, elapsed time.Duration, result Result, err error)
}

// Result summarizes persisted observations without exposing Application identities.
type Result struct {
	ObservedApplications int
	Candidates           int
	TrackedApplications  int
	NewCandidates        int
}

// Reconciler periodically reads Argo CD without requesting refresh or mutation.
type Reconciler struct {
	client   Client
	store    Store
	observer Observer
	interval time.Duration
	now      func() time.Time
	mu       sync.Mutex
}

// NewReconciler validates the reconciliation dependencies.
func NewReconciler(client Client, store Store, observer Observer, interval time.Duration) (*Reconciler, error) {
	if client == nil || store == nil || observer == nil {
		return nil, fmt.Errorf("invalid Argo CD reconciler dependencies: client, store, and observer are required")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("invalid Argo CD reconciliation interval: must be greater than 0")
	}
	return &Reconciler{client: client, store: store, observer: observer, interval: interval, now: time.Now}, nil
}

// Run reconciles immediately and then at the configured interval until cancellation.
func (r *Reconciler) Run(ctx context.Context) error {
	_, _ = r.ReconcileOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = r.ReconcileOnce(ctx)
		}
	}
}

// ReconcileOnce executes the same bounded operation used by polling and future manual refresh commands.
func (r *Reconciler) ReconcileOnce(ctx context.Context) (Result, error) {
	// A manual refresh and the ticker must not let two reconciliation tokens overwrite each other.
	r.mu.Lock()
	defer r.mu.Unlock()
	started := r.now().UTC()
	values, err := r.client.ListApplications(ctx)
	if err != nil {
		r.observer.ObserveReconciliation("error", r.now().UTC().Sub(started), Result{}, err)
		return Result{}, err
	}
	result, err := r.store.Reconcile(ctx, values, started)
	if err != nil {
		r.observer.ObserveReconciliation("error", r.now().UTC().Sub(started), Result{ObservedApplications: len(values)}, err)
		return Result{}, err
	}
	r.observer.ObserveReconciliation("success", r.now().UTC().Sub(started), result, nil)
	return result, nil
}
