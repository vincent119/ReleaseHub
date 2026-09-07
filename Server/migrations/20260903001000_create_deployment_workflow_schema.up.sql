CREATE TABLE release_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    description TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX release_workflows_name_idx ON release_workflows (lower(name));

CREATE TABLE release_workflow_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES release_workflows(id),
    version_number BIGINT NOT NULL CHECK (version_number > 0),
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('Draft', 'Published', 'Disabled')),
    document JSONB NOT NULL CHECK (jsonb_typeof(document) = 'object'),
    lock_version BIGINT NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    created_by UUID NOT NULL REFERENCES users(id),
    published_at TIMESTAMPTZ NULL,
    disabled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, version_number),
    CHECK (
        (lifecycle = 'Draft' AND published_at IS NULL AND disabled_at IS NULL) OR
        (lifecycle = 'Published' AND published_at IS NOT NULL AND disabled_at IS NULL) OR
        (lifecycle = 'Disabled' AND published_at IS NOT NULL AND disabled_at IS NOT NULL)
    )
);

CREATE INDEX release_workflow_versions_lifecycle_idx
ON release_workflow_versions (workflow_id, lifecycle, version_number DESC);

CREATE TABLE deployment_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('platform', 'project')),
    owner_project_id UUID NULL REFERENCES projects(id),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    description TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (owner_kind = 'platform' AND owner_project_id IS NULL) OR
        (owner_kind = 'project' AND owner_project_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX deployment_plans_owner_name_idx
ON deployment_plans (
    owner_kind,
    COALESCE(owner_project_id, '00000000-0000-0000-0000-000000000000'::uuid),
    lower(name)
);

CREATE TABLE deployment_plan_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES deployment_plans(id),
    version_number BIGINT NOT NULL CHECK (version_number > 0),
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('Draft', 'Published', 'Disabled')),
    document JSONB NOT NULL CHECK (jsonb_typeof(document) = 'object'),
    lock_version BIGINT NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    created_by UUID NOT NULL REFERENCES users(id),
    published_at TIMESTAMPTZ NULL,
    disabled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, version_number),
    CHECK (
        (lifecycle = 'Draft' AND published_at IS NULL AND disabled_at IS NULL) OR
        (lifecycle = 'Published' AND published_at IS NOT NULL AND disabled_at IS NULL) OR
        (lifecycle = 'Disabled' AND published_at IS NOT NULL AND disabled_at IS NOT NULL)
    )
);

CREATE INDEX deployment_plan_versions_lifecycle_idx
ON deployment_plan_versions (plan_id, lifecycle, version_number DESC);

CREATE TABLE deployment_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    project_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    workflow_version_id UUID NOT NULL REFERENCES release_workflow_versions(id),
    plan_version_id UUID NOT NULL REFERENCES deployment_plan_versions(id),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    active BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (environment_id, project_id, organization_id)
        REFERENCES environments(id, project_id, organization_id)
);

CREATE UNIQUE INDEX deployment_bindings_active_environment_idx
ON deployment_bindings (environment_id)
WHERE active;

CREATE TABLE deployment_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    project_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    classification TEXT NOT NULL DEFAULT 'Standard'
        CHECK (classification IN ('Standard', 'ForwardRollback')),
    status TEXT NOT NULL DEFAULT 'Open' CHECK (status IN ('Open', 'Closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (environment_id, project_id, organization_id)
        REFERENCES environments(id, project_id, organization_id)
);

CREATE INDEX deployment_requests_scope_status_idx
ON deployment_requests (organization_id, project_id, environment_id, status, created_at DESC);

CREATE TABLE deployment_request_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES deployment_requests(id),
    version_number BIGINT NOT NULL CHECK (version_number > 0),
    status TEXT NOT NULL CHECK (status IN (
        'Candidate',
        'PendingReview',
        'Approved',
        'Deploying',
        'Succeeded',
        'Failed',
        'PartialFailed',
        'Blocked',
        'Superseded',
        'Terminated'
    )),
    fingerprint TEXT NOT NULL CHECK (length(btrim(fingerprint)) > 0),
    workflow_version_id UUID NOT NULL REFERENCES release_workflow_versions(id),
    plan_version_id UUID NOT NULL REFERENCES deployment_plan_versions(id),
    title TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 255),
    change_description TEXT NOT NULL DEFAULT '',
    issue_url TEXT NOT NULL DEFAULT '',
    scheduled_for TIMESTAMPTZ NULL,
    source_snapshot JSONB NOT NULL CHECK (jsonb_typeof(source_snapshot) = 'object'),
    lock_version BIGINT NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    created_by UUID NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, version_number),
    UNIQUE (request_id, fingerprint)
);

CREATE INDEX deployment_request_versions_status_idx
ON deployment_request_versions (status, created_at DESC);

