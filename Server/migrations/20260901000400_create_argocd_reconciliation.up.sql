CREATE TABLE argocd_application_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    argocd_namespace TEXT NOT NULL CHECK (length(btrim(argocd_namespace)) BETWEEN 1 AND 253),
    argocd_application_name TEXT NOT NULL CHECK (length(btrim(argocd_application_name)) BETWEEN 1 AND 253),
    argocd_project TEXT NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    sources JSONB NOT NULL DEFAULT '[]'::jsonb,
    destination_server TEXT NOT NULL,
    destination_name TEXT NOT NULL DEFAULT '',
    destination_namespace TEXT NOT NULL DEFAULT '',
    automated_sync BOOLEAN NOT NULL,
    sync_status TEXT NOT NULL DEFAULT '',
    health_status TEXT NOT NULL DEFAULT '',
    operation_phase TEXT NOT NULL DEFAULT '',
    resolved_revision TEXT NOT NULL DEFAULT '',
    resolved_revisions JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_version TEXT NOT NULL DEFAULT '',
    discovery_status TEXT NOT NULL DEFAULT 'present' CHECK (discovery_status IN ('present', 'missing')),
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    missing_since TIMESTAMPTZ NULL,
    reconciliation_token UUID NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (argocd_namespace, argocd_application_name)
);

CREATE INDEX argocd_application_candidates_status_idx
ON argocd_application_candidates (discovery_status, last_seen_at DESC);

CREATE TABLE argocd_application_snapshots (
    application_id UUID PRIMARY KEY REFERENCES applications(id),
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    sources JSONB NOT NULL DEFAULT '[]'::jsonb,
    destination_server TEXT NOT NULL,
    destination_name TEXT NOT NULL DEFAULT '',
    destination_namespace TEXT NOT NULL DEFAULT '',
    automated_sync BOOLEAN NOT NULL,
    managed_label_present BOOLEAN NOT NULL,
    sync_status TEXT NOT NULL DEFAULT '',
    health_status TEXT NOT NULL DEFAULT '',
    operation_phase TEXT NOT NULL DEFAULT '',
    resolved_revision TEXT NOT NULL DEFAULT '',
    resolved_revisions JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_version TEXT NOT NULL DEFAULT '',
    reconciled_at TIMESTAMPTZ NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    missing_since TIMESTAMPTZ NULL,
    reconciliation_token UUID NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX argocd_application_snapshots_missing_idx
ON argocd_application_snapshots (missing_since)
WHERE missing_since IS NOT NULL;
