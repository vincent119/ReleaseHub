# ReleaseHub Operations Runbook

The normal path is in the [Production Release Guide](release-flow.md). This runbook starts when a Queue waits, an Execution fails, or an operator must recover it.

## Scope

This runbook covers the production Deployment Queue, `Partial Failed`, manual unlock, Argo CD outages, and Forward Rollback. Operators need read access to the target Organization, Project, and Environment. Retry, terminate, and unlock also require the effective capability shown by the UI.

ReleaseHub does not write to a GitOps repository, control Image Updater, change image tags or digests, or use Argo CD history rollback. Never bypass the Release Workflow by enabling automated sync or operating directly in Argo CD.

## Common Checks

First inspect workloads and recent events. These commands are read-only:

```bash
releasehub_namespace=releasehub
kubectl -n "$releasehub_namespace" get deployment,pod \
  -l app.kubernetes.io/name=releasehub
kubectl -n "$releasehub_namespace" get events \
  --sort-by=.metadata.creationTimestamp
```

Record the following from the Deployment Request detail page:

- Request ID and Version.
- Organization, Project, and Environment.
- Workflow and Plan version.
- Queue, Operation, Sync, Health, revision, and actual image digest for every Application.
- Available actions and configuration drift notifications.

A Running Pod alone does not prove deployment success. Use the reconciled Application result shown by ReleaseHub.

## Queue Waits Too Long

### Decision

1. A Request waiting for review or a manual transition is not a queue incident. The authorized reviewer or deployer must act.
2. If another execution still operates or locks the same Application, keep waiting. A later Request must not overwrite it.
3. Treat unavailable Worker replicas, expired job leases, or failed Argo CD connectivity as an infrastructure incident.

### Procedure

Read Worker logs and filter by `component=worker`, Request ID, Execution ID, or Application key:

```bash
kubectl -n "$releasehub_namespace" logs deployment/releasehub-worker \
  --since=30m
```

- After Worker recovers, wait for lease and fencing takeover followed by Argo CD reconciliation.
- Infrastructure retry resumes the same job. It must not create a Deployment Request or become a business retry that bypasses Workflow policy.
- If Argo CD may have started the Application operation, Worker must reconcile the operation identity and pinned revision before sending anything else.

The expected result is safe takeover followed by completion or an explicit `Blocked` or failed state. If progress does not resume, escalate the recorded identifiers and Worker logs to a platform administrator.

## Scheduled Release or Maintenance Window Wait

### Decision

The Deployment Request list and detail page show the requested earliest time, the Server-computed next eligible time, and a reason as separate values:

- `ScheduledTime`: the Request Version `scheduledFor` time has not arrived.
- `MaintenanceWindow`: earliest-start has arrived, but the current time is outside a weekly maintenance window.
- `Blackout`: the candidate time is inside a blackout.
- `Ready`: earliest-start and policy allow the current time. Queue, lock, and preflight checks must still succeed before execution starts.

These values are computed by Server. Browser time formatting does not mean that the browser determines eligibility. Weekly windows use the policy IANA time zone. Blackouts and next eligible times are absolute instants. On a DST transition day, use the displayed next eligible instant instead of manually applying a fixed UTC offset.

### Preconditions

- Reading the policy requires view access to the Environment. Updating it requires `deployment_schedule.manage`.
- Record the Environment, current policy version, time zone, weekly windows, blackouts, Request ID and Version, requested earliest time, and next eligible time.
- Confirm that the target Request has not created or started an Execution. A started Execution is not affected by later policy changes or by a closing window.

### Procedure

1. On Plans, select the same Organization, Project, and Environment, then open Deployment Schedule.
2. Confirm that the time zone is a valid IANA name, weekly windows neither cross midnight nor overlap, and every blackout has an ordered start and end.
3. If the policy matches the approved schedule, wait. Do not manually retry, create a replacement Request, edit queue rows, or trigger Argo CD Sync directly.
4. If the business approves a policy change, an authorized operator saves it. The page submits the current version as an optimistic update. On `409 Conflict`, keep the unsent input, reload the latest policy, compare changes, and decide whether to resubmit. Do not overwrite another operator's update.
5. Reload the Request after the update. A waiting job uses the latest policy on its next claim, so the next eligible time may move earlier or later. A started Execution is not interrupted.
6. A Worker restart does not require a replacement job. Allow PostgreSQL `available_at`, leases, and fencing to continue the wait. If the restart occurs during a deployment, use normal Execution reconciliation instead of treating it as a schedule defer.

### Validation and Stop Conditions

A normal defer emits `Deployment job deferred by schedule` with `job_type`, low-cardinality `reason`, `wait`, `next_eligible_at`, and `policy_version`. Prometheus exposes:

- `releasehub_deployment_schedule_deferred_total{reason=...}`
- `releasehub_deployment_schedule_wait_seconds{reason=...}`

