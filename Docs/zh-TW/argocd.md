# Argo CD 整合

Worker 使用專用 Argo CD account 與 gRPC token。Application 必須精確設定：

```yaml
metadata:
  labels:
    releasehub.io/managed: "true"
```

Worker 讀取 Application 清單、Sync／Health／Operation 狀態、source／destination mapping 與 target manifests。候選 Application 仍需由管理者完成 mapping 驗證及人工確認。Production onboarding 確認後，ReleaseHub 只會移除 `spec.syncPolicy.automated`，再讀回驗證；不建立或刪除 Application，也不修改 Argo CD Project。

若 label 被移除、automated sync 被重開、Application 消失或 mapping 改變，Worker 只標記 configuration drift 並通知，不會自行修正。ReleaseHub 不寫 GitOps repository、不控制 Image Updater、不改 image tag/digest，也不觸發正式 sync。

