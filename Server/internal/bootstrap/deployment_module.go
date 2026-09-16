package bootstrap

import (
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

type deploymentReadServices struct {
	history       *deployapp.DeploymentHistoryService
	notifications *deployapp.NotificationService
}

func newDeploymentHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine, argo *argoinfra.Client) (httpserver.DeploymentHandlerOptions, error) {
	requests, err := newDeploymentRequestService(db, policy)
	if err != nil {
		return httpserver.DeploymentHandlerOptions{}, err
	}
	workflows, err := newAPIWorkflowRuntimeService(db, policy)
	if err != nil {
		return httpserver.DeploymentHandlerOptions{}, err
	}
	reads, err := newDeploymentReadServices(db, policy)
	if err != nil {
		return httpserver.DeploymentHandlerOptions{}, err
	}
	options := httpserver.DeploymentHandlerOptions{
		Requests: requests, Workflows: workflows, History: reads.history,
		Notifications: reads.notifications,
	}
	return addExecutionControl(db, policy, argo, options)
}

func addExecutionControl(db *gorm.DB, policy *authzinfra.PolicyEngine, argo *argoinfra.Client, options httpserver.DeploymentHandlerOptions) (httpserver.DeploymentHandlerOptions, error) {
	if argo == nil {
		return options, nil
	}
	execution, err := newExecutionControlService(db, policy, argo)
	options.Executions = execution
	return options, err
}

func newDeploymentReadServices(db *gorm.DB, policy *authzinfra.PolicyEngine) (deploymentReadServices, error) {
	history, err := newDeploymentHistoryService(db, policy)
	if err != nil {
		return deploymentReadServices{}, err
	}
	notifications, err := newNotificationService(db, policy)
	return deploymentReadServices{history: history, notifications: notifications}, err
}

func newNotificationService(db *gorm.DB, policy *authzinfra.PolicyEngine) (*deployapp.NotificationService, error) {
	repository, err := deployinfra.NewNotificationRepository(db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewNotificationService(repository, policy)
}

func newDeploymentHistoryService(db *gorm.DB, policy *authzinfra.PolicyEngine) (*deployapp.DeploymentHistoryService, error) {
	repository, err := deployinfra.NewDeploymentHistoryRepository(db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewDeploymentHistoryService(repository, policy)
}

func newExecutionControlService(db *gorm.DB, policy *authzinfra.PolicyEngine, argo *argoinfra.Client) (*deployapp.ExecutionControlService, error) {
	repository, err := deployinfra.NewExecutionRepository(db)
	if err != nil {
		return nil, err
	}
	actualStates, err := deployinfra.NewArgoActualStateReader(argo)
	if err != nil {
		return nil, err
	}
	return deployapp.NewExecutionControlService(deployapp.ExecutionControlServiceOptions{
		Repository: repository, Authorizer: policy, ActualStates: actualStates,
		Terminator: argo, Clock: deployapp.SystemWorkflowClock{},
	})
}

func newDeploymentRequestService(db *gorm.DB, policy *authzinfra.PolicyEngine) (*deployapp.DeploymentRequestService, error) {
	repository, err := deployinfra.NewDeploymentRequestRepository(db)
	if err != nil {
		return nil, err
	}
	schedules, err := deployinfra.NewDeploymentScheduleRepository(db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewDeploymentRequestService(repository, schedules, policy, deployapp.SystemWorkflowClock{})
}

func newAPIWorkflowRuntimeService(db *gorm.DB, policy *authzinfra.PolicyEngine) (*deployapp.WorkflowRuntimeService, error) {
	repository, err := deployinfra.NewWorkflowRuntimeRepository(db)
	if err != nil {
		return nil, err
	}
	assignments, err := deployinfra.NewReviewAssignmentRepository(db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewWorkflowRuntimeService(deployapp.WorkflowRuntimeServiceOptions{
		Repository: repository, Authorizer: policy,
		Assignments: assignments, Clock: deployapp.SystemWorkflowClock{},
	})
}
