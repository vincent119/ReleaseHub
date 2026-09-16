# 架構

## 目的與範圍

本文件供平台開發者與維運人員理解 ReleaseHub 的 runtime 邊界、資料來源與部署控制流。ReleaseHub 負責發佈治理，不取代 GitOps repository、Image Updater、Argo CD 或 Kubernetes controller。

## 元件責任

ReleaseHub 以兩個 container image 提供四個獨立 workload：

| Workload | Image               | 責任                                                               |
| -------- | ------------------- | ------------------------------------------------------------------ |
| Web      | `releasehub-web`    | 靜態前端、BFF API reverse proxy、無日誌健康檢查                    |
| API      | `releasehub-server` | Gin HTTP API、OIDC session、RBAC 與管理操作                        |
| Worker   | `releasehub-server` | Candidate reconciliation、Deployment Queue、Argo CD 執行與通知投影 |
| Migrate  | `releasehub-server` | 部署前執行 versioned SQL migration                                 |

API、Worker 與 Migrate 各自使用對應的 PostgreSQL connection pool。OIDC session、Deployment Request、Deployment Schedule policy、Queue、Audit 與 Outbox 均保存於 PostgreSQL。設定 schema 目前保留 `redis.*` 欄位，但 runtime 尚未使用 Redis 作為 session 或 Queue 儲存區。

Migrate 是獨立一次性 Job；API 與 Worker 啟動時不會執行 migration。Worker 透過 Argo CD gRPC API 讀取及操作 Application，並以 AWS ECR read-only API 驗證 image digest。

## 系統邊界與資料流

```mermaid
flowchart LR
  User[使用者] -->|HTTPS| Web[Web／Nginx]
  Web -->|BFF API／SSE| API[ReleaseHub API]
  IdP[OIDC Provider] <-->|登入、refresh、logout| API

  API -->|Command、Session、Schedule policy、Audit、Outbox| DB[(PostgreSQL)]
  Worker[ReleaseHub Worker] -->|Claim Job、重讀 Schedule policy、寫入結果| DB
  Migrate[Migrate Job] -->|Versioned SQL| DB

  GitWriter[Application CI／Image Updater] -->|寫入 image 版本| Git[(GitOps Repository)]
  Git -->|期望狀態| Argo[Argo CD]
  Worker <-->|gRPC：Application、refresh、Sync、狀態| Argo
  Worker -->|DescribeImages| ECR[(Amazon ECR)]
  Argo -->|套用 manifests| K8s[Kubernetes]
```

GitOps repository 與 Image Updater 的責任不變：Image Updater 或 Application CI 負責把新 image 版本寫入 GitOps repository，Argo CD 以 Git 為期望狀態。ReleaseHub 不寫 Git、不控制 Image Updater、不設定 image override。

## Production 發佈控制流

1. Worker 從 Argo CD target manifests 與 live state 偵測 production Candidate。
2. Worker 以 ECR 驗證 image，並建立不可變的 Deployment Request Version。
3. Release Workflow 決定審核與可用轉換；Deployment Plan 決定 Application 相依關係與並行方式。
4. `scheduledFor` 是不可變 Request Version 的 earliest-start；系統先取目前時間與 `scheduledFor` 的較晚者，再依 Environment 的 Deployment Schedule policy 計算下一個可執行時間。
5. 建立 initial Job 或 retry Job 時會依當下 policy 設定 `available_at`。Worker claim Job 後會重讀最新 policy，並在建立 Execution、取得 Application lock、呼叫 Argo CD 或 ECR 前再次評估。
6. 若時間不符合 weekly maintenance window 或落在 blackout，Worker 以原 lease 與 fencing token 將 Job defer 回 `Pending`。這不是執行失敗，不消耗 retry attempt，也不寫入 failure error。
7. 符合時段後，Worker 執行 hard refresh，重新取得 manifests、diff 與 digest；內容漂移時失敗封閉。
8. 驗證通過後，Worker 對核准的 revision 呼叫 Argo CD Sync，並持續對帳 operation、Sync 與 Health。Execution 一旦開始，不因 window 結束或 policy 更新而中斷。

### 排程判定時序

```mermaid
sequenceDiagram
  participant Queue as PostgreSQL Queue
  participant Worker as ReleaseHub Worker
  participant Policy as Environment Schedule Policy
  participant Executor as Deployment Executor
  participant Argo as Argo CD／ECR

  Worker->>Queue: Claim available Job 與 lease／fencing token
  Worker->>Policy: 讀取最新 policy 與 Request scheduledFor
  alt 尚未符合 earliest-start、window 或 blackout
    Worker->>Queue: Fenced defer，更新 available_at 並回復 attempt
  else 可以開始
    Worker->>Executor: 建立或接續 Execution
    Executor->>Argo: Preflight、lock、Sync 與 watch
  end
```

Weekly window 以 IANA timezone 的當地星期與分鐘判定，起點包含、終點不包含；blackout 則保存為 UTC absolute instant。DST spring-forward 不存在的當地時間不會產生可執行 instant；fall-back 重複時間的兩個 instant 只要落在同一 local window 均可執行。搜尋下一個時間最多前進 366 天，找不到時維持 fail closed，並依 Worker retry delay 重新評估。

Policy 更新採 optimistic version。等待中的 Job 在下一次 claim 時套用最新 policy，因此可能提前或延後；Worker restart 後仍由 PostgreSQL 的 `available_at`、lease 與 fencing 接續，不需要記憶體內 timer。已開始的 Execution 不重新套用 window gate。

## 失敗邊界與限制

- Argo CD、ECR 或 target manifests 無法提供足夠證據時，不執行 Sync。
- 設定漂移會阻擋建立 Deployment Request、部署及 Forward Rollback。
- Application operation lock 與 Job lease 保存於 PostgreSQL，避免同一 Application 被並行覆寫。
- Schedule defer 發生在 Execution、Application lock 及外部系統 side effect 之前；stale Worker 無法使用舊 fencing token 改寫 Job。
- 第一階段只允許一個 Worker replica，確保設定的 Application 並行數是全平台上限。
- ReleaseHub 不使用 Argo CD history rollback。回復歷史版本採 Forward Rollback，由來源流程產生新版本，再走一般 Deployment Request、審核與部署流程。
