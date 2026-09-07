package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

type deploymentRequestEvent struct {
	observation      deploydomain.CandidateObservation
	requestID        uuid.UUID
	requestVersionID uuid.UUID
	classification   string
}

func appendDeploymentRequestCreated(ctx context.Context, tx *gorm.DB, value deploymentRequestEvent) error {
	payload, err := json.Marshal(map[string]any{
		"requestId": value.requestID, "requestVersionId": value.requestVersionID,
		"applicationId":  value.observation.ApplicationID,
		"targetRevision": value.observation.TargetRevision, "fingerprint": value.observation.Fingerprint,
		"classification": value.classification,
	})
	if err != nil {
		return fmt.Errorf("marshal deployment request event: %w", err)
	}
	event, err := platform.NewEvent("deployment.request.created", "deployment_request", value.requestID.String(), payload, value.observation.ObservedAt)
	if err != nil {
		return fmt.Errorf("create deployment request event: %w", err)
	}
	if err := appendDeploymentRequestAudit(ctx, tx, value); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func appendDeploymentRequestAudit(ctx context.Context, tx *gorm.DB, value deploymentRequestEvent) error {
	return database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: value.observation.ObservedAt, OrganizationID: &value.observation.OrganizationID,
		Action: "deployment_request.create", ResourceType: "deployment_request", ResourceID: value.requestID.String(),
		RequestID: "worker-candidate-reconciliation",
		Metadata: map[string]any{"requestVersionId": value.requestVersionID,
			"applicationId": value.observation.ApplicationID, "classification": value.classification},
	})
}