CREATE UNIQUE INDEX deployment_request_versions_active_idx
ON deployment_request_versions (request_id)
WHERE status NOT IN ('Succeeded', 'Failed', 'Superseded', 'Terminated');

CREATE TABLE deployment_request_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_version_id UUID NOT NULL REFERENCES deployment_request_versions(id),
    application_id UUID NOT NULL REFERENCES applications(id),
    application_key TEXT NOT NULL CHECK (length(btrim(application_key)) BETWEEN 1 AND 255),
    live_revision TEXT NOT NULL DEFAULT '',
    target_revision TEXT NOT NULL CHECK (length(btrim(target_revision)) > 0),
    target_revisions JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(target_revisions) = 'array'),
    manifest_hash TEXT NOT NULL CHECK (length(btrim(manifest_hash)) > 0),
    diff_hash TEXT NOT NULL CHECK (length(btrim(diff_hash)) > 0),
    diff_snapshot JSONB NOT NULL CHECK (jsonb_typeof(diff_snapshot) IN ('object', 'array')),
    source_snapshot JSONB NOT NULL CHECK (jsonb_typeof(source_snapshot) = 'object'),
    execution_order INTEGER NOT NULL DEFAULT 0 CHECK (execution_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_version_id, application_id),
    UNIQUE (request_version_id, application_key)
);

CREATE INDEX deployment_request_applications_application_idx
ON deployment_request_applications (application_id, created_at DESC);

CREATE TABLE deployment_request_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_application_id UUID NOT NULL REFERENCES deployment_request_applications(id),
    image_reference TEXT NOT NULL CHECK (length(btrim(image_reference)) > 0),
    registry TEXT NOT NULL CHECK (length(btrim(registry)) > 0),
    repository TEXT NOT NULL CHECK (length(btrim(repository)) > 0),
    image_tag TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL CHECK (image_digest ~ '^sha256:[a-f0-9]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_application_id, image_reference)
);

CREATE TABLE deployment_workflow_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_version_id UUID NOT NULL UNIQUE REFERENCES deployment_request_versions(id),
    workflow_version_id UUID NOT NULL REFERENCES release_workflow_versions(id),
    current_state_key TEXT NOT NULL CHECK (length(btrim(current_state_key)) BETWEEN 1 AND 128),
    status TEXT NOT NULL CHECK (status IN ('Running', 'Completed', 'Rejected', 'Blocked', 'Superseded')),
    lock_version BIGINT NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'Running' AND completed_at IS NULL) OR (status <> 'Running' AND completed_at IS NOT NULL))
);

CREATE TABLE deployment_workflow_transitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID NOT NULL REFERENCES deployment_workflow_instances(id),
    request_version_id UUID NOT NULL REFERENCES deployment_request_versions(id),
    from_state_key TEXT NOT NULL,
    to_state_key TEXT NOT NULL,
    transition_key TEXT NOT NULL CHECK (length(btrim(transition_key)) BETWEEN 1 AND 128),
    actor_id UUID NULL REFERENCES users(id),
    reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_instance_id, idempotency_key)
);

CREATE TABLE deployment_review_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID NOT NULL REFERENCES deployment_workflow_instances(id),
    request_version_id UUID NOT NULL REFERENCES deployment_request_versions(id),
    state_key TEXT NOT NULL CHECK (length(btrim(state_key)) BETWEEN 1 AND 128),
    stage_number INTEGER NOT NULL CHECK (stage_number > 0),
    policy_type TEXT NOT NULL CHECK (policy_type IN (
        'AnyApprover',
        'MinimumApprovals',
        'AllApprovers',
        'RoleMinimumOne',
        'SequentialStages'
    )),
    required_approvals INTEGER NOT NULL DEFAULT 1 CHECK (required_approvals > 0),
    allow_self_review BOOLEAN NOT NULL DEFAULT false,
    assignee_snapshot JSONB NOT NULL CHECK (jsonb_typeof(assignee_snapshot) = 'object'),
    status TEXT NOT NULL CHECK (status IN ('Pending', 'Approved', 'Rejected', 'ReassignmentRequired', 'Closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ NULL,
    UNIQUE (workflow_instance_id, state_key, stage_number),
    CHECK (
        (status IN ('Pending', 'ReassignmentRequired') AND closed_at IS NULL) OR
        (status NOT IN ('Pending', 'ReassignmentRequired') AND closed_at IS NOT NULL)
    )
);

CREATE INDEX deployment_review_tasks_pending_idx
ON deployment_review_tasks (status, created_at)
WHERE status IN ('Pending', 'ReassignmentRequired');

CREATE TABLE deployment_review_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_task_id UUID NOT NULL REFERENCES deployment_review_tasks(id),
    reviewer_id UUID NOT NULL REFERENCES users(id),
    decision TEXT NOT NULL CHECK (decision IN ('Approve', 'Reject')),
    reason TEXT NOT NULL DEFAULT '',
    permission_snapshot JSONB NOT NULL CHECK (jsonb_typeof(permission_snapshot) = 'object'),
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (review_task_id, reviewer_id),
    UNIQUE (review_task_id, idempotency_key),
    CHECK (decision <> 'Reject' OR length(btrim(reason)) > 0)
);

