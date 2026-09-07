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

## Kustomize

建立 production overlay，patch ConfigMap、ServiceAccount annotations 與 image 名稱；不要直接把環境 Secret 寫入 base。

```bash
kustomize build Deployments/kustomize/base | kubectl apply -f -
```

Migrate Job 使用 Argo CD `PreSync` hook 與 sync wave `-10`，其餘 workload 使用預設 wave。非 Argo CD 部署者必須先執行 Migrate 並等待成功，再更新 API、Worker 與 Web。

Web 的 `/healthz` 與 `/readyz`、API 的 `/healthz` 與 `/readyz` 可供 Kubernetes 或 ALB probe 使用，這些路徑不寫 access log。API 服務名稱需能由 Web 解析為 `releasehub-api:7580`。
