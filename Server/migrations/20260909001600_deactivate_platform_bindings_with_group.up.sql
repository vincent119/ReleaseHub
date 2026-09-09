ALTER TABLE authorization_groups ADD COLUMN system_key TEXT;

UPDATE authorization_groups
SET system_key = 'platform_administrators', updated_at = now()
WHERE owner_kind = 'platform' AND owner_id IS NULL AND lower(name) = 'platform_administrators';

CREATE UNIQUE INDEX authorization_groups_system_key_unique
ON authorization_groups (system_key)
WHERE system_key IS NOT NULL;

CREATE OR REPLACE FUNCTION releasehub_disable_group_policies() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.disabled_at IS NULL AND NEW.disabled_at IS NOT NULL THEN
        UPDATE authorization_group_role_bindings SET active = false, updated_at = now()
        WHERE group_id = NEW.id AND active;
        UPDATE authorization_platform_role_bindings SET active = false, updated_at = now()
        WHERE group_id = NEW.id AND active;
        UPDATE authorization_deny_policies SET active = false, updated_at = now()
        WHERE group_id = NEW.id AND active;
    END IF;
    RETURN NEW;
END;
$$;
