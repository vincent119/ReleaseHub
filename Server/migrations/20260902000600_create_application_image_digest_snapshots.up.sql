CREATE TABLE application_image_digest_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id),
    manifest_revision TEXT NOT NULL CHECK (length(trim(manifest_revision)) > 0),
    image_reference TEXT NOT NULL CHECK (length(trim(image_reference)) > 0),
    registry TEXT NOT NULL DEFAULT '',
    repository TEXT NOT NULL DEFAULT '',
    image_tag TEXT NOT NULL DEFAULT '',
    requested_digest TEXT NOT NULL DEFAULT '',
    resolved_digest TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('Available', 'Unavailable')),
    error_code TEXT NOT NULL DEFAULT '',
    resolved_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (status = 'Available' AND resolved_digest <> '' AND error_code = '')
        OR
        (status = 'Unavailable' AND resolved_digest = '' AND error_code <> '')
    )
);

CREATE INDEX application_image_digest_snapshots_application_resolved_idx
ON application_image_digest_snapshots (application_id, resolved_at DESC);
