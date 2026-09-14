package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
)

var workflowReferenceTables = []string{
	"deployment_bindings",
	"deployment_request_versions",
	"deployment_workflow_instances",
	"deployment_candidate_observations",
}

type workflowDeletion struct {
	mutation   deployapp.WorkflowMutation
	workflowID uuid.UUID
	expected   uint64
}

// DeleteUnusedDraft removes a workflow only when its latest version is current,
// every version is a draft, and no deployment record references any version.
func (r *WorkflowDefinitionRepository) DeleteUnusedDraft(ctx context.Context, mutation deployapp.WorkflowMutation, workflowID uuid.UUID, expected uint64) error {
	deletion := workflowDeletion{mutation: mutation, workflowID: workflowID, expected: expected}
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		return deleteUnusedDraftTransaction(ctx, tx, deletion)
	})
}

func deleteUnusedDraftTransaction(ctx context.Context, tx *gorm.DB, deletion workflowDeletion) error {
	if err := lockWorkflow(tx, deletion.workflowID); err != nil {
		return err
	}
	versions, err := deletableWorkflowVersions(tx, deletion.workflowID, deletion.expected)
	if err != nil {
		return err
	}
	if err := ensureWorkflowVersionsUnused(tx, versions); err != nil {
		return err
	}
	if err := appendWorkflowDeletion(ctx, tx, deletion, len(versions)); err != nil {
		return err
	}
	return deleteWorkflowRecords(tx, deletion.workflowID)
}

func deletableWorkflowVersions(tx *gorm.DB, workflowID uuid.UUID, expected uint64) ([]uuid.UUID, error) {
	var versions []releaseWorkflowVersionModel
	if err := tx.Select("id", "version_number", "lifecycle").
		Where("workflow_id = ?", workflowID).Order("version_number").Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("read workflow deletion eligibility: %w", err)
	}
	if len(versions) == 0 || versions[len(versions)-1].VersionNumber != expected {
		return nil, deployapp.ErrWorkflowConflict
	}
	return draftWorkflowVersionIDs(versions)
}

func draftWorkflowVersionIDs(versions []releaseWorkflowVersionModel) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(versions))
	for _, version := range versions {
		if version.Lifecycle != string(deploydomain.DefinitionDraft) {
			return nil, deployapp.ErrWorkflowConflict
		}
		ids = append(ids, version.ID)
	}
	return ids, nil
}

func ensureWorkflowVersionsUnused(tx *gorm.DB, versionIDs []uuid.UUID) error {
	for _, table := range workflowReferenceTables {
		var count int64
		if err := tx.Table(table).Where("workflow_version_id IN ?", versionIDs).Limit(1).Count(&count).Error; err != nil {
			return fmt.Errorf("check workflow references in %s: %w", table, err)
		}
		if count > 0 {
			return deployapp.ErrWorkflowConflict
		}
	}
	return nil
}

func appendWorkflowDeletion(ctx context.Context, tx *gorm.DB, deletion workflowDeletion, versionCount int) error {
	return appendWorkflowChange(ctx, tx, workflowChange{
		mutation: deletion.mutation, workflowID: deletion.workflowID, action: "deleted",
		metadata: map[string]any{"expectedVersion": deletion.expected, "versionCount": versionCount},
	})
}

func deleteWorkflowRecords(tx *gorm.DB, workflowID uuid.UUID) error {
	if err := tx.Where("workflow_id = ?", workflowID).Delete(&releaseWorkflowVersionModel{}).Error; err != nil {
		return workflowWriteError("delete release workflow versions", err)
	}
	result := tx.Delete(&releaseWorkflowModel{}, "id = ?", workflowID)
	if result.Error != nil {
		return workflowWriteError("delete release workflow", result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrWorkflowNotFound
	}
	return nil
}
