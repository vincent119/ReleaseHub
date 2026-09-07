package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestReconcilerRunsImmediatelyAndContinuesAfterReadFailure(t *testing.T) {
	client := &clientStub{errors: []error{errors.New("unavailable"), nil}}
	store := &storeStub{}
	observer := &observerStub{observed: make(chan string, 2)}
	reconciler, err := NewReconciler(client, store, observer, time.Millisecond)
	if err != nil {
		t.Fatalf("create reconciler: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- reconciler.Run(ctx) }()

	for range 2 {
		select {
		case <-observer.observed:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("reconciliation did not continue after a read failure")
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("stop reconciler: %v", err)
	}
	if client.callCount() < 2 || store.callCount() != 1 {
		t.Fatalf("unexpected calls: client=%d store=%d", client.callCount(), store.callCount())
	}
}

type clientStub struct {
	mu     sync.Mutex
	errors []error
	calls  int
}

func (s *clientStub) ListApplications(context.Context) ([]argodomain.Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= len(s.errors) && s.errors[s.calls-1] != nil {
		return nil, s.errors[s.calls-1]
	}
	return []argodomain.Application{}, nil
}

func (s *clientStub) callCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

type storeStub struct {
	mu    sync.Mutex
	calls int
}

func (s *storeStub) Reconcile(_ context.Context, values []argodomain.Application, _ time.Time) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return Result{ObservedApplications: len(values)}, nil
}

func (s *storeStub) callCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

type observerStub struct{ observed chan string }

func (s *observerStub) ObserveReconciliation(status string, _ time.Duration, _ Result, _ error) {
	s.observed <- status
}
