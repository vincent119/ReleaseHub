CREATE TABLE deployment_schedule_policies (
    environment_id UUID PRIMARY KEY REFERENCES environments(id),
    enabled BOOLEAN NOT NULL,
    time_zone TEXT NOT NULL CHECK (length(btrim(time_zone)) BETWEEN 1 AND 255),
    weekly_windows JSONB NOT NULL CHECK (jsonb_typeof(weekly_windows) = 'array'),
    blackouts JSONB NOT NULL CHECK (jsonb_typeof(blackouts) = 'array'),
    version BIGINT NOT NULL CHECK (version > 0),
    created_by UUID NOT NULL REFERENCES users(id),
    updated_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE deployment_schedule_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID NOT NULL REFERENCES users(id),
    environment_id UUID NOT NULL REFERENCES environments(id),
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    request_fingerprint TEXT NOT NULL CHECK (request_fingerprint ~ '^[a-f0-9]{64}$'),
    response_policy JSONB NOT NULL CHECK (jsonb_typeof(response_policy) = 'object'),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (actor_id, idempotency_key)
);

INSERT INTO authorization_permissions (key, platform_only, project_role_delegable)
VALUES ('deployment_schedule.manage', false, false);

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000100', 'deployment_schedule.manage'),
    ('00000000-0000-0000-0000-000000000101', 'deployment_schedule.manage'),
    ('00000000-0000-0000-0000-000000000102', 'deployment_schedule.manage');
