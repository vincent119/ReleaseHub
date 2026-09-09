ALTER TABLE sessions
    ADD COLUMN authentication_method TEXT NOT NULL DEFAULT 'oidc'
        CHECK (authentication_method IN ('oidc', 'local')),
    ALTER COLUMN refresh_token_ciphertext DROP NOT NULL;

CREATE TABLE local_credentials (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash BYTEA NOT NULL,
    must_change_password BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
