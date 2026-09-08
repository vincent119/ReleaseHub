# Configuration

Server reads `/app/configs/config.yaml` by default. Precedence from lowest to highest is defaults, YAML, `RELEASEHUB_` environment variables, and CLI flags. Nested fields use underscores, so `database.password` maps to `RELEASEHUB_DATABASE_PASSWORD`.

The deployment templates require an existing Kubernetes Secret named `releasehub-secrets` with at least these keys:

- `RELEASEHUB_DATABASE_PASSWORD`
- `RELEASEHUB_REDIS_PASSWORD`
- `RELEASEHUB_OIDC_CLIENT_SECRET`
- `RELEASEHUB_ARGOCD_TOKEN`
- `RELEASEHUB_SESSION_ENCRYPTION_KEY`

Secrets must not be stored in values, the Kustomize base, container images, or Git. Non-secret settings belong in the ConfigMap. Before enabling Worker, configure a valid Argo CD address and token plus the allowed ECR repositories in one AWS account and region.

`log.format` accepts `json` or `console`. API and Worker logs include their component category. Health, readiness, metrics, and tracing endpoints do not emit access logs.

## Worker and Notifications

| YAML field | Default | Purpose |
| --- | --- | --- |
| `worker.reconcile_interval` | `30s` | Reload Argo CD Applications and detect candidates or configuration drift |
| `worker.deployment_poll_interval` | `1s` | Poll for claimable deployment jobs |
| `worker.job_lease_duration` | `30s` | Lifetime of job leases and fencing ownership |
| `worker.job_retry_delay` | `5s` | Delay before an infrastructure failure is requeued |
| `worker.application_lock_duration` | `30s` | Lifetime of Application operation locks |
| `worker.max_parallel_deployments` | `10` | Maximum Applications executed by one Worker; a Plan may set a lower limit |
| `notifications.retention` | `168h` | Retention for in-app notifications; Audit is unaffected |
| `notifications.projection_interval` | `1s` | Interval for Outbox projection and availability of SSE events |

Phase one must run exactly one Worker replica. Multiple Worker replicas would each apply `max_parallel_deployments`, so the platform-wide limit could not be guaranteed. Web Nginx proxies the exact SSE path `/api/v1/notifications/events` with buffering and caching disabled. After reconnecting with `Last-Event-ID`, the client fetches data again under the user's current permissions.
