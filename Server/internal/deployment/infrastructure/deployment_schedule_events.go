package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

func deploymentScheduleFingerprint(expected uint64, document deploymentSchedulePolicyDocument) (string, error) {
	payload, err := json.Marshal(struct {
		ExpectedVersion uint64                           `json:"expectedVersion"`
		Policy          deploymentSchedulePolicyDocument `json:"policy"`
	}{ExpectedVersion: expected, Policy: document})
	if err != nil {
		return "", fmt.Errorf("marshal deployment schedule fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func appendDeploymentScheduleChange(ctx context.Context, tx *gorm.DB, mutation deployapp.DeploymentScheduleMutation, policy deploydomain.DeploymentSchedulePolicy) error {
	metadata := deploymentScheduleMetadata(policy)
	event, err := newDeploymentScheduleEvent(policy, mutation, metadata)
	if err != nil {
		return err
	}
	actorID, organizationID := mutation.ActorID, mutation.OrganizationID
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: mutation.OccurredAt, ActorID: &actorID, OrganizationID: &organizationID,
		Action: "deployment_schedule.updated", ResourceType: "deployment_schedule",
		ResourceID: policy.EnvironmentID.String(), RequestID: mutation.RequestID, Metadata: metadata,
	}); err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

func deploymentScheduleMetadata(policy deploydomain.DeploymentSchedulePolicy) map[string]any {
	return map[string]any{
		"version": policy.Version, "weeklyWindowCount": len(policy.WeeklyWindows), "blackoutCount": len(policy.Blackouts),
	}
}

func newDeploymentScheduleEvent(policy deploydomain.DeploymentSchedulePolicy, mutation deployapp.DeploymentScheduleMutation, metadata map[string]any) (platform.Event, error) {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return platform.Event{}, fmt.Errorf("marshal deployment schedule event: %w", err)
	}
	event, err := platform.NewEvent("deployment_schedule.updated", "deployment_schedule", policy.EnvironmentID.String(), payload, mutation.OccurredAt)
	if err != nil {
		return platform.Event{}, fmt.Errorf("create deployment schedule event: %w", err)
	}
	return event, nil
}
