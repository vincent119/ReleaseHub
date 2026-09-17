DROP TRIGGER IF EXISTS audit_logs_reject_mutation ON audit_logs;
DROP FUNCTION IF EXISTS releasehub_reject_audit_log_mutation();

DELETE FROM authorization_role_permissions
WHERE permission_key = 'audit.view';

DELETE FROM authorization_permissions
WHERE key = 'audit.view';

DROP INDEX IF EXISTS audit_logs_request_id_idx;
DROP INDEX IF EXISTS audit_logs_application_occurred_idx;
DROP INDEX IF EXISTS audit_logs_environment_occurred_idx;
DROP INDEX IF EXISTS audit_logs_project_occurred_idx;
DROP INDEX IF EXISTS audit_logs_occurred_id_idx;

ALTER TABLE audit_logs
    DROP CONSTRAINT IF EXISTS audit_logs_actor_display_name_check,
    DROP CONSTRAINT IF EXISTS audit_logs_scope_ancestry_check,
    DROP CONSTRAINT IF EXISTS audit_logs_scope_resolution_check,
    DROP COLUMN IF EXISTS scope_resolution,
    DROP COLUMN IF EXISTS actor_display_name,
    DROP COLUMN IF EXISTS application_id,
    DROP COLUMN IF EXISTS environment_id,
    DROP COLUMN IF EXISTS project_id;
