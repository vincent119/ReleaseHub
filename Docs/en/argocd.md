# Argo CD Integration

Worker uses a dedicated Argo CD account and gRPC token. An Application must carry this exact label:

```yaml
metadata:
  labels:
    releasehub.io/managed: "true"
```

Worker reads the Application list, Sync, Health, Operation state, source and destination mappings, and target manifests. An administrator must still validate and confirm the candidate mapping. After production onboarding confirmation, ReleaseHub only removes `spec.syncPolicy.automated` and verifies the result with a readback. It does not create or delete Applications or modify Argo CD Projects.

If the label is removed, automated sync is enabled again, the Application disappears, or its mapping changes, Worker only records configuration drift and emits a notification. It does not repair the change automatically. ReleaseHub does not write to a GitOps repository, control Image Updater, change image tags or digests, or trigger a production sync.

