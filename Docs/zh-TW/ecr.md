# Amazon ECR 整合

第一階段限制為單一 AWS account 與單一 region。`aws.ecr_repositories` 是明確 allow-list，target manifest 中不屬於此範圍的 image 會失敗封閉。

ReleaseHub 使用 Pod Identity 或 IRSA 的 AWS default credential chain，不保存 static AWS credentials。所需 ECR data permission 只有 `ecr:DescribeImages`；若使用 private ECR，AWS credential provider 仍需取得其執行環境所要求的 STS permission。

ReleaseHub 從 Argo CD target manifests 取得 container image reference，再以精確 tag 或 digest 查詢 ECR。解析結果保存為不可變快照。ECR 不可用、image 不存在或 digest 不一致時，不會猜測或替代版本。

