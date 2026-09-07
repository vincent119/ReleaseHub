INSERT INTO authorization_permissions (key, platform_only, project_role_delegable) VALUES
    ('argocd.candidate.view', true, false),
    ('argocd.candidate.assign', true, false);

INSERT INTO authorization_roles (id, owner_kind, name, system_key)
VALUES ('00000000-0000-0000-0000-000000000100', 'platform', 'platform_administrator', 'platform_administrator');

INSERT INTO authorization_role_permissions (role_id, permission_key) VALUES
    ('00000000-0000-0000-0000-000000000100', 'platform.manage'),
    ('00000000-0000-0000-0000-000000000100', 'argocd.candidate.view'),
    ('00000000-0000-0000-0000-000000000100', 'argocd.candidate.assign');

CREATE TABLE authorization_platform_role_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES authorization_groups(id),
    role_id UUID NOT NULL REFERENCES authorization_roles(id),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, role_id)
);

CREATE TRIGGER authorization_platform_role_bindings_revision
AFTER INSERT OR UPDATE OR DELETE ON authorization_platform_role_bindings
FOR EACH STATEMENT EXECUTE FUNCTION releasehub_bump_authorization_policy_revision();
