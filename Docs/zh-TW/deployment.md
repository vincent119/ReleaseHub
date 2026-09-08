# 部署

## 建置 image

```bash
docker build -t releasehub-server:local Server
docker build -t releasehub-web:local Web
```

兩個 image 都以 non-root user 執行。Server image 由 API、Worker、Migrate 共用，並以第一個 command argument 選擇 component。

## Helm

先建立 `releasehub-secrets`，再覆寫連線位址、AWS 範圍、image repository/tag 與 ServiceAccount annotations：

```bash
helm upgrade --install releasehub Deployments/helm/releasehub \
  --namespace releasehub --create-namespace \
  --values values.production.yaml
```

Helm 將 Migrate 建立為 `pre-install,pre-upgrade` hook；migration 失敗時不應啟動新版 API 或 Worker。

`configuration.worker` 可設定 reconciliation、Queue polling、lease、retry、Application lock 與 Application 並行上限；`configuration.notifications` 可設定通知保存與投影間隔。第一階段的 `worker.replicas` 必須保持 `1`。

## Kustomize

建立 production overlay，patch ConfigMap、ServiceAccount annotations 與 image 名稱；不要直接把環境 Secret 寫入 base。

```bash
kustomize build Deployments/kustomize/base | kubectl apply -f -
```

Migrate Job 使用 Argo CD `PreSync` hook 與 sync wave `-10`，其餘 workload 使用預設 wave。非 Argo CD 部署者必須先執行 Migrate 並等待成功，再更新 API、Worker 與 Web。

Web 的 `/healthz` 與 `/readyz`、API 的 `/healthz` 與 `/readyz` 可供 Kubernetes 或 ALB probe 使用，這些路徑不寫 access log。API 服務名稱需能由 Web 解析為 `releasehub-api:7580`。

部署後的 Queue、`Partial Failed`、人工解鎖、Argo CD 中斷與 Forward Rollback 處理方式請參閱[操作 Runbook](operations-runbook.md)。

## 部署描述驗證

安裝 Helm、Kustomize、kubeconform、Trivy 與 yq 後執行：

```bash
make verify-deployments
make verify-deployment-security
```

第一個 target 驗證 Helm／Kustomize 可渲染且 workload、Service、Worker 與通知設定等價；第二個 target 以 kubeconform 驗證 Kubernetes schema、以 Trivy 阻擋 High／Critical misconfiguration，並確認 SSE proxy 已關閉 buffering／cache 且保留長連線 timeout。
