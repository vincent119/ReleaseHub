package infrastructure

import (
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent119/zlogger"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

// DeploymentScheduleObserver emits bounded metrics and logs for policy deferrals.
type DeploymentScheduleObserver struct {
	logger   *zlogger.Logger
	deferred *prometheus.CounterVec
	wait     *prometheus.HistogramVec
}

var _ deployapp.DeploymentScheduleObserver = (*DeploymentScheduleObserver)(nil)

// NewDeploymentScheduleObserver registers schedule telemetry with the worker registry.
func NewDeploymentScheduleObserver(registry prometheus.Registerer, logger *zlogger.Logger) (*DeploymentScheduleObserver, error) {
	if registry == nil || logger == nil {
		return nil, errors.New("deployment schedule observer dependencies are required")
	}
	observer := newDeploymentScheduleObserver(logger)
	for _, collector := range []prometheus.Collector{observer.deferred, observer.wait} {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register deployment schedule metric: %w", err)
		}
	}
	return observer, nil
}

func newDeploymentScheduleObserver(logger *zlogger.Logger) *DeploymentScheduleObserver {
	return &DeploymentScheduleObserver{
		logger: logger,
		deferred: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "releasehub", Subsystem: "deployment", Name: "schedule_deferred_total",
			Help: "Total number of deployment jobs deferred by schedule policy.",
		}, []string{"reason"}),
		wait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "releasehub", Subsystem: "deployment", Name: "schedule_wait_seconds",
			Help: "Scheduled deployment wait duration in seconds.", Buckets: prometheus.DefBuckets,
		}, []string{"reason"}),
	}
}

// ObserveDeploymentScheduleDefer records one defer without resource identifiers.
func (o *DeploymentScheduleObserver) ObserveDeploymentScheduleDefer(value deployapp.DeploymentScheduleDeferObservation) {
	o.deferred.WithLabelValues(value.Reason).Inc()
	o.wait.WithLabelValues(value.Reason).Observe(value.Wait.Seconds())
	o.logger.Info("Deployment job deferred by schedule",
		zlogger.String("job_type", string(value.JobType)),
		zlogger.String("reason", value.Reason),
		zlogger.Duration("wait", value.Wait),
		zlogger.String("next_eligible_at", value.NextEligibleAt.UTC().Format(time.RFC3339)),
		zlogger.String("policy_version", fmt.Sprint(value.PolicyVersion)),
	)
}
