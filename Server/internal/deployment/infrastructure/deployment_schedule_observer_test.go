package infrastructure

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/vincent119/zlogger"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentScheduleObserverRecordsBoundedReasonMetric(t *testing.T) {
	registry := prometheus.NewRegistry()
	observer, err := NewDeploymentScheduleObserver(registry, zlogger.NewNop())
	require.NoError(t, err)

	observer.ObserveDeploymentScheduleDefer(deployapp.DeploymentScheduleDeferObservation{
		JobType: deploydomain.JobExecuteDeployment, Reason: "MaintenanceWindow",
		Wait: 2 * time.Hour, NextEligibleAt: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC),
		PolicyVersion: 4,
	})

	require.Equal(t, float64(1), testutil.ToFloat64(observer.deferred.WithLabelValues("MaintenanceWindow")))
	metrics, err := registry.Gather()
	require.NoError(t, err)
	found := false
	for _, metric := range metrics {
		found = found || metric.GetName() == "releasehub_deployment_schedule_wait_seconds"
	}
	require.True(t, found)
}
