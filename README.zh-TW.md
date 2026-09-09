# ReleaseHub

[English](README.en.md)

ReleaseHub 是以 Argo CD 為部署執行端的發佈治理平台，讓平台團隊集中管理 Application onboarding、Production Deployment Request、審核、部署順序、執行狀態與稽核紀錄。GitOps repository 仍是期望狀態來源；ReleaseHub 不寫 Git、不控制 Image Updater，也不以 image override 取代既有 GitOps 流程。

## 目前能力

- 從 Argo CD 探索並 onboarding 帶有管理 label 的 Application。
- 從 Argo CD target manifests 與 ECR digest 建立不可變的 Deployment Request Version。
- 以 Release Workflow 管理審核與人工或自動轉換。
- 以 Deployment Plan DAG 定義 Application 相依關係與平行上限。
- 執行部署前 hard refresh、內容漂移檢查、指定 revision Sync 與結果對帳。
- 提供 `Partial Failed`、失敗 Application retry、terminate、人工解鎖及 Forward Rollback 流程。
- 使用 OIDC、Group／Role／Scope／Deny、站內通知與 Audit 管理存取及操作證據。
- 依 `manager_password` 建立本機 `admin` 帳號，並強制首次登入後變更密碼。

## 開發驗證

### 前置需求

| 工具 | 需求來源 |
| --- | --- |
| Go `1.26.6` | `Server/go.mod` |
| Node.js `22.22.0` 以上 | `Web/package.json` |
| pnpm `11.19.0` | `Web/package.json` |
| Docker | PostgreSQL Testcontainers 整合測試 |
| Helm、Kustomize、kubeconform、Trivy、yq | 部署描述驗證 |

在 repository root 執行：

```bash
cd Web
pnpm install --frozen-lockfile
cd ../Server
go mod download
cd ..
make verify
```

`make verify` 成功時會以 exit code `0` 結束，並完成 OpenAPI 產物一致性、Server／Web 測試、lint、build、雙語文件配對，以及 Helm／Kustomize schema 與安全設定驗證。若只想查看可用 target，可執行 `make help`。

啟動完整服務前，仍需提供 PostgreSQL、Argo CD、AWS ECR 與 Kubernetes 環境；啟用本機登入時，OIDC 為選用整合。請依[設定文件](Docs/zh-TW/configuration.md)及[部署文件](Docs/zh-TW/deployment.md)準備。

本機初始化可設定 `manager_password: admin` 以建立 `admin / admin`。首次登入後必須先變更密碼，才能執行其他已認證操作。詳情請參考[設定文件](Docs/zh-TW/configuration.md#初始管理員)。

## Repository 結構

- `Server/`：Go API、Worker 與 Migrate command。
- `Web/`：React Web 與 Nginx runtime。
- `API/`：OpenAPI 契約來源。
- `Deployments/`：Helm 與 Kustomize 部署描述。
- `Docs/zh-TW/`、`Docs/en/`：中英文技術文件。

## 文件導覽

- 第一次部署：[設定](Docs/zh-TW/configuration.md) → [部署](Docs/zh-TW/deployment.md) → [操作 Runbook](Docs/zh-TW/operations-runbook.md)
- 平台與資料流：[架構](Docs/zh-TW/architecture.md)
- Application 整合：[Argo CD](Docs/zh-TW/argocd.md) → [Amazon ECR](Docs/zh-TW/ecr.md)
- 身分與 Session：[OIDC](Docs/zh-TW/oidc.md)

## 第一階段限制

- 單一 Argo CD instance。
- 單一 AWS account 與單一 region。
- Worker replica 固定為 `1`，以維持全平台 Application 並行上限。
- 真實 Argo CD／ECR 隔離環境仍需另行完成端對端驗證，repository 驗證不能取代 production 演練。

## 開發命令

```bash
make generate       # 重新產生 Go 與 TypeScript OpenAPI 程式碼
make lint-openapi   # 驗證 OpenAPI 契約
make test           # 執行 Server 與 Web 測試
make build          # 建置 Server commands 與 Web
make verify         # 執行完整 repository 驗證
```

## 授權

授權方式尚未決定，因此目前沒有 `LICENSE` 檔案。
