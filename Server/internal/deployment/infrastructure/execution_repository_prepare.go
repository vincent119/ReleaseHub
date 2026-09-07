package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func lockExecutionVersion(ctx context.Context, tx *gorm.DB, versionID uuid.UUID) error {
	result := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, versionID.String())
	if result.Error != nil {
		return fmt.Errorf("lock deployment execution version: %w", result.Error)
	}
	return nil
}

func prepareExecution(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) (deployapp.ExecutionSnapshot, error) {
	model, exists, err := findExecution(ctx, tx, versionID)
	if err != nil {
		return deployapp.ExecutionSnapshot{}, err
	}
	if !exists {
		model, err = insertExecution(ctx, tx, versionID, now)
		if err != nil {
			return deployapp.ExecutionSnapshot{}, err
		}
	}
	return loadExecutionSnapshot(ctx, tx, model)
}

func findExecution(ctx context.Context, tx *gorm.DB, versionID uuid.UUID) (deploymentExecutionModel, bool, error) {
	var model deploymentExecutionModel
	err := tx.WithContext(ctx).Where("request_version_id = ? AND attempt = 1", versionID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deploymentExecutionModel{}, false, nil
	}
	if err != nil {
		return deploymentExecutionModel{}, false, fmt.Errorf("find deployment execution: %w", err)
	}
	return model, true, nil
}

func insertExecution(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) (deploymentExecutionModel, error) {
	version, err := loadExecutionVersion(ctx, tx, versionID)
	if err != nil {
		return deploymentExecutionModel{}, err
	}
	plan, encoded, err := loadExecutionPlan(ctx, tx, version.PlanVersionID)
	if err != nil {
		return deploymentExecutionModel{}, err
	}
	model, err := createExecution(ctx, tx, executionCreate{version: version, snapshot: encoded, now: now})
	if err != nil {
		return deploymentExecutionModel{}, err
	}
	input := executionNodeInsert{executionID: model.ID, versionID: versionID, plan: plan, now: now}
	if err := insertExecutionNodes(ctx, tx, input); err != nil {
		return deploymentExecutionModel{}, err
	}
	return model, markRequestDeploying(ctx, tx, versionID, now)
}

func loadExecutionVersion(ctx context.Context, tx *gorm.DB, versionID uuid.UUID) (deploymentRequestVersionModel, error) {
	var version deploymentRequestVersionModel
	if err := tx.WithContext(ctx).First(&version, "id = ?", versionID).Error; err != nil {
		return version, fmt.Errorf("load deployment Request Version: %w", err)
	}
	return version, nil
}

type executionCreate struct {
	version  deploymentRequestVersionModel
	snapshot datatypes.JSON
	now      time.Time
}

func createExecution(ctx context.Context, tx *gorm.DB, input executionCreate) (deploymentExecutionModel, error) {
	model := deploymentExecutionModel{
		ID: uuid.New(), RequestVersionID: input.version.ID, PlanVersionID: input.version.PlanVersionID,
		Attempt: 1, Status: "Preflight", TriggerKind: "Workflow", PlanSnapshot: input.snapshot,
		LockVersion: 1, CreatedAt: input.now, UpdatedAt: input.now,
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return model, fmt.Errorf("create deployment execution: %w", err)
	}
	return model, nil
}

func loadExecutionPlan(ctx context.Context, tx *gorm.DB, planID uuid.UUID) (deploydomain.DeploymentPlanDocument, datatypes.JSON, error) {
	var model deploymentPlanVersionModel
	if err := tx.WithContext(ctx).First(&model, "id = ?", planID).Error; err != nil {
		return deploydomain.DeploymentPlanDocument{}, nil, fmt.Errorf("load execution deployment Plan: %w", err)
	}
	document, err := unmarshalDeploymentPlanDocument(model.Document)
	if err != nil {
		return deploydomain.DeploymentPlanDocument{}, nil, err
	}
	encoded, err := marshalDeploymentPlanDocument(document)
	return document, encoded, err
}

type executionNodeInsert struct {
	executionID uuid.UUID
	versionID   uuid.UUID
	plan        deploydomain.DeploymentPlanDocument
	nodes       map[string]deploydomain.DeploymentPlanNode
	now         time.Time
}

func insertExecutionNodes(ctx context.Context, tx *gorm.DB, input executionNodeInsert) error {
	applications, err := loadRequestApplications(ctx, tx, input.versionID)
	if err != nil {
		return err
	}
	input.nodes = planNodesByApplication(input.plan)
	for _, application := range applications {
		if err := createExecutionNode(ctx, tx, input, application); err != nil {
			return err
		}
	}
	return nil
}

