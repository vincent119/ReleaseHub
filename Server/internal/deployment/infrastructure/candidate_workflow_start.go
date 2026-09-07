package infrastructure

import (
	"context"
	"fmt"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

const pendingWorkflowStartsQuery = `
SELECT request.id AS request_id,
       version.id AS request_version_id
FROM deployment_request_versions version
JOIN deployment_requests request ON request.id = version.request_id
JOIN release_workflow_versions workflow ON workflow.id = version.workflow_version_id
LEFT JOIN deployment_workflow_instances instance ON instance.request_version_id = version.id
WHERE instance.id IS NULL
  AND workflow.lifecycle = 'Published'
  AND version.status NOT IN ('Succeeded', 'Failed', 'Superseded', 'Terminated')
ORDER BY version.created_at, version.id`

// ListPendingWorkflowStarts returns active Request Versions missing their pinned Workflow instance.
func (r *CandidateRepository) ListPendingWorkflowStarts(ctx context.Context) ([]deployapp.PendingWorkflowStart, error) {
	var values []deployapp.PendingWorkflowStart
	if err := r.db.WithContext(ctx).Raw(pendingWorkflowStartsQuery).Scan(&values).Error; err != nil {
		return nil, fmt.Errorf("list pending deployment Workflow starts: %w", err)
	}
	return values, nil
}
