package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

const matchingSuccessfulHistoryQuery = `
SELECT request.id AS request_id, version.id AS request_version_id,
       execution.id AS execution_id, request.classification, execution.completed_at
FROM deployment_executions execution
JOIN deployment_request_versions version ON version.id = execution.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE request.organization_id = ? AND request.project_id = ? AND request.environment_id = ?
  AND version.id <> ? AND execution.status = 'Succeeded'
  AND NOT EXISTS (
    SELECT 1 FROM deployment_request_applications target_application
    JOIN deployment_request_images target_image ON target_image.request_application_id = target_application.id
    WHERE target_application.request_version_id = ? AND NOT EXISTS (
      SELECT 1 FROM deployment_request_applications history_application
      JOIN deployment_request_images history_image ON history_image.request_application_id = history_application.id
      WHERE history_application.request_version_id = version.id
        AND history_application.application_id = target_application.application_id
        AND history_image.registry = target_image.registry AND history_image.repository = target_image.repository
        AND history_image.image_digest = target_image.image_digest))
  AND (SELECT COUNT(*) FROM deployment_request_applications WHERE request_version_id = version.id) =
      (SELECT COUNT(*) FROM deployment_request_applications WHERE request_version_id = ?)
ORDER BY execution.completed_at DESC`

type candidateClassification struct {
	ctx              context.Context
	tx               *gorm.DB
	requestID        uuid.UUID
	requestVersionID uuid.UUID
	observedAt       time.Time
}

type candidateClassificationEvidence struct {
	organizationID uuid.UUID
	current        []deploydomain.DeploymentRequestApplicationSnapshot
	history        []deploydomain.DeploymentHistoryItem
}

func classifyCandidateRequest(value candidateClassification) (string, error) {
	target, err := loadRequestApplications(value.ctx, value.tx, value.requestVersionID)
	if err != nil {
		return "", err
	}
	evidence, err := loadCandidateClassificationEvidence(value, target)
	if err != nil {
		return "", err
	}
	classification := deploydomain.ClassifyDeployment(target, evidence.current, evidence.history)
	if err := saveCandidateClassification(value, evidence.organizationID, classification); err != nil {
		return "", err
	}
	return classification, nil
}

func loadCandidateClassificationEvidence(value candidateClassification, target []deploydomain.DeploymentRequestApplicationSnapshot) (candidateClassificationEvidence, error) {
	scope, err := loadCandidateClassificationScope(value)
	if err != nil {
		return candidateClassificationEvidence{}, err
	}
	current, err := loadCurrentApplicationSnapshots(value.ctx, value.tx, target)
	if err != nil {
		return candidateClassificationEvidence{}, err
	}
	history, err := loadMatchingSuccessfulHistory(value, scope)
	return candidateClassificationEvidence{
		organizationID: scope.OrganizationID, current: current, history: history,
	}, err
}

type candidateClassificationScope struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
}

func loadCandidateClassificationScope(value candidateClassification) (candidateClassificationScope, error) {
	var scope candidateClassificationScope
	err := value.tx.WithContext(value.ctx).Table("deployment_requests").
		Select("organization_id, project_id, environment_id").Where("id = ?", value.requestID).Scan(&scope).Error
	if err != nil || scope.OrganizationID == uuid.Nil {
		return scope, fmt.Errorf("load candidate classification scope: %w", executionLookupError(err))
	}
	return scope, nil
}

func loadMatchingSuccessfulHistory(value candidateClassification, scope candidateClassificationScope) ([]deploydomain.DeploymentHistoryItem, error) {
	var rows []deploymentHistoryRow
	err := value.tx.WithContext(value.ctx).Raw(matchingSuccessfulHistoryQuery,
		scope.OrganizationID, scope.ProjectID, scope.EnvironmentID,
		value.requestVersionID, value.requestVersionID, value.requestVersionID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load matching successful deployment history: %w", err)
	}
	return loadDeploymentHistoryItems(value.ctx, value.tx, rows)
}

func loadCurrentApplicationSnapshots(ctx context.Context, tx *gorm.DB, target []deploydomain.DeploymentRequestApplicationSnapshot) ([]deploydomain.DeploymentRequestApplicationSnapshot, error) {
	result := make([]deploydomain.DeploymentRequestApplicationSnapshot, 0, len(target))
	for _, application := range target {
		current, err := loadCurrentApplicationSnapshot(ctx, tx, application.ApplicationID)
		if err != nil {
			return nil, err
		}
		result = append(result, current)
	}
	return result, nil
}

func loadCurrentApplicationSnapshot(ctx context.Context, tx *gorm.DB, applicationID uuid.UUID) (deploydomain.DeploymentRequestApplicationSnapshot, error) {
	var node deploymentExecutionNodeModel
	err := tx.WithContext(ctx).Where("application_id = ? AND actual_images <> '[]'::jsonb", applicationID).
		Order("updated_at DESC, id DESC").Limit(1).Find(&node).Error
	if err != nil {
		return deploydomain.DeploymentRequestApplicationSnapshot{}, fmt.Errorf("load current Application deployment state: %w", err)
	}
	return deploydomain.DeploymentRequestApplicationSnapshot{
		ApplicationID: applicationID, LiveRevision: node.ActualRevision, Images: historyActualImageSnapshots(node.ActualImages),
	}, nil
}

func saveCandidateClassification(value candidateClassification, organizationID uuid.UUID, classification string) error {
	result := value.tx.WithContext(value.ctx).Model(&deploymentRequestModel{}).
		Where("id = ?", value.requestID).Updates(map[string]any{"classification": classification, "updated_at": value.observedAt})
	if result.Error != nil || result.RowsAffected != 1 {
		return fmt.Errorf("save deployment request classification: %w", executionMutationError(result))
	}
	if classification != deploydomain.DeploymentClassificationForwardRollback {
		return nil
	}
	return appendForwardRollbackEvidence(value, organizationID)
}

func appendForwardRollbackEvidence(value candidateClassification, organizationID uuid.UUID) error {
	metadata := map[string]any{"requestVersionId": value.requestVersionID, "classification": deploydomain.DeploymentClassificationForwardRollback}
	if err := database.AppendAudit(value.ctx, value.tx, database.AuditRecord{
		OccurredAt: value.observedAt, OrganizationID: &organizationID,
		Action:       "deployment_request.forward_rollback_classified",
		ResourceType: "deployment_request", ResourceID: value.requestID.String(), Metadata: metadata,
	}); err != nil {
		return err
	}
	payload, _ := json.Marshal(metadata)
	event, err := platform.NewEvent("deployment.request.forward_rollback_classified", "deployment_request",
		value.requestID.String(), payload, value.observedAt)
	if err != nil {
		return err
	}
	return database.AppendOutbox(value.ctx, value.tx, event)
}
