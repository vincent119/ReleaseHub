CREATE TABLE onboarding_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID NOT NULL REFERENCES users(id),
    application_id UUID NOT NULL REFERENCES applications(id),
    operation TEXT NOT NULL CHECK (operation IN ('dry_run', 'confirm')),
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    request_fingerprint TEXT NOT NULL CHECK (request_fingerprint ~ '^[a-f0-9]{64}$'),
    state TEXT NOT NULL CHECK (state IN ('Accepted', 'Completed')),
    onboarding_status TEXT NOT NULL DEFAULT '',
    onboarding_version BIGINT NULL CHECK (onboarding_version IS NULL OR onboarding_version > 0),
    response_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    UNIQUE (actor_id, operation, idempotency_key)
);

CREATE INDEX onboarding_commands_application_accepted_idx
ON onboarding_commands (application_id, operation, updated_at)
WHERE state = 'Accepted';
