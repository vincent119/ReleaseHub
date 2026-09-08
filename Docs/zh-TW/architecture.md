# 架構

ReleaseHub 以兩個 container image 提供四個獨立 workload：

| Workload | Image | 責任 |
| --- | --- | --- |
| Web | `releasehub-web` | 靜態前端、BFF API reverse proxy、無日誌健康檢查 |
| API | `releasehub-server` | Gin HTTP API、OIDC session、RBAC 與管理操作 |
| Worker | `releasehub-server` | Candidate reconciliation、Deployment Queue、Argo CD 執行與通知投影 |
| Migrate | `releasehub-server` | 部署前執行 versioned SQL migration |

API 與 Worker 分別使用自己的 PostgreSQL connection pool，並共用 Redis session 基礎設施。Worker 透過 Argo CD gRPC API 讀取 Application；ECR 只使用 read-only API。Migrate 是獨立一次性 Job，API 與 Worker 啟動時不會自動 migration。

ReleaseHub 依 Argo CD Application 的 target manifests 與 live state 偵測 production 發佈候選，建立 immutable Deployment Request Version，並在 Release Workflow 通過後依 Deployment Plan 呼叫 Argo CD Sync。部署前會執行 hard refresh，確認核准的 revision 與 image digest 未漂移；結果則由 Worker 持續對帳。

GitOps repository 與 Image Updater 的責任不變：Image Updater 或應用程式 CI 負責把新 image 版本寫回 GitOps repository，Argo CD 以 Git 為期望狀態。ReleaseHub 不寫 Git、不控制 Image Updater、不設定 image override，也不使用 Argo CD history rollback。回復歷史版本採 Forward Rollback，由來源流程產生新的版本，再走一般 Deployment Request、審核與部署流程。