CREATE TABLE deployment_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_version_id UUID NOT NULL REFERENCES deployment_request_versions(id),
    plan_version_id UUID NOT NULL REFERENCES deployment_plan_versions(id),
    attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN (
        'Queued',
        'Preflight',
        'Running',
        'Succeeded',
        'Failed',
        'PartialFailed',
        'Blocked',
        'Terminated'
    )),
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('Workflow', 'Retry')),
    triggered_by UUID NULL REFERENCES users(id),
    plan_snapshot JSONB NOT NULL CHECK (jsonb_typeof(plan_snapshot) = 'object'),
    lock_version BIGINT NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_version_id, attempt)
);

CREATE INDEX deployment_executions_status_idx
ON deployment_executions (status, created_at);

CREATE TABLE deployment_execution_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES deployment_executions(id),
    request_application_id UUID NOT NULL REFERENCES deployment_request_applications(id),
    application_id UUID NOT NULL REFERENCES applications(id),
    node_key TEXT NOT NULL CHECK (length(btrim(node_key)) BETWEEN 1 AND 255),
    status TEXT NOT NULL CHECK (status IN (
        'Waiting',
        'Queued',
        'Preflight',
        'Syncing',
        'Stabilizing',
        'Succeeded',
        'Failed',
        'Blocked',
        'Terminated',
        'Skipped'
    )),
    operation_id TEXT NOT NULL DEFAULT '',
    sync_status TEXT NOT NULL DEFAULT '',
    health_status TEXT NOT NULL DEFAULT '',
    actual_revision TEXT NOT NULL DEFAULT '',
    actual_images JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(actual_images) = 'array'),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (execution_id, application_id),
    UNIQUE (execution_id, node_key)
);

CREATE INDEX deployment_execution_nodes_status_idx
ON deployment_execution_nodes (execution_id, status);

CREATE TABLE deployment_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL CHECK (job_type IN (
        'ObserveApplication',
        'CreateRequestVersion',
        'AdvanceWorkflow',
        'ExecuteDeployment',
        'ReconcileExecution',
        'ProjectNotification',
        'ExpireNotification'
    )),
    aggregate_type TEXT NOT NULL CHECK (length(btrim(aggregate_type)) BETWEEN 1 AND 128),
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status TEXT NOT NULL DEFAULT 'Pending'
        CHECK (status IN ('Pending', 'Running', 'Succeeded', 'Failed', 'Cancelled')),
    idempotency_key TEXT NOT NULL UNIQUE CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    fencing_token BIGINT NOT NULL DEFAULT 0 CHECK (fencing_token >= 0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (status = 'Running' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL AND fencing_token > 0) OR
        (status <> 'Running' AND lease_owner IS NULL AND lease_expires_at IS NULL)
    )
);

CREATE INDEX deployment_jobs_claim_idx
ON deployment_jobs (available_at, created_at)
WHERE status = 'Pending';

CREATE INDEX deployment_jobs_expired_lease_idx
ON deployment_jobs (lease_expires_at)
WHERE status = 'Running';

CREATE TABLE deployment_application_locks (
    application_id UUID PRIMARY KEY REFERENCES applications(id),
    execution_id UUID NOT NULL REFERENCES deployment_executions(id),
    owner_token UUID NOT NULL,
    fencing_token BIGINT NOT NULL CHECK (fencing_token > 0),
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (execution_id, application_id)
);

CREATE INDEX deployment_application_locks_execution_idx
ON deployment_application_locks (execution_id);

CREATE TABLE deployment_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id UUID NOT NULL REFERENCES users(id),
    organization_id UUID NULL REFERENCES organizations(id),
    project_id UUID NULL REFERENCES projects(id),
    environment_id UUID NULL REFERENCES environments(id),
    application_id UUID NULL REFERENCES applications(id),
    event_type TEXT NOT NULL CHECK (length(btrim(event_type)) BETWEEN 1 AND 128),
    resource_type TEXT NOT NULL CHECK (length(btrim(resource_type)) BETWEEN 1 AND 128),
    resource_id UUID NOT NULL,
    event_id UUID NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > occurred_at)
);

CREATE INDEX deployment_notifications_recipient_idx
ON deployment_notifications (recipient_id, occurred_at DESC);

CREATE INDEX deployment_notifications_expiry_idx
ON deployment_notifications (expires_at);

