# 架構

ReleaseHub 以兩個 container image 提供四個獨立 workload：

| Workload | Image | 責任 |
| --- | --- | --- |
| Web | `releasehub-web` | 靜態前端、BFF API reverse proxy、無日誌健康檢查 |
| API | `releasehub-server` | Gin HTTP API、OIDC session、RBAC 與管理操作 |
| Worker | `releasehub-server` | Argo CD reconciliation、設定漂移與 ECR digest 解析 |
| Migrate | `releasehub-server` | 部署前執行 versioned SQL migration |

API 與 Worker 分別使用自己的 PostgreSQL connection pool，並共用 Redis session 基礎設施。Worker 透過 Argo CD gRPC API 讀取 Application；ECR 只使用 read-only API。Migrate 是獨立一次性 Job，API 與 Worker 啟動時不會自動 migration。

目前邊界只涵蓋平台基礎、Application discovery、onboarding、設定漂移與目標 image digest 快照。ReleaseHub 不修改 GitOps repository、不控制 Argo CD Image Updater，也不設定 image override 或觸發正式發佈。

