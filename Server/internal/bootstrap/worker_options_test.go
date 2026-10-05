package bootstrap

import (
	"testing"
	"time"

	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

func TestDeploymentExecutorOptionsPreserveWorkerConfigurationAndPorts(t *testing.T) {
	argo := &argoinfra.Client{}
	preflight := &deployapp.PreflightService{}
	repository := &deployinfra.ExecutionRepository{}
	locks := &deployinfra.ApplicationLocks{}
	dependencies := workerDependencies{
		resources: &processResources{argoClient: argo},
		cfg: config.Config{Worker: config.WorkerConfig{
			MaxParallelDeployments: 7, ApplicationLockDuration: 3 * time.Minute,
			DeploymentPollInterval: 11 * time.Second,
		}},
	}
	options := deploymentExecutorOptions(dependencies, preflight, repository, locks)
	if options.Repository != repository || options.Preflight != preflight || options.Argo != argo || options.Locks != locks {
		t.Fatal("executor ports changed")
	}
	if options.MaxParallel != 7 || options.LockTTL != 3*time.Minute || options.WatchTimeout != 11*time.Second {
		t.Fatalf("worker configuration changed: %#v", options)
	}
}