func createExecutionNode(ctx context.Context, tx *gorm.DB, input executionNodeInsert, application deploydomain.DeploymentRequestApplicationSnapshot) error {
	node, exists := input.nodes[application.ApplicationKey]
	if !exists {
		return errors.New("request Application is not defined by deployment Plan")
	}
	model := deploymentExecutionNodeModel{
		ID: uuid.New(), ExecutionID: input.executionID, RequestApplicationID: application.ID,
		ApplicationID: application.ApplicationID, NodeKey: node.Key,
		Status: "Waiting", ActualImages: datatypes.JSON("[]"), UpdatedAt: input.now,
	}
	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create deployment execution node: %w", err)
	}
	return nil
}

func markRequestDeploying(ctx context.Context, tx *gorm.DB, versionID uuid.UUID, now time.Time) error {
	result := tx.WithContext(ctx).Model(&deploymentRequestVersionModel{}).
		Where("id = ? AND status NOT IN ?", versionID, []string{"Superseded", "Terminated", "Succeeded", "Failed"}).
		Updates(map[string]any{"status": "Deploying", "lock_version": gorm.Expr("lock_version + 1"), "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("mark deployment Request Version deploying: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("deployment Request Version cannot start")
	}
	return nil
}

func loadExecutionSnapshot(ctx context.Context, tx *gorm.DB, model deploymentExecutionModel) (deployapp.ExecutionSnapshot, error) {
	plan, err := unmarshalDeploymentPlanDocument(model.PlanSnapshot)
	if err != nil {
		return deployapp.ExecutionSnapshot{}, err
	}
	applications, err := loadRequestApplications(ctx, tx, model.RequestVersionID)
	if err != nil {
		return deployapp.ExecutionSnapshot{}, err
	}
	targets, err := loadExecutionTargets(ctx, tx, executionTargetLoad{
		executionID: model.ID, applications: applications, plan: plan,
	})
	return deployapp.ExecutionSnapshot{
		ID: model.ID, RequestVersionID: model.RequestVersionID, Status: model.Status,
		Plan: plan, Targets: targets,
	}, err
}

type executionTargetLoad struct {
	executionID  uuid.UUID
	applications []deploydomain.DeploymentRequestApplicationSnapshot
	plan         deploydomain.DeploymentPlanDocument
}

func loadExecutionTargets(ctx context.Context, tx *gorm.DB, input executionTargetLoad) ([]deployapp.ExecutionTarget, error) {
	identities, err := loadExecutionIdentities(ctx, tx, input.applications)
	if err != nil {
		return nil, err
	}
	nodes := planNodesByApplication(input.plan)
	runtime, err := loadExecutionNodeRuntime(ctx, tx, input.executionID)
	if err != nil {
		return nil, err
	}
	result := make([]deployapp.ExecutionTarget, 0, len(input.applications))
	for _, application := range input.applications {
		result = append(result, executionTargetFromModels(application, identities, nodes, runtime))
	}
	return result, nil
}

func executionTargetFromModels(application deploydomain.DeploymentRequestApplicationSnapshot, identities map[uuid.UUID]executionApplicationModel, nodes map[string]deploydomain.DeploymentPlanNode, runtime map[uuid.UUID]deploymentExecutionNodeModel) deployapp.ExecutionTarget {
	identity := identities[application.ApplicationID]
	node := runtime[application.ApplicationID]
	return deployapp.ExecutionTarget{
		Node: nodes[application.ApplicationKey], Status: node.Status, OperationID: node.OperationID,
		Preflight: deployapp.PreflightTarget{Snapshot: application,
			Identity: argodomain.ApplicationIdentity{
				Namespace: identity.ArgoCDNamespace, Name: identity.ArgoCDApplicationName,
			}, ArgoProject: identity.ArgoCDProject},
	}
}

func loadExecutionIdentities(ctx context.Context, tx *gorm.DB, applications []deploydomain.DeploymentRequestApplicationSnapshot) (map[uuid.UUID]executionApplicationModel, error) {
	ids := make([]uuid.UUID, 0, len(applications))
	for _, application := range applications {
		ids = append(ids, application.ApplicationID)
	}
	var models []executionApplicationModel
	if err := tx.WithContext(ctx).Table("applications").Where("id IN ?", ids).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load execution Application identities: %w", err)
	}
	result := make(map[uuid.UUID]executionApplicationModel, len(models))
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func loadExecutionNodeRuntime(ctx context.Context, tx *gorm.DB, executionID uuid.UUID) (map[uuid.UUID]deploymentExecutionNodeModel, error) {
	var models []deploymentExecutionNodeModel
	if err := tx.WithContext(ctx).Where("execution_id = ?", executionID).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("load execution node runtime: %w", err)
	}
	result := make(map[uuid.UUID]deploymentExecutionNodeModel, len(models))
	for _, model := range models {
		result[model.ApplicationID] = model
	}
	return result, nil
}

func planNodesByApplication(plan deploydomain.DeploymentPlanDocument) map[string]deploydomain.DeploymentPlanNode {
	result := make(map[string]deploydomain.DeploymentPlanNode, len(plan.Nodes))
	for _, node := range plan.Nodes {
		result[node.ApplicationKey] = node
	}
	return result
}
