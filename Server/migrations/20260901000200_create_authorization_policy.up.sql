CREATE TABLE authorization_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('platform', 'organization', 'project')),
    owner_id UUID NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    oidc_viewer_only BOOLEAN NOT NULL DEFAULT false,
    disabled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((owner_kind = 'platform' AND owner_id IS NULL) OR (owner_kind <> 'platform' AND owner_id IS NOT NULL))
);

CREATE UNIQUE INDEX authorization_groups_owner_name_idx
ON authorization_groups (owner_kind, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

CREATE TABLE authorization_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('platform', 'project')),
    owner_id UUID NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    system_key TEXT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((owner_kind = 'platform' AND owner_id IS NULL) OR (owner_kind = 'project' AND owner_id IS NOT NULL))
);

CREATE UNIQUE INDEX authorization_roles_owner_name_idx
ON authorization_roles (owner_kind, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

CREATE TABLE authorization_permissions (
    key TEXT PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    platform_only BOOLEAN NOT NULL DEFAULT false,
    project_role_delegable BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE authorization_role_permissions (
    role_id UUID NOT NULL REFERENCES authorization_roles(id),
    permission_key TEXT NOT NULL REFERENCES authorization_permissions(key),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_key)
);

CREATE TABLE authorization_group_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES authorization_groups(id),
    user_id UUID NOT NULL REFERENCES users(id),
    source TEXT NOT NULL CHECK (source IN ('manual', 'oidc')),
    issuer TEXT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((source = 'manual' AND issuer IS NULL) OR (source = 'oidc' AND issuer IS NOT NULL))
);

CREATE UNIQUE INDEX authorization_group_memberships_manual_idx
ON authorization_group_memberships (group_id, user_id)
WHERE source = 'manual';

CREATE UNIQUE INDEX authorization_group_memberships_oidc_idx
ON authorization_group_memberships (group_id, user_id, issuer)
WHERE source = 'oidc';

CREATE INDEX authorization_group_memberships_user_idx
ON authorization_group_memberships (user_id)
WHERE active;

CREATE TABLE authorization_group_role_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES authorization_groups(id),
    role_id UUID NOT NULL REFERENCES authorization_roles(id),
    organization_id UUID NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('project', 'environment', 'application')),
    project_id UUID NOT NULL,
    environment_id UUID NULL,
    application_id UUID NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scope_kind = 'project' AND environment_id IS NULL AND application_id IS NULL) OR
        (scope_kind = 'environment' AND environment_id IS NOT NULL AND application_id IS NULL) OR
        (scope_kind = 'application' AND environment_id IS NOT NULL AND application_id IS NOT NULL)
    )
);

CREATE INDEX authorization_group_role_bindings_group_idx
ON authorization_group_role_bindings (group_id)
WHERE active;

CREATE TABLE authorization_deny_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES authorization_groups(id),
    permission_key TEXT NOT NULL REFERENCES authorization_permissions(key),
    organization_id UUID NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('project', 'environment', 'application')),
    project_id UUID NOT NULL,
    environment_id UUID NULL,
    application_id UUID NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scope_kind = 'project' AND environment_id IS NULL AND application_id IS NULL) OR
        (scope_kind = 'environment' AND environment_id IS NOT NULL AND application_id IS NULL) OR
        (scope_kind = 'application' AND environment_id IS NOT NULL AND application_id IS NOT NULL)
    )
);

CREATE INDEX authorization_deny_policies_group_idx
ON authorization_deny_policies (group_id)
WHERE active;

CREATE TABLE authorization_oidc_group_mappings (
    issuer TEXT NOT NULL,
    provider_group TEXT NOT NULL,
    group_id UUID NOT NULL REFERENCES authorization_groups(id),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issuer, provider_group),
    UNIQUE (issuer, group_id)
);

