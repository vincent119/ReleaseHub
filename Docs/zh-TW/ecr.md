# Amazon ECR 整合

## 目的與範圍

ReleaseHub 使用 Amazon ECR 驗證 Argo CD target manifests 中的 image 是否存在，並將 tag 解析為不可變 digest。第一階段限制為單一 AWS account 與單一 region；`aws.ecr_repositories` 是明確 allow-list，範圍外的 image 會失敗封閉。

## 身分與權限

ReleaseHub 使用 Pod Identity 或 IRSA 的 AWS default credential chain，不保存 static AWS credentials。所需 ECR data permission 只有 `ecr:DescribeImages`；若使用 private ECR，AWS credential provider 仍需取得其執行環境要求的 STS permission。

## 解析與失敗處理

1. Worker 從 Argo CD target manifests 取得 container image reference。
2. Worker 驗證 registry、AWS account、region 與 repository allow-list。
3. Worker 以精確 tag 或 digest 呼叫 ECR `DescribeImages`。
4. 成功結果保存為不可變 digest 快照，供建立 Deployment Request 及部署前比對。

ECR 不可用、image 不存在、repository 超出範圍或 digest 不一致時，ReleaseHub 不猜測、不替代版本，也不執行該次部署。ECR lifecycle policy 已刪除歷史 digest 時，該版本不能作為 Forward Rollback 目標。
