CREATE OR REPLACE FUNCTION releasehub_disable_group_policies() RETURNS trigger
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

DROP INDEX IF EXISTS authorization_groups_system_key_unique;
ALTER TABLE authorization_groups DROP COLUMN IF EXISTS system_key;
