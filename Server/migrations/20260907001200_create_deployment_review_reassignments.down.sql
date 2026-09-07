DROP INDEX IF EXISTS deployment_review_reassignments_task_idx;
DROP TABLE IF EXISTS deployment_review_reassignments;
DELETE FROM authorization_role_permissions
WHERE permission_key = 'deployment_request.reassign';
DELETE FROM authorization_permissions
WHERE key = 'deployment_request.reassign';
