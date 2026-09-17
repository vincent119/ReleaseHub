DELETE FROM authorization_role_permissions
WHERE role_id = '00000000-0000-0000-0000-000000000100'
  AND permission_key = 'audit.view';
