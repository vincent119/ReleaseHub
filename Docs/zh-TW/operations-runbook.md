# ReleaseHub 操作 Runbook

## 適用範圍

本文件處理 production Deployment Queue、`Partial Failed`、人工解鎖、Argo CD 中斷及 Forward Rollback。操作者必須具有目標 Organization、Project、Environment 的檢視權限；執行 retry、terminate 或 unlock 時，還需具備畫面顯示的 effective capability。

ReleaseHub 不寫 GitOps repository、不控制 Image Updater、不修改 image tag／digest，也不使用 Argo CD history rollback。不得以重新開啟 automated sync 或直接操作 Argo CD 規避 Release Workflow。

## 共通檢查

先確認 workload 與近期事件。下列命令只讀，不會改變叢集狀態：

```bash
releasehub_namespace=releasehub
kubectl -n "$releasehub_namespace" get deployment,pod \
  -l app.kubernetes.io/name=releasehub
kubectl -n "$releasehub_namespace" get events \
  --sort-by=.metadata.creationTimestamp
```

再從 ReleaseHub Deployment Request 詳細頁記錄：

- Request ID 與 Version。
- Organization、Project、Environment。
- Workflow 與 Plan version。
- 各 Application 的 Queue、Operation、Sync、Health、revision 與實際 image digest。
- 當下可用操作及 configuration drift 通知。

不要只依 Pod Running 判斷部署成功；以 ReleaseHub 對帳後的 Application 結果為準。

## Queue 長時間等待

### 判斷

1. 若 Request 仍在審核或等待 manual transition，這不是 Queue incident，交由有權限的審核者或發佈者處理。
2. 若有相同 Application 的執行仍在進行或鎖定，保持等待；不同 Request 不得互相覆蓋同一 Application。
3. 若 Worker 不可用、Job lease 過期或 Argo CD 無法連線，視為基礎設施事件。

### 處理

讀取 Worker 日誌時，以 `component=worker`、Request ID、Execution ID 或 Application key 篩選：

```bash
kubectl -n "$releasehub_namespace" logs deployment/releasehub-worker \
  --since=30m
```

- Worker 恢復後，等待 lease／fencing 接手及 Argo CD reconciliation。
- 基礎設施 retry 只恢復同一 Job，不得建立新的 Deployment Request，也不得當成 Workflow 允許的 business retry。
- 若 Application 已被 Argo CD 執行，Worker 必須先對帳 operation identity 與 pinned revision；不可盲目重送 Sync。

預期結果為原 Job 被安全接手、完成或明確進入 `Blocked`／失敗狀態。若狀態沒有進展，將上述識別資訊與 Worker 日誌交給平台管理者處理。

## 排程發布或維護時段等待

### 判斷

Deployment Request 清單與詳細頁會分開顯示 requested earliest time、Server 計算的 next eligible time 及原因：

- `ScheduledTime`：尚未到 Request Version 的 `scheduledFor`。
- `MaintenanceWindow`：已達 earliest-start，但目前不在 weekly maintenance window。
- `Blackout`：候選時間落在 blackout。
- `Ready`：目前時間符合 earliest-start 與 policy；實際開始仍需 Queue、lock 與 preflight 成功。

以上值由 Server 計算，瀏覽器顯示時間不代表瀏覽器自行判定 eligibility。Weekly window 依 policy 的 IANA timezone 解讀；blackout 與 next eligible time 是 absolute instant。DST 轉換日必須以畫面顯示的 next eligible time 為準，不要用固定 UTC offset 人工換算。

### 前置條件

- 讀取 policy 需要目標 Environment 的檢視權限；修改需要 `deployment_schedule.manage`。
- 先記錄 Environment、目前 policy version、時區、weekly windows、blackouts、Request ID／Version、requested earliest time 與 next eligible time。
- 確認目標 Request 尚未建立或開始 Execution。已開始的 Execution 不受後續 policy 修改或 window 結束影響。

### 處理

1. 在 Plans 選取相同 Organization、Project 與 Environment，開啟 Deployment Schedule。
2. 確認時區為有效 IANA 名稱，weekly window 未跨日或重疊，blackout 起訖順序正確。
3. 若目前 policy 符合預期，保持等待；不要手動 retry、重新建立 Request、修改 Queue row 或直接觸發 Argo CD Sync。
4. 若業務核准調整 policy，由具權限操作者保存變更。畫面使用目前 version 進行 optimistic update；若收到 `409 Conflict`，保留尚未送出的內容，重新整理最新 policy、比較差異後再決定是否重送，不可覆蓋他人更新。
5. 更新後重新讀取 Request。等待中的 Job 會在下一次 claim 使用最新 policy，next eligible time 可能提前或延後；已開始 Execution 不會被中斷。
6. Worker restart 時不需建立替代 Job。等待 PostgreSQL `available_at`、lease 與 fencing 接續；若 restart 發生於已開始部署，依既有 Execution reconciliation 判斷，不把它改判為 schedule defer。

### 驗證與停止條件

正常 defer 應出現日誌 `Deployment job deferred by schedule`，欄位包含 `job_type`、低基數 `reason`、`wait`、`next_eligible_at` 與 `policy_version`。Prometheus 指標為：

- `releasehub_deployment_schedule_deferred_total{reason=...}`
- `releasehub_deployment_schedule_wait_seconds{reason=...}`

