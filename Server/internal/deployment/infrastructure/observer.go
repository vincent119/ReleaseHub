package infrastructure

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent119/zlogger"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

// CandidateObserver emits bounded metrics and failure logs for candidate reconciliation.
type CandidateObserver struct {
	logger     *zlogger.Logger
	cycles     *prometheus.CounterVec
	duration   prometheus.Histogram
	created    prometheus.Counter
	duplicates prometheus.Counter
	failures   prometheus.Counter
}

var _ deployapp.CandidateObserver = (*CandidateObserver)(nil)

// NewCandidateObserver registers deployment candidate reconciliation metrics.
func NewCandidateObserver(registry prometheus.Registerer, logger *zlogger.Logger) (*CandidateObserver, error) {
	if registry == nil || logger == nil {
		return nil, fmt.Errorf("invalid candidate observer dependencies: Prometheus registry and logger are required")
	}
	observer := newCandidateObserver(logger)
	if err := registerCandidateCollectors(registry, observer); err != nil {
		return nil, err
	}
	return observer, nil
}

func newCandidateObserver(logger *zlogger.Logger) *CandidateObserver {
	return &CandidateObserver{
		logger: logger, cycles: candidateCycles(), duration: candidateDuration(),
		created:    candidateCounter("candidate_requests_created_total", "Total number of Deployment Requests created from candidate observations."),
		duplicates: candidateCounter("candidate_duplicates_total", "Total number of duplicate deployment candidates ignored."),
		failures:   candidateCounter("candidate_application_failures_total", "Total number of Application candidate reconciliation failures."),
	}
}

func candidateCycles() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "releasehub", Subsystem: "deployment", Name: "candidate_reconciliation_total",
		Help: "Total number of deployment candidate reconciliation cycles.",
	}, []string{"status"})
}

func candidateDuration() prometheus.Histogram {
	return prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "releasehub", Subsystem: "deployment", Name: "candidate_reconciliation_duration_seconds",
		Help: "Deployment candidate reconciliation latency in seconds.", Buckets: prometheus.DefBuckets,
	})
}

func candidateCounter(name, help string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "releasehub", Subsystem: "deployment", Name: name, Help: help,
	})
}

func registerCandidateCollectors(registry prometheus.Registerer, observer *CandidateObserver) error {
	for _, collector := range []prometheus.Collector{observer.cycles, observer.duration, observer.created, observer.duplicates, observer.failures} {
		if err := registry.Register(collector); err != nil {
			return fmt.Errorf("register deployment candidate metric: %w", err)
		}
	}
	return nil
}

// ObserveCandidateReconciliation records one cycle without external identifiers.
func (o *CandidateObserver) ObserveCandidateReconciliation(status string, elapsed time.Duration, result deployapp.CandidateResult, err error) {
	o.cycles.WithLabelValues(status).Inc()
	o.duration.Observe(elapsed.Seconds())
	o.created.Add(float64(result.CreatedRequests))
	o.duplicates.Add(float64(result.DuplicateCandidates))
	o.failures.Add(float64(result.FailedApplications))
	if err != nil {
		o.logger.Error("Deployment candidate reconciliation failed", zlogger.Err(err))
	}
}
