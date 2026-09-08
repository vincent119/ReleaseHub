# ReleaseHub

ReleaseHub 是以 Argo CD 為部署執行端的發佈治理平台。本 repository 目前處於平台基礎建置階段。

## Repository 結構

- `Server/`：Go API、Worker 與 Migration command。
- `Web/`：React Web 與 Nginx runtime。
- `API/`：OpenAPI 契約來源。
- `Deployments/`：Helm 與 Kustomize 部署描述。
- `Docs/zh-TW/`、`Docs/en/`：中英文技術文件。

## 產生 API 程式碼

```bash
make generate
```

## 驗證 OpenAPI

```bash
make lint-openapi
```

## 文件

- [架構](Docs/zh-TW/architecture.md)
- [設定](Docs/zh-TW/configuration.md)
- [OIDC](Docs/zh-TW/oidc.md)
- [Argo CD 整合](Docs/zh-TW/argocd.md)
- [Amazon ECR 整合](Docs/zh-TW/ecr.md)
- [部署](Docs/zh-TW/deployment.md)
- [操作 Runbook](Docs/zh-TW/operations-runbook.md)

完整整合驗證可執行：

```bash
make verify
```

## 授權

授權方式尚未決定，因此目前沒有 `LICENSE` 檔案。
