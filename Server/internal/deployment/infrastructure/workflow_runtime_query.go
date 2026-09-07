package infrastructure

const workflowRuntimeQuery = `
SELECT request.id AS request_id,
       version.id AS request_version_id,
       version.created_by AS request_creator_id,
       request.organization_id,
       request.project_id,
       request.environment_id,
       workflow.workflow_id,
       workflow.id AS workflow_version_id,
       workflow.version_number,
       workflow.lifecycle,
       workflow.document,
       workflow.lock_version AS version_lock,
       workflow.created_by AS version_created_by,
       workflow.created_at AS version_created_at,
       workflow.updated_at AS version_updated_at,
       workflow.published_at,
       workflow.disabled_at,
       instance.id AS instance_id,
       instance.current_state_key,
       instance.status AS instance_status,
       instance.lock_version AS instance_lock,
       instance.started_at,
       instance.completed_at
FROM deployment_request_versions version
JOIN deployment_requests request ON request.id = version.request_id
JOIN release_workflow_versions workflow ON workflow.id = version.workflow_version_id
LEFT JOIN deployment_workflow_instances instance ON instance.request_version_id = version.id
WHERE version.id = ?`
