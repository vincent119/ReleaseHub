# 部署

## 目的與前置條件

本文件供平台維運人員建置並部署 ReleaseHub。執行前必須準備 PostgreSQL、Kubernetes namespace、`releasehub-secrets`、Argo CD service account，以及可透過 Pod Identity 或 IRSA 存取的 ECR 範圍；只有啟用 OIDC 登入時才需要 OIDC client。所有 production 設定應放在私有 values 或 Kustomize overlay，不得提交 Secret。

## 建置 image

```bash
docker build -t releasehub-server:local Server
docker build -t releasehub-web:local Web
```

兩個 image 都以 non-root user 執行。Server image 由 API、Worker、Migrate 共用，並以第一個 command argument 選擇 component。

## Helm

先建立 `releasehub-secrets`，再建立 production values 檔案，覆寫連線位址、AWS 範圍、image repository/tag 與 ServiceAccount annotations。repository 不提供 production values；下列變數必須指向操作者建立的檔案：

```bash
releasehub_values_file=/path/to/releasehub-values.production.yaml
test -f "$releasehub_values_file"
helm upgrade --install releasehub Deployments/helm/releasehub \
  --namespace releasehub --create-namespace \
  --values "$releasehub_values_file" \
  --wait --timeout 10m
```

Helm 將 Migrate 建立為 `pre-install,pre-upgrade` hook。Migration 失敗時 Helm 會停止 release；不要略過失敗或單獨啟動新版 API／Worker。成功後確認 release 狀態與 workload：

```bash
helm -n releasehub status releasehub
kubectl -n releasehub get deployment,pod \
  -l app.kubernetes.io/name=releasehub
```

`configuration.worker` 可設定 reconciliation、Queue polling、lease、retry、Application lock 與 Application 並行上限；`configuration.notifications` 可設定通知保存與投影間隔。第一階段的 `worker.replicas` 必須保持 `1`。

## Kustomize

建立 production overlay，patch ConfigMap、ServiceAccount annotations 與 image 名稱；不要直接把環境 Secret 寫入 base。先確認 overlay 可以 render：

```bash
kustomize build /path/to/releasehub-production-overlay >/tmp/releasehub-production.yaml
```

Migrate Job 使用 Argo CD `PreSync` hook 與 sync wave `-10`，其餘 workload 使用預設 wave。由 Argo CD 管理該 overlay 時，只有 Migrate 成功後才會繼續同步其他 workload。

不要把完整 Kustomize base 或 overlay 直接串給 `kubectl apply`。普通 `kubectl` 不執行 Argo CD hook 語意，無法保證 migration 先完成。目前 repository 沒有提供非 Argo CD 的分階段 Kustomize pipeline；若必須使用其他部署器，需先建立能分別執行 Migrate、等待 Job 完成，再更新 API、Worker 與 Web 的部署流程。Migration 失敗或逾時時必須停止。

Web 的 `/healthz` 與 `/readyz`、API 的 `/healthz` 與 `/readyz` 可供 Kubernetes 或 ALB probe 使用。成功的 probe 不寫 access log；失敗回應仍保留診斷資訊。API 服務名稱需能由 Web 解析為 `releasehub-api:7580`。

部署後的 Queue、`Partial Failed`、人工解鎖、Argo CD 中斷與 Forward Rollback 處理方式請參閱[操作 Runbook](operations-runbook.md)。

## 部署描述驗證

安裝 Helm、Kustomize、kubeconform、Trivy 與 yq 後執行：

```bash
make verify-deployments
make verify-deployment-security
```

第一個 target 驗證 Helm／Kustomize 可渲染且 workload、Service、Worker 與通知設定等價；第二個 target 以 kubeconform 驗證 Kubernetes schema、以 Trivy 阻擋 High／Critical misconfiguration，並確認 SSE proxy 已關閉 buffering／cache 且保留長連線 timeout。

完成部署描述驗證不代表外部整合已可用。Production 上線前仍需在隔離環境驗證 OIDC、Argo CD hard refresh／Sync、ECR lookup、通知 SSE、migration 失敗及 Worker restart。
