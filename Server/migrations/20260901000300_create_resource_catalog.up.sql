CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    active BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX organizations_name_idx ON organizations (lower(name));

INSERT INTO organizations (id, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'default');

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    active BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id)
);

CREATE UNIQUE INDEX projects_organization_name_idx ON projects (organization_id, lower(name));

CREATE TABLE environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    project_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    environment_type TEXT NOT NULL CHECK (environment_type IN ('Development', 'Testing', 'Staging', 'Production')),
    active BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, organization_id) REFERENCES projects(id, organization_id),
    UNIQUE (id, project_id, organization_id)
);

CREATE UNIQUE INDEX environments_project_name_idx ON environments (project_id, lower(name));

CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    project_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    argocd_namespace TEXT NOT NULL CHECK (length(btrim(argocd_namespace)) BETWEEN 1 AND 253),
    argocd_application_name TEXT NOT NULL CHECK (length(btrim(argocd_application_name)) BETWEEN 1 AND 253),
    argocd_project TEXT NOT NULL CHECK (length(btrim(argocd_project)) BETWEEN 1 AND 253),
    destination_server TEXT NOT NULL CHECK (length(btrim(destination_server)) > 0),
    destination_namespace TEXT NOT NULL CHECK (length(btrim(destination_namespace)) BETWEEN 1 AND 253),
    source_repo_url TEXT NOT NULL CHECK (length(btrim(source_repo_url)) > 0),
    source_target_revision TEXT NOT NULL CHECK (length(btrim(source_target_revision)) > 0),
    source_path TEXT NOT NULL CHECK (
        length(btrim(source_path)) > 0 AND source_path <> '.' AND source_path !~ '^/' AND
        source_path !~ '(^|/)\.\.(/|$)'
    ),
    active BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (environment_id, project_id, organization_id) REFERENCES environments(id, project_id, organization_id)
);

CREATE UNIQUE INDEX applications_environment_name_idx ON applications (environment_id, lower(name));
CREATE UNIQUE INDEX applications_argocd_identity_idx ON applications (argocd_namespace, argocd_application_name);
CREATE INDEX applications_gitops_source_idx ON applications (source_repo_url, source_target_revision, source_path);

CREATE TABLE environment_label_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    project_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    label_key TEXT NOT NULL CHECK (length(btrim(label_key)) BETWEEN 1 AND 317),
    label_value TEXT NOT NULL CHECK (length(btrim(label_value)) BETWEEN 0 AND 63),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (environment_id, project_id, organization_id) REFERENCES environments(id, project_id, organization_id)
);

CREATE UNIQUE INDEX environment_label_mappings_project_label_idx
ON environment_label_mappings (project_id, label_key, label_value);
