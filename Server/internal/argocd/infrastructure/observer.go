package infrastructure

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent119/zlogger"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
)

// ReconciliationObserver emits bounded metrics and failure logs for the Worker.
type ReconciliationObserver struct {
	logger       *zlogger.Logger
	cycles       *prometheus.CounterVec
	duration     prometheus.Histogram
	applications prometheus.Gauge
	candidates   prometheus.Gauge
	tracked      prometheus.Gauge
}

var _ argoapp.Observer = (*ReconciliationObserver)(nil)

// NewReconciliationObserver registers Argo CD reconciliation metrics.
func NewReconciliationObserver(registry prometheus.Registerer, logger *zlogger.Logger) (*ReconciliationObserver, error) {
	if registry == nil || logger == nil {
		return nil, fmt.Errorf("invalid reconciliation observer dependencies: Prometheus registry and logger are required")
	}
	observer := &ReconciliationObserver{
		logger: logger,
		cycles: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "reconciliation_total",
			Help: "Total number of Argo CD reconciliation cycles.",
		}, []string{"status"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "reconciliation_duration_seconds",
			Help: "Argo CD reconciliation latency in seconds.", Buckets: prometheus.DefBuckets,
		}),
		applications: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "observed_applications",
			Help: "Number of Applications returned by the latest successful Argo CD reconciliation.",
		}),
		candidates: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "candidate_applications",
			Help: "Number of candidate Applications in the latest successful Argo CD reconciliation.",
		}),
		tracked: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "tracked_applications",
			Help: "Number of catalog Applications observed in the latest successful Argo CD reconciliation.",
		}),
	}
	for _, collector := range []prometheus.Collector{observer.cycles, observer.duration, observer.applications, observer.candidates, observer.tracked} {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register Argo CD reconciliation metric: %w", err)
		}
	}
	return observer, nil
}

// OnboardingRecoveryObserver emits bounded metrics and failure logs for restart recovery.
type OnboardingRecoveryObserver struct {
	logger *zlogger.Logger
	cycles *prometheus.CounterVec
}

var _ argoapp.RecoveryObserver = (*OnboardingRecoveryObserver)(nil)

// NewOnboardingRecoveryObserver registers the onboarding recovery metric.
func NewOnboardingRecoveryObserver(registry prometheus.Registerer, logger *zlogger.Logger) (*OnboardingRecoveryObserver, error) {
	if registry == nil || logger == nil {
		return nil, fmt.Errorf("invalid onboarding recovery observer dependencies: Prometheus registry and logger are required")
	}
	observer := &OnboardingRecoveryObserver{
		logger: logger,
		cycles: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "releasehub", Subsystem: "argocd", Name: "onboarding_recovery_total",
			Help: "Total number of Application onboarding recovery cycles.",
		}, []string{"status"}),
	}
	if err := registry.Register(observer.cycles); err != nil {
		return nil, fmt.Errorf("register onboarding recovery metric: %w", err)
	}
	return observer, nil
}

// ObserveOnboardingRecovery records one recovery cycle without external identifiers.
func (o *OnboardingRecoveryObserver) ObserveOnboardingRecovery(err error) {
	if err != nil {
		o.cycles.WithLabelValues("error").Inc()
		o.logger.Error("Application onboarding recovery failed", zlogger.Err(err))
		return
	}
	o.cycles.WithLabelValues("success").Inc()
}

// ObserveReconciliation records one cycle without labels derived from external identifiers.
func (o *ReconciliationObserver) ObserveReconciliation(status string, elapsed time.Duration, result argoapp.Result, err error) {
	o.cycles.WithLabelValues(status).Inc()
	o.duration.Observe(elapsed.Seconds())
	if err != nil {
		o.logger.Error("Argo CD reconciliation failed", zlogger.Err(err))
		return
	}
	o.applications.Set(float64(result.ObservedApplications))
	o.candidates.Set(float64(result.Candidates))
	o.tracked.Set(float64(result.TrackedApplications))
}
