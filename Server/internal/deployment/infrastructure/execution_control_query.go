package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

const executionControlSelect = `SELECT request.id AS request_id, request.organization_id, request.project_id,
request.environment_id, request.classification, workflow.document AS workflow_document,
instance.id AS workflow_instance_id, instance.workflow_version_id AS workflow_instance_version_id,
instance.current_state_key AS workflow_state, instance.status AS workflow_status,
instance.lock_version AS workflow_lock_version, instance.started_at AS workflow_started_at,
instance.completed_at AS workflow_completed_at, execution.id AS execution_id,
execution.request_version_id, execution.plan_version_id, execution.attempt,
execution.status AS execution_status, execution.trigger_kind, execution.lock_version,
execution.created_at, execution.started_at, execution.completed_at
FROM deployment_executions execution
JOIN deployment_request_versions version ON version.id = execution.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
JOIN release_workflow_versions workflow ON workflow.id = version.workflow_version_id
JOIN deployment_workflow_instances instance ON instance.request_version_id = version.id`

// LoadByID loads one execution with its persisted authorization scope.
func (r *ExecutionRepository) LoadByID(ctx context.Context, executionID uuid.UUID) (deployapp.ExecutionControlSnapshot, error) {
	return r.loadControlSnapshot(ctx, executionControlSelect+" WHERE execution.id = ?", executionID)
}

// LoadLatest loads the latest attempt for the exact Request and Version path.
func (r *ExecutionRepository) LoadLatest(ctx context.Context, requestID, versionID uuid.UUID) (deployapp.ExecutionControlSnapshot, error) {
	query := executionControlSelect + " WHERE request.id = ? AND version.id = ? ORDER BY execution.attempt DESC LIMIT 1"
	return r.loadControlSnapshot(ctx, query, requestID, versionID)
}

func (r *ExecutionRepository) loadControlSnapshot(ctx context.Context, query string, values ...any) (deployapp.ExecutionControlSnapshot, error) {
	var source executionControlSourceModel
	if err := r.db.WithContext(ctx).Raw(query, values...).Scan(&source).Error; err != nil {
		return deployapp.ExecutionControlSnapshot{}, fmt.Errorf("load deployment execution control: %w", err)
	}
	if source.ExecutionID == uuid.Nil {
		return deployapp.ExecutionControlSnapshot{}, deployapp.ErrExecutionNotFound
	}
	return executionControlSnapshot(ctx, r.db, source)
}

func executionControlSnapshot(ctx context.Context, db *gorm.DB, source executionControlSourceModel) (deployapp.ExecutionControlSnapshot, error) {
	workflow, err := unmarshalWorkflowDocument(source.WorkflowDocument)
	if err != nil {
		return deployapp.ExecutionControlSnapshot{}, err
	}
	scope, err := authz.NewEnvironmentScope(source.OrganizationID, source.ProjectID, source.EnvironmentID)
	if err != nil {
		return deployapp.ExecutionControlSnapshot{}, err
	}
	nodes, err := loadControlNodes(ctx, db, source.ExecutionID)
	execution := executionControlFromModel(source, nodes)
	return deployapp.ExecutionControlSnapshot{RequestID: source.RequestID, Scope: scope,
		Workflow: workflow, WorkflowInstance: executionWorkflowInstance(source),
		Classification: source.Classification, Execution: execution}, err
}

func executionWorkflowInstance(source executionControlSourceModel) deploydomain.WorkflowInstance {
	return deploydomain.WorkflowInstance{
		ID: source.WorkflowInstanceID, RequestVersionID: source.RequestVersionID,
		WorkflowVersionID: source.WorkflowInstanceVersionID, CurrentStateKey: source.WorkflowState,
		Status: deploydomain.WorkflowInstanceStatus(source.WorkflowStatus), LockVersion: source.WorkflowLockVersion,
		StartedAt: source.WorkflowStartedAt, CompletedAt: source.WorkflowCompletedAt,
	}
}

func loadControlNodes(ctx context.Context, db *gorm.DB, executionID uuid.UUID) ([]deployapp.DeploymentExecutionNode, error) {
	var models []deploymentExecutionNodeModel
	if err := db.WithContext(ctx).Where("execution_id = ?", executionID).Order("node_key").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load deployment execution nodes: %w", err)
	}
	identities, err := loadControlIdentities(ctx, db, models)
	if err != nil {
		return nil, err
	}
	result := make([]deployapp.DeploymentExecutionNode, 0, len(models))
	for _, model := range models {
		identity := identities[model.ApplicationID]
		result = append(result, executionControlNode(model, argodomain.ApplicationIdentity{
			Namespace: identity.ArgoCDNamespace, Name: identity.ArgoCDApplicationName}, identity.ArgoCDProject))
	}
	return result, nil
}

func loadControlIdentities(ctx context.Context, db *gorm.DB, nodes []deploymentExecutionNodeModel) (map[uuid.UUID]executionApplicationModel, error) {
	ids := make([]uuid.UUID, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ApplicationID)
	}
	var models []executionApplicationModel
	if err := db.WithContext(ctx).Table("applications").Where("id IN ?", ids).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load deployment execution Application identities: %w", err)
	}
	result := make(map[uuid.UUID]executionApplicationModel, len(models))
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func executionControlNode(model deploymentExecutionNodeModel, identity argodomain.ApplicationIdentity, project string) deployapp.DeploymentExecutionNode {
	var images []deploydomain.DeploymentRequestImageSnapshot
	_ = json.Unmarshal(model.ActualImages, &images)
	return deployapp.DeploymentExecutionNode{
		ID: model.ID, RequestApplicationID: model.RequestApplicationID, ApplicationID: model.ApplicationID,
		NodeKey: model.NodeKey, Status: model.Status, OperationID: model.OperationID,
		SyncStatus: model.SyncStatus, HealthStatus: model.HealthStatus, ActualRevision: model.ActualRevision,
		ActualImages: images, ErrorCode: model.ErrorCode, ErrorMessage: model.ErrorMessage,
		Identity: identity, ArgoProject: project,
	}
}

func executionControlFromModel(source executionControlSourceModel, nodes []deployapp.DeploymentExecutionNode) deployapp.DeploymentExecution {
	return deployapp.DeploymentExecution{
		ID: source.ExecutionID, RequestVersionID: source.RequestVersionID, PlanVersionID: source.PlanVersionID,
		Attempt: source.Attempt, Status: source.ExecutionStatus, TriggerKind: source.TriggerKind,
		LockVersion: source.LockVersion, Nodes: nodes, CreatedAt: source.CreatedAt,
		StartedAt: source.StartedAt, CompletedAt: source.CompletedAt,
	}
}
