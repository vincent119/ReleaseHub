# OIDC

ReleaseHub uses the OIDC Authorization Code Flow with PKCE and server-side BFF sessions. Keycloak and Amazon Cognito are supported. Provider usernames remain unchanged. Login fails explicitly when a username already belongs to another OIDC identity; ReleaseHub never adds a generated suffix.

An OIDC group claim can map only to a ReleaseHub viewer group. A platform administrator must still assign the group to an Organization or Project scope in ReleaseHub. A project manager then manages grantable roles and resource scopes inside that Project. Users without a ReleaseHub scope cannot view business resources.

Maximum access-token age, session idle timeout, and absolute TTL are platform-wide settings that users cannot extend. A failed identity refresh or a locally disabled account revokes the session immediately. Provider back-channel logout can terminate an existing session sooner.

