ALTER TABLE argocd_application_snapshots
ADD COLUMN argocd_project TEXT NOT NULL DEFAULT '';

CREATE TABLE application_onboardings (
    application_id UUID PRIMARY KEY REFERENCES applications(id),
    status TEXT NOT NULL CHECK (status IN (
        'ValidationFailed',
        'AwaitingConfirmation',
        'Applying',
        'Managed',
        'ConfigurationDrift'
    )),
    validation_issues JSONB NOT NULL DEFAULT '[]'::jsonb,
    validated_resource_version TEXT NOT NULL DEFAULT '',
    validated_at TIMESTAMPTZ NULL,
    confirmed_by UUID NULL REFERENCES users(id),
    confirmed_at TIMESTAMPTZ NULL,
    managed_at TIMESTAMPTZ NULL,
    drift_detected_at TIMESTAMPTZ NULL,
    drift_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_error_code TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX application_onboardings_status_idx
ON application_onboardings (status, updated_at);
