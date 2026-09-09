DELETE FROM sessions WHERE authentication_method = 'local';
DROP TABLE IF EXISTS local_credentials;
ALTER TABLE sessions
    ALTER COLUMN refresh_token_ciphertext SET NOT NULL,
    DROP COLUMN authentication_method;
