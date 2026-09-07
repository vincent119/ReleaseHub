CREATE TABLE deployment_review_reassignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_task_id UUID NOT NULL REFERENCES deployment_review_tasks(id),
    actor_id UUID NOT NULL REFERENCES users(id),
    previous_assignee_snapshot JSONB NOT NULL CHECK (jsonb_typeof(previous_assignee_snapshot) = 'object'),
    assignee_snapshot JSONB NOT NULL CHECK (jsonb_typeof(assignee_snapshot) = 'object'),
    reason TEXT NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 10000),
    permission_snapshot JSONB NOT NULL CHECK (jsonb_typeof(permission_snapshot) = 'object'),
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    reassigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (review_task_id, idempotency_key)
);

CREATE INDEX deployment_review_reassignments_task_idx
ON deployment_review_reassignments (review_task_id, reassigned_at);

CREATE TRIGGER deployment_review_reassignments_immutable
BEFORE UPDATE OR DELETE ON deployment_review_reassignments
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();

INSERT INTO authorization_permissions (key, platform_only, project_role_delegable)
VALUES ('deployment_request.reassign', false, false);

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000100', 'deployment_request.reassign'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_request.reassign');
