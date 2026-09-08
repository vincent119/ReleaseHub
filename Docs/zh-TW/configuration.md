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

## Worker 與通知

| YAML 欄位 | 預設值 | 用途 |
| --- | --- | --- |
| `worker.reconcile_interval` | `30s` | 重新讀取 Argo CD Application 並偵測 Candidate／設定漂移 |
| `worker.deployment_poll_interval` | `1s` | 輪詢可 claim 的 Deployment Job |
| `worker.job_lease_duration` | `30s` | Job lease 與 fencing 的有效期間 |
| `worker.job_retry_delay` | `5s` | 基礎設施失敗後重新排隊前的等待時間 |
| `worker.application_lock_duration` | `30s` | Application operation lock 的有效期間 |
| `worker.max_parallel_deployments` | `10` | 單一 Worker 同時執行的 Application 上限；Plan 可設定更低上限 |
| `notifications.retention` | `168h` | 站內通知保存期間；不影響 Audit |
| `notifications.projection_interval` | `1s` | Outbox 投影及 SSE 可讀事件更新間隔 |

第一階段必須維持單一 Worker replica。多個 Worker replica 會各自套用 `max_parallel_deployments`，無法保證全平台上限。SSE 由 Web Nginx 的精確路徑 `/api/v1/notifications/events` 代理，並關閉 buffering 與 cache；Client 斷線後以 `Last-Event-ID` 重播，再重新取得當下權限資料。
