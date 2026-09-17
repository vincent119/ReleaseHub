ALTER TABLE audit_logs
    ADD COLUMN project_id UUID NULL,
    ADD COLUMN environment_id UUID NULL,
    ADD COLUMN application_id UUID NULL,
    ADD COLUMN actor_display_name TEXT NULL,
    ADD COLUMN scope_resolution TEXT NOT NULL DEFAULT 'unresolved',
    ADD CONSTRAINT audit_logs_scope_resolution_check
        CHECK (scope_resolution IN ('resolved', 'unresolved')),
    ADD CONSTRAINT audit_logs_scope_ancestry_check
        CHECK (
            (application_id IS NULL OR (environment_id IS NOT NULL AND project_id IS NOT NULL AND organization_id IS NOT NULL)) AND
            (environment_id IS NULL OR (project_id IS NOT NULL AND organization_id IS NOT NULL)) AND
            (project_id IS NULL OR organization_id IS NOT NULL)
        ),
    ADD CONSTRAINT audit_logs_actor_display_name_check
        CHECK (actor_display_name IS NULL OR length(btrim(actor_display_name)) BETWEEN 1 AND 255);

UPDATE audit_logs audit
SET actor_display_name = identity.username
FROM users identity
WHERE audit.actor_id = identity.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'organization'
)
UPDATE audit_logs audit
SET organization_id = organization.id,
    scope_resolution = 'resolved'
FROM parsed
JOIN organizations organization ON organization.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'project'
)
UPDATE audit_logs audit
SET organization_id = project.organization_id,
    project_id = project.id,
    scope_resolution = 'resolved'
FROM parsed
JOIN projects project ON project.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type IN ('environment', 'deployment_schedule')
)
UPDATE audit_logs audit
SET organization_id = environment.organization_id,
    project_id = environment.project_id,
    environment_id = environment.id,
    scope_resolution = 'resolved'
FROM parsed
JOIN environments environment ON environment.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type IN ('application', 'application_onboarding')
)
UPDATE audit_logs audit
SET organization_id = application.organization_id,
    project_id = application.project_id,
    environment_id = application.environment_id,
    application_id = application.id,
    scope_resolution = 'resolved'
FROM parsed
JOIN applications application ON application.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_binding'
)
UPDATE audit_logs audit
SET organization_id = binding.organization_id,
    project_id = binding.project_id,
    environment_id = binding.environment_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_bindings binding ON binding.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_request'
)
UPDATE audit_logs audit
SET organization_id = request.organization_id,
    project_id = request.project_id,
    environment_id = request.environment_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_requests request ON request.id = parsed.resource_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_request_version'
)
UPDATE audit_logs audit
SET organization_id = request.organization_id,
    project_id = request.project_id,
    environment_id = request.environment_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_request_versions version ON version.id = parsed.resource_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_review_task'
)
UPDATE audit_logs audit
SET organization_id = request.organization_id,
    project_id = request.project_id,
    environment_id = request.environment_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_review_tasks review ON review.id = parsed.resource_id
JOIN deployment_request_versions version ON version.id = review.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_execution'
)
UPDATE audit_logs audit
SET organization_id = request.organization_id,
    project_id = request.project_id,
    environment_id = request.environment_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_executions execution ON execution.id = parsed.resource_id
JOIN deployment_request_versions version ON version.id = execution.request_version_id
JOIN deployment_requests request ON request.id = version.request_id
WHERE audit.id = parsed.id;

WITH parsed AS (
    SELECT id, CASE
        WHEN resource_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        THEN resource_id::uuid
    END AS resource_id
    FROM audit_logs
    WHERE resource_type = 'deployment_plan'
)
UPDATE audit_logs audit
SET organization_id = project.organization_id,
    project_id = plan.owner_project_id,
    scope_resolution = 'resolved'
FROM parsed
JOIN deployment_plans plan ON plan.id = parsed.resource_id
LEFT JOIN projects project ON project.id = plan.owner_project_id
WHERE audit.id = parsed.id;

UPDATE audit_logs
SET scope_resolution = 'resolved'
WHERE resource_type IN ('release_workflow', 'user', 'argocd_application_candidate');

CREATE INDEX audit_logs_occurred_id_idx
ON audit_logs (occurred_at DESC, id DESC);

CREATE INDEX audit_logs_project_occurred_idx
ON audit_logs (project_id, occurred_at DESC, id DESC)
WHERE project_id IS NOT NULL;

CREATE INDEX audit_logs_environment_occurred_idx
ON audit_logs (environment_id, occurred_at DESC, id DESC)
WHERE environment_id IS NOT NULL;

CREATE INDEX audit_logs_application_occurred_idx
ON audit_logs (application_id, occurred_at DESC, id DESC)
WHERE application_id IS NOT NULL;

CREATE INDEX audit_logs_request_id_idx
ON audit_logs (request_id, occurred_at DESC, id DESC)
WHERE request_id IS NOT NULL;

INSERT INTO authorization_permissions (key, platform_only, project_role_delegable)
VALUES ('audit.view', false, true);

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000101', 'audit.view'),
    ('00000000-0000-0000-0000-000000000102', 'audit.view');

CREATE FUNCTION releasehub_reject_audit_log_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'audit logs are append-only';
END;
$$;

CREATE TRIGGER audit_logs_reject_mutation
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_audit_log_mutation();
