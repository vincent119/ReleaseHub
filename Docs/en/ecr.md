# Amazon ECR Integration

The first phase is restricted to one AWS account and one region. `aws.ecr_repositories` is an explicit allow-list. An image outside this boundary in an Argo CD target manifest fails closed.

ReleaseHub uses the AWS default credential chain with Pod Identity or IRSA and never stores static AWS credentials. The ECR data permission is limited to `ecr:DescribeImages`. For private ECR, the AWS credential provider still needs the STS permissions required by its runtime environment.

ReleaseHub reads container image references from Argo CD target manifests and queries ECR by the exact tag or digest. The resolved result is stored as an immutable snapshot. ReleaseHub never guesses or substitutes a version when ECR is unavailable, an image is missing, or a digest differs.

