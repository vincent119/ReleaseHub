package bootstrap

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent119/commons/graceful"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	ecrapp "github.com/vincent119/ReleaseHub/Server/internal/ecr/application"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
	ecrinfra "github.com/vincent119/ReleaseHub/Server/internal/ecr/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/lifecycle"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
)

type workerApplication struct {
	cfg       config.Config
	resources *processResources
	tasks     []graceful.Task
}

type workerDependencies struct {
	cfg       config.Config
	resources *processResources
	registry  *prometheus.Registry
	policy    *policyModule
}

type candidateModules struct {
	repository *deployinfra.CandidateRepository
	observer   *deployinfra.CandidateObserver
	workflows  *deployapp.WorkflowRuntimeService
}

// RunWorker creates and runs the Worker process.
func RunWorker(cfg config.Config) (result error) {
	resources, err := newProcessResources(cfg, cfg.Database.Pools.Worker, observability.ComponentWorker)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, resources.closeAfterRun(context.Background()))
	}()
	app, err := newWorkerApplication(cfg, resources)
	if err != nil {
		_ = resources.runtime.closeTracer(context.Background())
		return err
	}
	return app.run()
}

func newWorkerApplication(cfg config.Config, resources *processResources) (*workerApplication, error) {
	registry := prometheus.NewRegistry()
	policy, err := newPolicyModule(resources.db, cfg.Database)
	if err != nil {
		return nil, err
	}
	dependencies := workerDependencies{cfg: cfg, resources: resources, registry: registry, policy: policy}
	tasks, err := newWorkerTasks(dependencies)
	if err != nil {
		return nil, err
	}
	return &workerApplication{cfg: cfg, resources: resources, tasks: tasks}, nil
}

func newWorkerTasks(dependencies workerDependencies) ([]graceful.Task, error) {
	reconciler, err := newApplicationReconciler(dependencies)
	if err != nil {
		return nil, err
	}
	candidates, err := newCandidateReconciler(dependencies)
	if err != nil {
		return nil, err
	}
	onboarding, err := newOnboardingRecovery(dependencies)
	if err != nil {
		return nil, err
	}
	processing, err := newWorkerProcessingTasks(dependencies)
	if err != nil {
		return nil, err
	}
	return []graceful.Task{dependencies.policy.listener.Run, reconciler.Run,
		onboarding.Run, candidates.Run, processing[0], processing[1]}, nil
}

func newWorkerProcessingTasks(dependencies workerDependencies) ([]graceful.Task, error) {
	deployments, err := newDeploymentJobConsumer(dependencies)
	if err != nil {
		return nil, err
	}
	notifications, err := newNotificationWorker(dependencies)
	if err != nil {
		return nil, err
	}
	return []graceful.Task{deployments.Run, notifications.Run}, nil
}

