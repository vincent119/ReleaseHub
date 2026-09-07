# OIDC

ReleaseHub 使用 OIDC Authorization Code Flow、PKCE 與 server-side BFF session，支援 Keycloak 或 Amazon Cognito。供應商傳入的 username 維持原值；若 username 已被另一個 OIDC identity 使用，登入會明確失敗，不會自動加 suffix。

OIDC group claim 只能映射到 ReleaseHub viewer group。平台管理者仍需在 ReleaseHub 內將群組指派至 Organization／Project scope；project manager 再管理自己 Project 可授予的角色與資源範圍。未取得任何 ReleaseHub scope 的使用者看不到業務資源。

access token 最長有效期、session idle timeout 與 absolute TTL 都由平台全域設定，使用者不能自行延長。identity refresh 失敗或本地帳號停用時，session 立即撤銷。供應商可搭配 back-channel logout 提前終止既有 session。

