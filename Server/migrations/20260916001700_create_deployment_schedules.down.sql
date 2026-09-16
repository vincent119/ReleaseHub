DELETE FROM authorization_role_permissions
WHERE permission_key = 'deployment_schedule.manage';

DELETE FROM authorization_permissions
WHERE key = 'deployment_schedule.manage';

DROP TABLE IF EXISTS deployment_schedule_commands;
DROP TABLE IF EXISTS deployment_schedule_policies;
