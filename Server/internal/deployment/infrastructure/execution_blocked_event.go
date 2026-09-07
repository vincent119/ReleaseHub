package infrastructure

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func appendExecutionBlockedEvidence(ctx context.Context, tx *gorm.DB, block deployapp.ExecutionBlock) error {
	metadata := map[string]any{
		"executionId": block.ExecutionID, "requestApplicationId": block.RequestApplicationID,
		"code": block.Code,
	}
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: block.OccurredAt, Action: "deployment_execution.block",
		ResourceType: "deployment_execution", ResourceID: block.ExecutionID.String(), Metadata: metadata,
	}); err != nil {
		return err
	}
	payload, _ := json.Marshal(metadata)
	event, err := platform.NewEvent("deployment.execution.blocked", "deployment_execution",
		block.ExecutionID.String(), payload, block.OccurredAt)
	if err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}
