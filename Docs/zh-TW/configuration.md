# 設定

Server 預設讀取 `/app/configs/config.yaml`。優先順序由低至高為 defaults、YAML、`RELEASEHUB_` environment variables、CLI flags。巢狀欄位以底線表示，例如 `database.password` 對應 `RELEASEHUB_DATABASE_PASSWORD`。

部署範本要求既有 Kubernetes Secret `releasehub-secrets`，至少提供：

- `RELEASEHUB_DATABASE_PASSWORD`
- `RELEASEHUB_REDIS_PASSWORD`
- `RELEASEHUB_OIDC_CLIENT_SECRET`
- `RELEASEHUB_ARGOCD_TOKEN`
- `RELEASEHUB_SESSION_ENCRYPTION_KEY`

Secret 不得寫入 values、Kustomize base、image 或 Git。非 Secret 設定放在 ConfigMap。啟用 Worker 前，必須設定有效的 Argo CD address/token，以及單一 AWS account、region 與允許的 ECR repository 清單。

`log.format` 可使用 `json` 或 `console`。每筆 API 與 Worker 日誌均包含 component 類別；health、readiness、metrics 與 tracing endpoints 不記 access log。

