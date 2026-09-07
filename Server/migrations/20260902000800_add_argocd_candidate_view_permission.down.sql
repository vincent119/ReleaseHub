DROP TRIGGER IF EXISTS authorization_platform_role_bindings_revision ON authorization_platform_role_bindings;
DROP TABLE IF EXISTS authorization_platform_role_bindings;
DELETE FROM authorization_role_permissions WHERE role_id = '00000000-0000-0000-0000-000000000100';
DELETE FROM authorization_roles WHERE id = '00000000-0000-0000-0000-000000000100';
DELETE FROM authorization_permissions WHERE key IN ('argocd.candidate.view', 'argocd.candidate.assign');
