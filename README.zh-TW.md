# ReleaseHub

[English](README.en.md)

ReleaseHub 把 Production 發布的候選版本、審核、部署順序和執行證據收在同一個地方，供平台團隊管理 Argo CD Application。CI 仍負責建置 image，GitOps repository 仍保存期望狀態，實際套用則交給 Argo CD。ReleaseHub 介入的是中間那段治理流程。

## ReleaseHub 管理什麼

- 從 Argo CD 找出帶有管理 label 的 Application，完成 onboarding 後持續檢查設定漂移。
- 依 target manifests、live resource diff 與 ECR digest 建立不可變的 Deployment Request Version。
- 用 Release Workflow 處理審核及人工或自動 transition。
- 用 Deployment Plan DAG 控制 Application 相依關係與平行上限。
- 在 Sync 前重新比對 revision、resource diff 和 image digest，內容變動就停止部署。
- 保存 `Partial Failed`、retry、terminate、人工解鎖、Forward Rollback、通知及 Audit 證據。
- 以 OIDC、Group、Role、Scope 與明確 Deny 控制操作權限。

ReleaseHub 不寫 Git，也不控制 Argo CD Image Updater。Production Application 的 automated sync 必須停用，核准後由 ReleaseHub 要求 Argo CD Sync 指定 revision。

## 從哪裡開始

準備部署環境時，依序閱讀[設定](Docs/zh-TW/configuration.md)與[部署](Docs/zh-TW/deployment.md)。PostgreSQL、Argo CD、AWS ECR 和 Kubernetes 都是必要外部服務。OIDC 只在啟用 OIDC 登入時需要。

第一次接 Application，先看 [Argo CD 整合](Docs/zh-TW/argocd.md)和 [Amazon ECR 整合](Docs/zh-TW/ecr.md)。完成 onboarding、Workflow、Plan 及 Environment Binding 後，再依 [Production 發布操作指南](Docs/zh-TW/release-flow.md)跑第一個受管發布。

遇到 Queue 等待、`Partial Failed`、終止、解鎖或 Argo CD 中斷時，直接查[操作 Runbook](Docs/zh-TW/operations-runbook.md)。

## 驗證 repository

### 工具版本

| 工具 | 版本或用途 |
| --- | --- |
| Go | `1.26.6`，來源為 `Server/go.mod` |
| Node.js | `22.22.0` 以上，來源為 `Web/package.json` |
| pnpm | `11.19.0`，來源為 `Web/package.json` |
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

成功時 `make verify` 以 exit code `0` 結束。它會檢查 OpenAPI 產物、Server 與 Web 測試、lint、build、雙語文件配對，以及 Helm／Kustomize schema 和安全設定。`make help` 可列出個別 target。

本機初始化可用 `manager_password: admin` 建立 `admin / admin`。第一次登入後必須立刻變更密碼，詳細設定在[初始管理員](Docs/zh-TW/configuration.md#初始管理員)。

## 文件

- 第一次部署：[設定](Docs/zh-TW/configuration.md) → [部署](Docs/zh-TW/deployment.md) → [操作 Runbook](Docs/zh-TW/operations-runbook.md)
- Production 發布：[發布操作指南](Docs/zh-TW/release-flow.md)
- 平台設計：[架構](Docs/zh-TW/architecture.md)
- Application 整合：[Argo CD](Docs/zh-TW/argocd.md) → [Amazon ECR](Docs/zh-TW/ecr.md)
- 登入與權限：[OIDC](Docs/zh-TW/oidc.md)
- 資料庫建置：[資料庫初始化](Docs/zh-TW/database-initialization.md)

## Repository 結構

- `Server/`：Go API、Worker 與 Migrate command。
- `Web/`：React Web 與 Nginx runtime。
- `API/`：OpenAPI 契約來源。
- `Deployments/`：Helm 與 Kustomize 部署描述。
- `Docs/zh-TW/`、`Docs/en/`：中英文技術文件。

## 第一階段限制

- 只連接一個 Argo CD instance。
- 只使用一個 AWS account 和 region。
- Worker replica 固定為 `1`，讓 Application 並行上限維持全平台一致。
- Repository 內的測試已通過，但真實 Argo CD／ECR 隔離環境端到端演練仍未完成。

## 常用開發命令

```bash
make generate       # 重新產生 Go 與 TypeScript OpenAPI 程式碼
make lint-openapi   # 驗證 OpenAPI 契約
make test           # 執行 Server 與 Web 測試
make build          # 建置 Server commands 與 Web
make verify         # 執行完整 repository 驗證
```

## 授權

授權方式尚未決定，目前沒有 `LICENSE` 檔案。
