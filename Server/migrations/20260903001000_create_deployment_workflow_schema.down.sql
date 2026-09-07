DELETE FROM authorization_role_permissions
WHERE permission_key IN (
    'workflow.manage',
    'deployment_plan.manage',
    'deployment_request.view',
    'deployment_request.update',
    'deployment_request.review',
    'deployment_request.deploy',
    'deployment_request.retry',
    'deployment_request.terminate',
    'deployment_request.unlock',
    'deployment_history.view',
    'notification.view',
    'notification.mark_read'
);

DELETE FROM authorization_permissions
WHERE key IN (
    'workflow.manage',
    'deployment_plan.manage',
    'deployment_request.view',
    'deployment_request.update',
    'deployment_request.review',
    'deployment_request.deploy',
    'deployment_request.retry',
    'deployment_request.terminate',
    'deployment_request.unlock',
    'deployment_history.view',
    'notification.view',
    'notification.mark_read'
);

DROP TRIGGER IF EXISTS deployment_plan_versions_protect_published ON deployment_plan_versions;
DROP TRIGGER IF EXISTS release_workflow_versions_protect_published ON release_workflow_versions;
DROP FUNCTION IF EXISTS releasehub_protect_published_definition();
DROP TRIGGER IF EXISTS deployment_request_versions_protect_snapshot ON deployment_request_versions;
DROP FUNCTION IF EXISTS releasehub_protect_request_version_snapshot();
DROP TRIGGER IF EXISTS deployment_review_decisions_immutable ON deployment_review_decisions;
DROP TRIGGER IF EXISTS deployment_workflow_transitions_immutable ON deployment_workflow_transitions;
DROP TRIGGER IF EXISTS deployment_request_images_immutable ON deployment_request_images;
DROP TRIGGER IF EXISTS deployment_request_applications_immutable ON deployment_request_applications;
DROP FUNCTION IF EXISTS releasehub_reject_immutable_row_mutation();

DROP TABLE IF EXISTS deployment_notification_reads;
DROP TABLE IF EXISTS deployment_notifications;
DROP TABLE IF EXISTS deployment_application_locks;
DROP TABLE IF EXISTS deployment_jobs;
DROP TABLE IF EXISTS deployment_execution_nodes;
DROP TABLE IF EXISTS deployment_executions;
DROP TABLE IF EXISTS deployment_review_decisions;
DROP TABLE IF EXISTS deployment_review_tasks;
DROP TABLE IF EXISTS deployment_workflow_transitions;
DROP TABLE IF EXISTS deployment_workflow_instances;
DROP TABLE IF EXISTS deployment_request_images;
DROP TABLE IF EXISTS deployment_request_applications;
DROP TABLE IF EXISTS deployment_request_versions;
DROP TABLE IF EXISTS deployment_requests;
DROP TABLE IF EXISTS deployment_bindings;
DROP TABLE IF EXISTS deployment_plan_versions;
DROP TABLE IF EXISTS deployment_plans;
DROP TABLE IF EXISTS release_workflow_versions;
DROP TABLE IF EXISTS release_workflows;
