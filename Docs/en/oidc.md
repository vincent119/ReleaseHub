# OIDC

## Purpose and Trust Boundary

ReleaseHub uses the OIDC Authorization Code Flow with PKCE and server-side BFF sessions. A Keycloak or Amazon Cognito issuer that satisfies the current configuration contract is supported. The browser holds only an opaque ReleaseHub session cookie. The OIDC refresh token is encrypted and stored in PostgreSQL; it is never returned to the frontend.

Provider usernames remain unchanged. Login fails explicitly when a username already belongs to another OIDC identity; ReleaseHub never adds a generated suffix.

## Groups and Authorization

An OIDC group claim can map only to a ReleaseHub viewer group. A platform administrator must still assign the group to an Organization or Project scope in ReleaseHub. A project manager then manages grantable roles and resource scopes inside that Project. Users without a ReleaseHub scope cannot view business resources.

Effective permissions are the union of Roles and Scopes bound to the user's Groups. An explicit Deny overrides an allow, and a locally disabled account overrides every grant. ReleaseHub does not assign a Role or individual permission directly to a User.

## Session Lifecycle

- `oidc.access_token_max_ttl` limits the provider access-token lifetime.
- `session.idle_timeout` extends with activity but never exceeds `session.absolute_ttl`.
- After `identity.sync_interval`, the next session authentication refreshes the OIDC identity and group claims.
- A failed identity refresh or group synchronization revokes that session. Disabling a local account revokes every session for that user in the same database transaction.
- Provider back-channel logout can revoke every ReleaseHub session for the same identity earlier.

Sessions and encrypted refresh tokens are currently stored in PostgreSQL, not Redis. See [Configuration](configuration.md) for cookie, CSRF token, redirect URL, and logout URL settings.