At the next eligible time, the Request projection becomes `Ready` before the job can enter the Executor. Repeated `reason=Unavailable` means that no valid time exists within the bounded 366-day search. Stop attempts to loosen or resubmit the job and inspect weekly windows and long blackouts. If the policy appears correct but the next eligible instant does not match, record the UTC instant, IANA time zone, policy version, and DST boundary, then escalate to a platform administrator.

### Recovery

There is no in-place policy rollback command. To recover from an incorrect policy, use Plans with the latest version to explicitly write the previously approved content. This creates a new version, Audit record, and Outbox event. Do not directly update or delete `deployment_schedule_policies`, `deployment_schedule_commands`, or queue rows.

## Partial Failed

### Decision

`Partial Failed` means that the same Execution contains both successful and failed Applications. The Project and Environment operation lock remains until retry succeeds for all failed Applications, or an authorized administrator terminates the execution and verifies actual state.

### Procedure

1. Identify the successful and failed Applications and record pinned revision, operation, Sync, Health, and actual digest.
2. If the Workflow, Plan, and effective capability allow retry, select failed Applications only. Never resend successful Applications.
3. If retry is not appropriate, terminate the execution and provide a reason.
4. Termination does not release the operation lock. Follow the manual unlock procedure and reread actual state for every Application.

The expected result is full success after failed-only retry, or a terminated Execution that preserves the inconsistent actual state. Do not start another deployment or Forward Rollback before actual-state verification.

## Manual Unlock

Manual unlock is a high-risk operation. Use it only when a terminated Execution cannot converge through normal retry or reconciliation.

### Preconditions

- The operator has the `deployment_request.unlock` effective capability.
- The Execution is terminated.
- Argo CD has no related operation still running.
- The actual revision and image digest of every Application are recorded.

### Procedure

1. Refresh Application state on the Request detail page.
2. Compare the approved snapshot with the actual revision and digest returned by Argo CD.
3. Enter the verified actual state and a mandatory reason, then submit unlock.
4. Reload the Request and confirm that the lock is released and Audit retains the before-and-after evidence.

ReleaseHub must reject unlock when an Application is unreadable, an operation is still active, or revision and digest cannot be verified. Never delete a database lock row directly.

## Argo CD Outage

### Decision

When refresh, Sync, or watch fails across multiple Applications, check the Argo CD API and controllers first. For one affected Application, inspect its configuration drift and operation state first.

### Procedure

1. Pause new manual deployments. Do not enable automated sync.
2. Restore Argo CD services, repository access, and controllers before recovering ReleaseHub Worker.
3. Wait for Worker hard refresh and reconciliation. Worker must match operation identity and pinned revision before continuing.
4. If reconciliation cannot prove the state, retain `Blocked` or failed status and use only the retry or terminate actions allowed by the Workflow and Plan.

The expected result is reconciliation of the existing Execution, not a replacement Request or a resend of every Sync. If Argo CD cannot provide enough evidence, remain fail closed and escalate to a platform administrator.

## Forward Rollback

Forward Rollback restores a historical digest through a new version. It remains a normal production release, not an emergency bypass.

### Preconditions

- The target is a fully `Succeeded` historical Deployment.
- Every historical Application digest still exists in ECR.
- The source repository and CI can publish a higher version tag that references the selected historical digest.

### Procedure

1. Confirm the target digest of every Application in successful history.
2. Publish a higher version from the source repository and CI. Image Updater then writes the version to the GitOps repository under its existing responsibility.
3. Wait for Argo CD to resolve the new target manifests. ReleaseHub detects the candidate, creates a new Deployment Request Version, and classifies it as Forward Rollback.
4. Follow the currently bound Release Workflow and then deploy through the Deployment Plan.

If any digest was removed by an ECR lifecycle policy, stop the entire operation and choose another fully successful version. ReleaseHub must not write Git, set an image override, invoke Argo CD history rollback, or restore only part of the Application set.

## Runbook Dry-run

After changing deployment documentation, run these checks without production side effects:

```bash
helm lint Deployments/helm/releasehub
helm template releasehub Deployments/helm/releasehub \
  --namespace releasehub >/tmp/releasehub-helm.yaml
kustomize build Deployments/kustomize/base \
  >/tmp/releasehub-kustomize.yaml
```

Walk through each decision for queue waits, schedule waits, policy optimistic conflicts, Worker restarts, DST boundaries, `Partial Failed`, terminate, unlock, Argo CD outages, and Forward Rollback. A dry-run must not change a production policy, perform Sync, retry, terminate, unlock, Git write-back, or cluster apply. It only confirms that commands are readable and that each procedure names the required role and evidence. Without a test environment, leave failure injection and Argo CD or ECR end-to-end checks for integration validation; do not claim that a production drill was completed.
