# 設定

## 載入方式與優先順序

Server 預設讀取 `/app/configs/config.yaml`。優先順序由低至高為 defaults、YAML、`RELEASEHUB_` environment variables、CLI flags。巢狀欄位以底線表示，例如 `database.password` 對應 `RELEASEHUB_DATABASE_PASSWORD`。

完整欄位與非 Secret 範例位於 [`Server/configs/config.example.yaml`](../../Server/configs/config.example.yaml)。API、Worker 與 Migrate 共用同一份 typed configuration，但各自使用對應的 database pool。

## 初始管理員

API 連線至已完成 migration 的資料庫前，先設定最上層 `manager_password`。當初始管理員資料尚未建立時，API 會將此值雜湊，並透過 GORM 建立 `admin` 帳號、本機 credential 與 `platform_administrator` membership；服務重新啟動不會覆寫既有密碼。

本機開發設定 `manager_password: admin` 時，初始登入為 `admin / admin`。首次登入後只能變更密碼或登出；新密碼長度必須為 8 至 72 bytes，完成變更會撤銷所有現有 Session。新資料庫將 `manager_password` 留空會略過本機管理員 bootstrap，但不會刪除既有本機 credential。

## 本機 API 啟動

完成 database migration 後，若要透過 HTTP 測試本機認證，請使用下列僅供本機使用的設定：

```yaml
manager_password: admin

database:
  ssl_mode: disable

oidc:
  issuer: ""
  client_id: ""
  client_secret: ""
  redirect_url: ""
  web_redirect_url: "http://localhost:5173/"
  logout_url: ""
  post_logout_redirect_url: ""

argocd:
  address: ""
  token: ""

session:
  encryption_key: ""
  cookie_secure: false
```

在 repository root 同時啟動 API 與 Web：

```bash
make dev
```

若 environment 尚未提供 Session 加密金鑰，`make dev-api` 會產生僅供該程序使用的暫時金鑰、關閉本機 HTTP 的 Secure cookie，並停用不完整的 OIDC 設定。`make dev-web` 會啟動 Vite，並將 `/api` 代理至 `http://127.0.0.1:7580`。Make 會優先使用 `PATH` 中的 `pnpm`，找不到時改用 `corepack pnpm`；若兩者都無法執行，preflight 會在 API 啟動前停止。

若不透過 Make 而只啟動 API，請先在目前的 shell 產生 Session 加密金鑰：

```bash
export RELEASEHUB_SESSION_ENCRYPTION_KEY="$(openssl rand -hex 32)"
go run ./cmd/releasehub api --config ./configs/config.yaml
```

金鑰至少必須有 32 bytes，且只會保留在目前的 shell。停用 OIDC 時，`oidc.redirect_url` 必須保持空值，否則 API 會將 OIDC 視為只完成部分設定並停止啟動。`argocd.address` 與 `argocd.token` 都為空時，API 不建立 Argo CD client，依賴 Argo CD 的操作會回傳 `503 Service Unavailable`；Worker 仍要求完整的 Argo CD 設定。若預設值不符合本機環境，可覆寫 `DEV_API_ADDRESS`、`DEV_API_PROXY_TARGET`、`DEV_WEB_PORT`、`DEV_WEB_URL` 或 `SERVER_CONFIG`。

## Secret 與外部依賴

部署範本要求既有 Kubernetes Secret `releasehub-secrets`，至少提供：

- `RELEASEHUB_DATABASE_PASSWORD`
- `RELEASEHUB_REDIS_PASSWORD`
- `RELEASEHUB_OIDC_CLIENT_SECRET`
- `RELEASEHUB_ARGOCD_TOKEN`
- `RELEASEHUB_SESSION_ENCRYPTION_KEY`

Secret 不得寫入 values、Kustomize base、image 或 Git。非 Secret 設定放在 ConfigMap。啟用 Worker 前，必須設定有效的 Argo CD address/token，以及單一 AWS account、region 與允許的 ECR repository 清單。

Session、Queue、Audit 與 Outbox 目前保存於 PostgreSQL。設定 schema 仍要求有效的 `redis.address` 並保留 pool 欄位，但 runtime 尚未建立 Redis client；不得把 Redis 視為目前的 session store 或 Queue store。

## 日誌與觀測

`log.format` 可使用 `json` 或 `console`。API 與 Worker application logs 均包含 component 類別。成功的 `/healthz`、`/readyz` 與設定的 metrics path 不產生 access log、request metrics 或 trace；失敗回應仍保留診斷觀測資料。Tracing 啟用時透過 OTLP gRPC exporter 傳送，不提供獨立 tracing HTTP endpoint。

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

## 啟動前驗證

缺少 database、Redis address、session encryption key 或 component 所需的 OIDC／Argo CD／AWS 設定時，Server 會在建立 runtime 前失敗。部署前至少確認：

1. ConfigMap 不含 Secret 值。
2. `releasehub-secrets` 的必要 key 均存在。
3. PostgreSQL 可連線且已依[資料庫初始化文件](database-initialization.md)完成 versioned migrations。
4. 啟用 OIDC 時，其 redirect URL 與外部 ReleaseHub URL 一致。
5. Argo CD 與 ECR 身分符合最小權限。
