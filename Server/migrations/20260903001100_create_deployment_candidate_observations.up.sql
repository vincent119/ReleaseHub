CREATE TABLE deployment_candidate_observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id),
    request_version_id UUID NOT NULL UNIQUE REFERENCES deployment_request_versions(id),
    workflow_version_id UUID NOT NULL REFERENCES release_workflow_versions(id),
    plan_version_id UUID NOT NULL REFERENCES deployment_plan_versions(id),
    target_revision TEXT NOT NULL CHECK (length(btrim(target_revision)) > 0),
    target_revisions JSONB NOT NULL CHECK (jsonb_typeof(target_revisions) = 'array'),
    manifest_hash TEXT NOT NULL CHECK (manifest_hash ~ '^[a-f0-9]{64}$'),
    diff_hash TEXT NOT NULL CHECK (diff_hash ~ '^[a-f0-9]{64}$'),
    diff_snapshot JSONB NOT NULL CHECK (jsonb_typeof(diff_snapshot) = 'array'),
    source_snapshot JSONB NOT NULL CHECK (jsonb_typeof(source_snapshot) = 'array'),
    image_snapshot JSONB NOT NULL CHECK (jsonb_typeof(image_snapshot) = 'array'),
    fingerprint TEXT NOT NULL CHECK (fingerprint ~ '^[a-f0-9]{64}$'),
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, fingerprint)
);

CREATE INDEX deployment_candidate_observations_application_observed_idx
ON deployment_candidate_observations (application_id, observed_at DESC);

CREATE TRIGGER deployment_candidate_observations_immutable
BEFORE UPDATE OR DELETE ON deployment_candidate_observations
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();
