# Amazon ECR Integration

## Purpose and Scope

ReleaseHub uses Amazon ECR to verify images found in Argo CD target manifests and resolve tags to immutable digests. Phase one is restricted to one AWS account and one region. `aws.ecr_repositories` is an explicit allow-list, and images outside this boundary fail closed.

## Identity and Permissions

ReleaseHub uses the AWS default credential chain with Pod Identity or IRSA and never stores static AWS credentials. The ECR data permission is limited to `ecr:DescribeImages`. For private ECR, the AWS credential provider still needs the STS permissions required by its runtime environment.

## Resolution and Failure Handling

1. Worker reads container image references from Argo CD target manifests.
2. Worker validates the registry, AWS account, region, and repository allow-list.
3. Worker calls ECR `DescribeImages` with the exact tag or digest.
4. A successful result is stored as an immutable digest snapshot for Deployment Request creation and preflight comparison.

When ECR is unavailable, an image is missing, a repository is outside the allowed scope, or a digest differs, ReleaseHub never guesses or substitutes a version and does not start that deployment. A historical digest removed by an ECR lifecycle policy cannot be selected for Forward Rollback.