CREATE TABLE deployment_notification_reads (
    notification_id UUID NOT NULL REFERENCES deployment_notifications(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (notification_id, user_id)
);

CREATE FUNCTION releasehub_reject_immutable_row_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable deployment record cannot be changed';
END;
$$;

CREATE TRIGGER deployment_request_applications_immutable
BEFORE UPDATE OR DELETE ON deployment_request_applications
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();

CREATE TRIGGER deployment_request_images_immutable
BEFORE UPDATE OR DELETE ON deployment_request_images
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();

CREATE TRIGGER deployment_workflow_transitions_immutable
BEFORE UPDATE OR DELETE ON deployment_workflow_transitions
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();

CREATE TRIGGER deployment_review_decisions_immutable
BEFORE UPDATE OR DELETE ON deployment_review_decisions
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();

CREATE FUNCTION releasehub_protect_request_version_snapshot() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(
        NEW.request_id,
        NEW.version_number,
        NEW.fingerprint,
        NEW.workflow_version_id,
        NEW.plan_version_id,
        NEW.title,
        NEW.change_description,
        NEW.issue_url,
        NEW.scheduled_for,
        NEW.source_snapshot,
        NEW.created_by,
        NEW.created_at
    ) IS DISTINCT FROM ROW(
        OLD.request_id,
        OLD.version_number,
        OLD.fingerprint,
        OLD.workflow_version_id,
        OLD.plan_version_id,
        OLD.title,
        OLD.change_description,
        OLD.issue_url,
        OLD.scheduled_for,
        OLD.source_snapshot,
        OLD.created_by,
        OLD.created_at
    ) THEN
        RAISE EXCEPTION 'deployment request version snapshot cannot be changed';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER deployment_request_versions_protect_snapshot
BEFORE UPDATE ON deployment_request_versions
FOR EACH ROW EXECUTE FUNCTION releasehub_protect_request_version_snapshot();

CREATE FUNCTION releasehub_protect_published_definition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.lifecycle <> 'Draft' AND ROW(
        NEW.version_number,
        NEW.document,
        NEW.created_by,
        NEW.created_at
    ) IS DISTINCT FROM ROW(
        OLD.version_number,
        OLD.document,
        OLD.created_by,
        OLD.created_at
    ) THEN
        RAISE EXCEPTION 'published definition cannot be changed';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER release_workflow_versions_protect_published
BEFORE UPDATE ON release_workflow_versions
FOR EACH ROW EXECUTE FUNCTION releasehub_protect_published_definition();

CREATE TRIGGER deployment_plan_versions_protect_published
BEFORE UPDATE ON deployment_plan_versions
FOR EACH ROW EXECUTE FUNCTION releasehub_protect_published_definition();

INSERT INTO authorization_permissions (key, platform_only, project_role_delegable) VALUES
    ('workflow.manage', true, false),
    ('deployment_plan.manage', false, false),
    ('deployment_request.view', false, true),
    ('deployment_request.update', false, true),
    ('deployment_request.review', false, true),
    ('deployment_request.deploy', false, true),
    ('deployment_request.retry', false, true),
    ('deployment_request.terminate', false, true),
    ('deployment_request.unlock', false, false),
    ('deployment_history.view', false, true),
    ('notification.view', false, true),
    ('notification.mark_read', false, true);

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000100', 'workflow.manage'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_plan.manage'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.update'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.review'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.deploy'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.retry'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.terminate'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.unlock'),
    ('00000000-0000-0000-0000-000000000100', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000100', 'notification.view'),
    ('00000000-0000-0000-0000-000000000100', 'notification.mark_read'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_plan.manage'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.update'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.review'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.deploy'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.retry'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.terminate'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.unlock'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000101', 'notification.view'),
    ('00000000-0000-0000-0000-000000000101', 'notification.mark_read'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.update'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.review'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.deploy'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.retry'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_request.terminate'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000102', 'notification.view'),
    ('00000000-0000-0000-0000-000000000102', 'notification.mark_read'),
    ('00000000-0000-0000-0000-000000000103', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000103', 'deployment_request.update'),
    ('00000000-0000-0000-0000-000000000103', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000103', 'notification.view'),
    ('00000000-0000-0000-0000-000000000103', 'notification.mark_read'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_request.update'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_request.deploy'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_request.retry'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_request.terminate'),
    ('00000000-0000-0000-0000-000000000104', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000104', 'notification.view'),
    ('00000000-0000-0000-0000-000000000104', 'notification.mark_read'),
    ('00000000-0000-0000-0000-000000000105', 'deployment_request.view'),
    ('00000000-0000-0000-0000-000000000105', 'deployment_history.view'),
    ('00000000-0000-0000-0000-000000000105', 'notification.view'),
    ('00000000-0000-0000-0000-000000000105', 'notification.mark_read');
