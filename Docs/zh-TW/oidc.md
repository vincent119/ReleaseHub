# OIDC

## 目的與信任邊界

ReleaseHub 使用 OIDC Authorization Code Flow、PKCE 與 server-side BFF session，支援符合目前設定契約的 Keycloak 或 Amazon Cognito issuer。Browser 只持有 opaque ReleaseHub session cookie；OIDC refresh token 加密後保存於 PostgreSQL，不回傳前端。

供應商傳入的 username 維持原值。若 username 已被另一個 OIDC identity 使用，登入會明確失敗，不會自動加 suffix。

## 群組與授權

OIDC group claim 只能映射到 ReleaseHub viewer group。平台管理者仍需在 ReleaseHub 內將群組指派至 Organization／Project scope；project manager 再管理自己 Project 可授予的角色與資源範圍。未取得任何 ReleaseHub scope 的使用者看不到業務資源。

有效權限由 Group 綁定的 Role 與 Scope 聯集產生，明確 Deny 優先於允許，本地帳號停用優先於所有授權。ReleaseHub 不支援直接把 Role 或單一 permission 指派給 User。

## Session 生命週期

- `oidc.access_token_max_ttl` 限制供應商 access token 最長有效期。
- `session.idle_timeout` 在使用者活動時延長，但不超過 `session.absolute_ttl`。
- `identity.sync_interval` 到期後，下一次驗證 session 時會 refresh OIDC identity 與 group claims。
- identity refresh 或群組同步失敗時，ReleaseHub 撤銷該 session；平台停用本地帳號時，會在同一 database transaction 撤銷該使用者的所有 sessions。
- 供應商可呼叫 back-channel logout，提前撤銷同一 identity 的所有 ReleaseHub sessions。

Session 與加密後的 refresh token 目前保存於 PostgreSQL，不使用 Redis 作為 session store。Cookie、CSRF token、redirect URL 與 logout URL 的完整設定請參考[設定文件](configuration.md)。
