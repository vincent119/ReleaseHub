package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOnboardingRecoveryRunsImmediatelyAndContinuesAfterFailure(t *testing.T) {
	t.Parallel()
	recoveryErr := errors.New("temporary recovery failure")
	service := &recoveryServiceStub{errors: []error{recoveryErr, nil}}
	observer := &recoveryObserverStub{}
	runner, err := NewOnboardingRecovery(service, observer, time.Millisecond)
	if err != nil {
		t.Fatalf("NewOnboardingRecovery() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if calls := service.callCount(); calls < 2 {
		t.Fatalf("RecoverApplying() calls = %d, want at least 2", calls)
	}
	if got := observer.first(); !errors.Is(got, recoveryErr) {
		t.Fatalf("first observed error = %v, want %v", got, recoveryErr)
	}
}

type recoveryServiceStub struct {
	mu     sync.Mutex
	calls  int
	errors []error
}

func (s *recoveryServiceStub) RecoverApplying(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.calls
	s.calls++
	if index >= len(s.errors) {
		return nil
	}
	return s.errors[index]
}

func (s *recoveryServiceStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type recoveryObserverStub struct {
	mu     sync.Mutex
	errors []error
}

func (o *recoveryObserverStub) ObserveOnboardingRecovery(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.errors = append(o.errors, err)
}

func (o *recoveryObserverStub) first() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.errors) == 0 {
		return nil
	}
	return o.errors[0]
}