func newNotificationWorker(dependencies workerDependencies) (*deployapp.NotificationWorker, error) {
	projector, err := deployinfra.NewNotificationProjector(dependencies.resources.db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewNotificationWorker(deployapp.NotificationWorkerOptions{
		Store: projector, PollInterval: dependencies.cfg.Notifications.ProjectionInterval,
		Retention: dependencies.cfg.Notifications.Retention,
	})
}

func newDeploymentJobConsumer(dependencies workerDependencies) (*deployapp.DeploymentJobConsumer, error) {
	executor, queue, err := newDeploymentConsumerExecution(dependencies)
	if err != nil {
		return nil, err
	}
	schedules, observer, err := newDeploymentScheduleWorker(dependencies)
	if err != nil {
		return nil, err
	}
	cfg := dependencies.cfg.Worker
	return deployapp.NewDeploymentJobConsumer(deployapp.DeploymentJobConsumerOptions{
		Queue: queue, Executor: executor, Schedules: schedules, Observer: observer,
		Owner: "worker-" + uuid.NewString(), PollInterval: cfg.DeploymentPollInterval,
		LeaseTTL: cfg.JobLeaseDuration, RetryDelay: cfg.JobRetryDelay,
	})
}

func newDeploymentConsumerExecution(dependencies workerDependencies) (*deployapp.DeploymentExecutor, *deployinfra.DeploymentJobQueue, error) {
	locker, err := newImageLocker(dependencies.cfg)
	if err != nil {
		return nil, nil, err
	}
	preflight, err := deployapp.NewPreflightService(dependencies.resources.argoClient, locker)
	if err != nil {
		return nil, nil, err
	}
	return newDeploymentExecutor(dependencies, preflight)
}

func newDeploymentScheduleWorker(dependencies workerDependencies) (*deployinfra.DeploymentScheduleGate, *deployinfra.DeploymentScheduleObserver, error) {
	schedules, err := deployinfra.NewDeploymentScheduleGate(dependencies.resources.db)
	if err != nil {
		return nil, nil, err
	}
	observer, err := deployinfra.NewDeploymentScheduleObserver(dependencies.registry, dependencies.resources.runtime.logger)
	return schedules, observer, err
}

func newDeploymentExecutor(dependencies workerDependencies, preflight *deployapp.PreflightService) (*deployapp.DeploymentExecutor, *deployinfra.DeploymentJobQueue, error) {
	repository, err := deployinfra.NewExecutionRepository(dependencies.resources.db)
	if err != nil {
		return nil, nil, err
	}
	locks, err := deployinfra.NewApplicationLocks(dependencies.resources.db)
	if err != nil {
		return nil, nil, err
	}
	queue, err := deployinfra.NewDeploymentJobQueue(dependencies.resources.db)
	if err != nil {
		return nil, nil, err
	}
	cfg := dependencies.cfg.Worker
	executor, err := deployapp.NewDeploymentExecutor(deployapp.DeploymentExecutorOptions{
		Repository: repository, Preflight: preflight, Argo: dependencies.resources.argoClient,
		Locks: locks, MaxParallel: cfg.MaxParallelDeployments, LockTTL: cfg.ApplicationLockDuration,
		WatchTimeout: cfg.DeploymentPollInterval,
	})
	return executor, queue, err
}

func newApplicationReconciler(dependencies workerDependencies) (*argoapp.Reconciler, error) {
	repository, err := argoinfra.NewReconciliationRepository(dependencies.resources.db)
	if err != nil {
		return nil, err
	}
	observer, err := argoinfra.NewReconciliationObserver(dependencies.registry, dependencies.resources.runtime.logger)
	if err != nil {
		return nil, err
	}
	return argoapp.NewReconciler(
		dependencies.resources.argoClient, repository, observer,
		dependencies.cfg.Worker.ReconcileInterval,
	)
}

func newCandidateReconciler(dependencies workerDependencies) (*deployapp.CandidateReconciler, error) {
	locker, err := newImageLocker(dependencies.cfg)
	if err != nil {
		return nil, err
	}
	modules, err := newCandidateModules(dependencies)
	if err != nil {
		return nil, err
	}
	return deployapp.NewCandidateReconciler(deployapp.CandidateReconcilerOptions{
		Repository: modules.repository, Argo: dependencies.resources.argoClient,
		Images: locker, Observer: modules.observer, Workflows: modules.workflows,
		Interval: dependencies.cfg.Worker.ReconcileInterval,
	})
}

func newCandidateModules(dependencies workerDependencies) (candidateModules, error) {
	repository, err := deployinfra.NewCandidateRepository(dependencies.resources.db)
	if err != nil {
		return candidateModules{}, err
	}
	observer, err := deployinfra.NewCandidateObserver(dependencies.registry, dependencies.resources.runtime.logger)
	if err != nil {
		return candidateModules{}, err
	}
	workflows, err := newWorkerWorkflowRuntime(dependencies)
	return candidateModules{repository: repository, observer: observer, workflows: workflows}, err
}

func newWorkerWorkflowRuntime(dependencies workerDependencies) (*deployapp.WorkflowRuntimeService, error) {
	repository, err := deployinfra.NewWorkflowRuntimeRepository(dependencies.resources.db)
	if err != nil {
		return nil, err
	}
	assignments, err := deployinfra.NewReviewAssignmentRepository(dependencies.resources.db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewWorkflowRuntimeService(deployapp.WorkflowRuntimeServiceOptions{
		Repository: repository, Authorizer: dependencies.policy.engine,
		Assignments: assignments, Clock: deployapp.SystemWorkflowClock{},
	})
}

func newImageLocker(cfg config.Config) (*ecrapp.ManifestImageLocker, error) {
	client, err := ecrinfra.NewClient(context.Background(), cfg.AWS)
	if err != nil {
		return nil, err
	}
	scope, err := ecrdomain.NewRegistryScope(cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepositories)
	if err != nil {
		return nil, err
	}
	return ecrapp.NewManifestImageLocker(client, scope)
}

func newOnboardingRecovery(dependencies workerDependencies) (*argoapp.OnboardingRecovery, error) {
	repository, err := argoinfra.NewOnboardingRepository(dependencies.resources.db)
	if err != nil {
		return nil, err
	}
	service, err := argoapp.NewOnboardingService(
		repository, dependencies.resources.argoClient, dependencies.policy.engine,
	)
	if err != nil {
		return nil, err
	}
	observer, err := argoinfra.NewOnboardingRecoveryObserver(dependencies.registry, dependencies.resources.runtime.logger)
	if err != nil {
		return nil, err
	}
	return argoapp.NewOnboardingRecovery(service, observer, dependencies.cfg.Worker.ReconcileInterval)
}

func (a *workerApplication) run() error {
	a.resources.runtime.logger.Info("Worker started")
	return lifecycle.Run(
		parallelTasks(a.tasks...),
		a.cfg.Runtime.ShutdownTimeout,
		a.resources.runtime.logger,
		a.resources.runtime.closeTracer,
	)
}