CREATE TABLE authorization_policy_revision (
    singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO authorization_policy_revision (singleton, revision) VALUES (true, 0);

CREATE FUNCTION releasehub_bump_authorization_policy_revision() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    next_revision BIGINT;
BEGIN
    UPDATE authorization_policy_revision
    SET revision = revision + 1, updated_at = now()
    WHERE singleton = true
    RETURNING revision INTO next_revision;
    PERFORM pg_notify('releasehub_policy_revision', next_revision::text);
    RETURN NULL;
END;
$$;

CREATE TRIGGER authorization_groups_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_groups
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_roles_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_roles
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_permissions_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_permissions
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_role_permissions_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_role_permissions
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_group_memberships_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_group_memberships
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_group_role_bindings_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_group_role_bindings
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_deny_policies_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_deny_policies
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
CREATE TRIGGER authorization_oidc_group_mappings_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_oidc_group_mappings
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();

CREATE FUNCTION releasehub_validate_project_role_permission() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    role_owner_kind TEXT;
    permission_delegable BOOLEAN;
BEGIN
    SELECT owner_kind INTO role_owner_kind FROM authorization_roles WHERE id = NEW.role_id;
    SELECT project_role_delegable INTO permission_delegable FROM authorization_permissions WHERE key = NEW.permission_key;
    IF role_owner_kind = 'project' AND NOT permission_delegable THEN
        RAISE EXCEPTION 'permission % cannot be added to a Project-owned role', NEW.permission_key;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER authorization_role_permissions_validate
BEFORE INSERT OR UPDATE ON authorization_role_permissions
FOR EACH ROW EXECUTE FUNCTION releasehub_validate_project_role_permission();

CREATE FUNCTION releasehub_disable_group_policies() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.disabled_at IS NULL AND NEW.disabled_at IS NOT NULL THEN
        UPDATE authorization_group_role_bindings SET active = false, updated_at = now()
        WHERE group_id = NEW.id AND active;
        UPDATE authorization_deny_policies SET active = false, updated_at = now()
        WHERE group_id = NEW.id AND active;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER authorization_groups_disable_policies
AFTER UPDATE OF disabled_at ON authorization_groups
FOR EACH ROW EXECUTE FUNCTION releasehub_disable_group_policies();

INSERT INTO authorization_permissions (key, platform_only, project_role_delegable) VALUES
    ('platform.manage', true, false),
    ('project.manage', false, false),
    ('group.manage', false, false),
    ('role.manage', false, false),
    ('resource.view', false, true),
    ('application.onboard', false, true),
    ('application.refresh', false, true),
    ('application.automated_sync.disable', false, true),
    ('ecr.digest.read', false, true);

INSERT INTO authorization_roles (id, owner_kind, name, system_key) VALUES
    ('00000000-0000-0000-0000-000000000101', 'platform', 'project_manager', 'project_manager'),
    ('00000000-0000-0000-0000-000000000102', 'platform', 'devops_manager', 'devops_manager'),
    ('00000000-0000-0000-0000-000000000103', 'platform', 'dev_member', 'dev_member'),
    ('00000000-0000-0000-0000-000000000104', 'platform', 'devops_member', 'devops_member'),
    ('00000000-0000-0000-0000-000000000105', 'platform', 'viewer', 'viewer');

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000101', 'project.manage'),
    ('00000000-0000-0000-0000-000000000101', 'group.manage'),
    ('00000000-0000-0000-0000-000000000101', 'role.manage'),
    ('00000000-0000-0000-0000-000000000101', 'resource.view'),
    ('00000000-0000-0000-0000-000000000101', 'application.onboard'),
    ('00000000-0000-0000-0000-000000000101', 'application.refresh'),
    ('00000000-0000-0000-0000-000000000101', 'application.automated_sync.disable'),
    ('00000000-0000-0000-0000-000000000101', 'ecr.digest.read'),
    ('00000000-0000-0000-0000-000000000102', 'resource.view'),
    ('00000000-0000-0000-0000-000000000102', 'application.onboard'),
    ('00000000-0000-0000-0000-000000000102', 'application.refresh'),
    ('00000000-0000-0000-0000-000000000102', 'application.automated_sync.disable'),
    ('00000000-0000-0000-0000-000000000102', 'ecr.digest.read'),
    ('00000000-0000-0000-0000-000000000103', 'resource.view'),
    ('00000000-0000-0000-0000-000000000104', 'resource.view'),
    ('00000000-0000-0000-0000-000000000104', 'application.refresh'),
    ('00000000-0000-0000-0000-000000000104', 'ecr.digest.read'),
    ('00000000-0000-0000-0000-000000000105', 'resource.view');