到達 next eligible time 後，Request projection 應變為 `Ready`，Job 才能進入 Executor。若 `reason=Unavailable` 持續出現，表示 366 天搜尋範圍內沒有合法時段；停止進一步放寬或重送，檢查 weekly windows 與長期 blackout。若 policy 看似正確但 next eligible time 不一致，記錄 UTC instant、IANA timezone、policy version 與 DST 邊界後升級給平台管理者。

### 回復方式

Policy 沒有就地 rollback 指令。若新 policy 不正確，透過 Plans 以最新 version 明確寫回前一份已核准內容，產生下一個 version、Audit 與 Outbox event。不得直接更新或刪除 `deployment_schedule_policies`、`deployment_schedule_commands` 或 Queue row。

## Partial Failed

### 判斷

`Partial Failed` 表示同一 Execution 中有 Application 成功、也有 Application 失敗。Project／Environment 的 operation lock 會持續存在，直到 retry 全部成功，或有權限的管理者 terminate 並完成實際狀態核對。

### 處理

1. 在 Application 結果中確認成功與失敗清單、pinned revision、operation、Sync、Health 與實際 digest。
2. 若 Workflow／Plan 與 effective capability 允許 retry，只選擇失敗 Application；成功 Application 不得重送。
3. 若不應 retry，執行 terminate 並填寫原因。
4. Terminate 不會自動解除 operation lock；依「人工解鎖」流程重新讀取所有 Application 的實際狀態。

預期結果為失敗 Application 成功後整體完成，或 Execution 維持已終止且清楚保存版本不一致狀態。不得在未核對實際狀態前開始另一個部署或 Forward Rollback。

## 人工解鎖

人工解鎖是高風險操作，只能用於已 terminate 且無法由正常 retry／reconciliation 收斂的 Execution。

### 前置條件

- 操作者具有 `deployment_request.unlock` effective capability。
- Execution 已 terminate。
- 已確認 Argo CD 沒有仍在執行的相關 operation。
- 已逐一記錄 Application 的實際 revision 與 image digest。

### 處理

1. 在 Request 詳細頁重新整理 Application 狀態。
2. 比對 ReleaseHub 顯示的預期 snapshot 與 Argo CD 回傳的實際 revision／digest。
3. 輸入核對後的實際狀態與強制原因，再送出 unlock。
4. 重新讀取 Request，確認 lock 已解除且 Audit 保存解除前後證據。

只要任一 Application 無法讀取、operation 尚未停止，或 revision／digest 不能確認，ReleaseHub 應拒絕解鎖。不得直接刪除資料庫 lock row。

## Argo CD 中斷

### 判斷

若多個 Application 同時出現 refresh、Sync 或 watch 失敗，先判斷 Argo CD API／controller 是否可用；單一 Application 失敗則優先檢查該 Application 的設定漂移與 operation。

### 處理

1. 暫停新的人工部署操作；不要重開 automated sync。
2. 恢復 Argo CD 服務、repository 存取與 controller，再恢復 ReleaseHub Worker。
3. 等待 Worker hard refresh 與 reconciliation。Worker 必須核對 operation identity 與 pinned revision後才能接續。
4. 若核對失敗，保留 `Blocked` 或失敗狀態，依 Workflow／Plan 可用操作決定 retry 或 terminate。

預期結果是既有 Execution 被對帳，而不是重新建立 Request 或重送所有 Sync。若 Argo CD 回傳狀態不足以證明實際結果，維持 fail closed 並交由平台管理者處理。

## Forward Rollback

Forward Rollback 用新版本恢復歷史 digest，仍是一般 production 發佈，不是緊急旁路。

### 前置條件

- 目標來自完整 `Succeeded` 的歷史 Deployment。
- 所有 Application 的歷史 digest 仍存在於 ECR。
- 原始 repository／CI 能產生符合版本規則的新 tag，並指向選定的歷史 digest。

### 處理

1. 在成功歷史確認所有 Application 的目標 digest。
2. 由原始 repository／CI 發布較新的版本，讓 Image Updater 依既有責任寫回 GitOps repository。
3. 等待 Argo CD 解析新的 target manifests；ReleaseHub 將其偵測為 Candidate，建立新的 Deployment Request Version，並分類為 Forward Rollback。
4. 依目前綁定的 Release Workflow 審核，再依 Deployment Plan 部署。

若任一 digest 已被 ECR lifecycle policy 刪除，整批停止並改選另一個完整成功版本。不得由 ReleaseHub 寫 Git、設定 image override、使用 Argo CD history rollback，或只回復部分 Application。

## Runbook Dry-run

每次部署文件異動後，在無 production side effect 的環境完成：

```bash
helm lint Deployments/helm/releasehub
helm template releasehub Deployments/helm/releasehub \
  --namespace releasehub >/tmp/releasehub-helm.yaml
kustomize build Deployments/kustomize/base \
  >/tmp/releasehub-kustomize.yaml
```

接著逐段演練 Queue、排程等待、policy optimistic conflict、Worker restart、DST 邊界、`Partial Failed`、terminate、unlock、Argo CD 中斷及 Forward Rollback 的判斷點。Dry-run 不修改 production policy，不執行 Sync、retry、terminate、unlock、Git write-back 或叢集套用；只確認命令可讀、角色與必要證據完整。缺少測試環境時，將實際故障注入與 Argo CD／ECR E2E 保留給整合驗證，不得據此宣稱 production 演練完成。
