package application

import (
	"context"
	"fmt"
	"time"
)

// ApplyingRecovery resumes onboarding commands that were persisted before an interrupted Argo CD mutation.
type ApplyingRecovery interface {
	RecoverApplying(context.Context) error
}

// RecoveryObserver records bounded onboarding-recovery telemetry.
type RecoveryObserver interface {
	ObserveOnboardingRecovery(error)
}

// OnboardingRecovery runs recovery independently from read-only reconciliation.
type OnboardingRecovery struct {
	service  ApplyingRecovery
	observer RecoveryObserver
	interval time.Duration
}

// NewOnboardingRecovery validates the Worker recovery dependencies.
func NewOnboardingRecovery(service ApplyingRecovery, observer RecoveryObserver, interval time.Duration) (*OnboardingRecovery, error) {
	if service == nil || observer == nil {
		return nil, fmt.Errorf("invalid onboarding recovery dependencies")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("invalid onboarding recovery interval: must be greater than 0")
	}
	return &OnboardingRecovery{service: service, observer: observer, interval: interval}, nil
}

// Run recovers immediately after startup and then periodically until cancellation.
func (r *OnboardingRecovery) Run(ctx context.Context) error {
	r.recoverOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.recoverOnce(ctx)
		}
	}
}

func (r *OnboardingRecovery) recoverOnce(ctx context.Context) {
	r.observer.ObserveOnboardingRecovery(r.service.RecoverApplying(ctx))
}
