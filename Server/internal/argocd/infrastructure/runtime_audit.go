package infrastructure

import (
	"context"
	"errors"
	"time"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"gorm.io/gorm"
)

// RuntimeAuditWriter stores metadata-only evidence for sensitive runtime reads.
type RuntimeAuditWriter struct{ db *gorm.DB }

// NewRuntimeAuditWriter creates a runtime read auditor backed by the existing Audit Trail.
func NewRuntimeAuditWriter(db *gorm.DB) (*RuntimeAuditWriter, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &RuntimeAuditWriter{db: db}, nil
}

// RecordRuntimeRead appends one audit event without persisting manifest, event, or log content.
func (w *RuntimeAuditWriter) RecordRuntimeRead(ctx context.Context, value argoapp.RuntimeAuditRecord) error {
	actorID := value.ActorID
	return w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return database.AppendAudit(ctx, tx, database.AuditRecord{
			OccurredAt: time.Now().UTC(), ActorID: &actorID,
			OrganizationID: &value.Application.OrganizationID, ProjectID: &value.Application.ProjectID,
			EnvironmentID: &value.Application.EnvironmentID, ApplicationID: &value.Application.ID,
			Action: value.Action, ResourceType: "kubernetes_resource", ResourceID: value.Resource.Key(),
			RequestID: value.RequestID,
			Metadata: map[string]any{
				"group": value.Resource.Group, "version": value.Resource.Version,
				"kind": value.Resource.Kind, "namespace": value.Resource.Namespace, "name": value.Resource.Name,
			},
		})
	})
}
