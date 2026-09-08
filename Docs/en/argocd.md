# Argo CD Integration

Worker uses a dedicated Argo CD account and gRPC token. An Application must carry this exact label:

```yaml
metadata:
  labels:
    releasehub.io/managed: "true"
```

Worker reads the Application list, Sync, Health, Operation state, source and destination mappings, and target manifests. An administrator must still validate and confirm the candidate mapping. After production onboarding confirmation, ReleaseHub only removes `spec.syncPolicy.automated` and verifies the result with a readback. It does not create or delete Applications or modify Argo CD Projects.

If the label is removed, automated sync is enabled again, the Application disappears, or its mapping changes, Worker records configuration drift and emits a notification without repairing the change automatically. While drift exists, creating a Deployment Request, deploying, and performing Forward Rollback are blocked.

Application onboarding does not trigger production Sync. After an immutable Deployment Request Version passes its Release Workflow, Worker requests an Argo CD hard refresh, verifies the approved Application revision, target manifests, and ECR digest, and then syncs the pinned revision according to the Deployment Plan. ReleaseHub does not write to a GitOps repository, control Image Updater, change image tags or digests, or use Argo CD history rollback.
