# Configuration

## Loading and Precedence

Server reads `/app/configs/config.yaml` by default. Precedence from lowest to highest is defaults, YAML, `RELEASEHUB_` environment variables, and CLI flags. Nested fields use underscores, so `database.password` maps to `RELEASEHUB_DATABASE_PASSWORD`.

The complete non-secret example is [`Server/configs/config.example.yaml`](../../Server/configs/config.example.yaml). API, Worker, and Migrate share the same typed configuration, while each component uses its configured database pool.

## Initial Manager

Set the top-level `manager_password` value before the API starts against an initialized database. When the initial-manager records are missing, the API hashes this value and uses GORM to create the `admin` account, its local credential, and its `platform_administrator` membership. Existing credentials are never overwritten during restart.

For local development, setting `manager_password: admin` creates the initial `admin / admin` login. The first login is restricted to changing the password or signing out. The new password must contain 8 to 72 bytes, and changing it revokes all active Sessions. On a new database, leaving `manager_password` empty skips local-manager bootstrap; it does not remove an existing local credential.

## Local API Startup

After applying database migrations, use the following local-only settings when testing local authentication over HTTP:

```yaml
manager_password: admin

database:
  ssl_mode: disable

oidc:
  issuer: ""
  client_id: ""
  client_secret: ""
  redirect_url: ""
  web_redirect_url: "http://localhost:5173/"
  logout_url: ""
  post_logout_redirect_url: ""

argocd:
  address: ""
  token: ""

session:
  encryption_key: ""
  cookie_secure: false
```

From the repository root, start API and Web together:

```bash
make dev
```

`make dev-api` generates an ephemeral Session encryption key when the environment does not already provide one, disables Secure cookies for local HTTP, and disables an incomplete OIDC configuration. `make dev-web` starts Vite and proxies `/api` to `http://127.0.0.1:7580`. Make uses `pnpm` from `PATH` when available and otherwise falls back to `corepack pnpm`; if neither can run, the preflight check stops before API starts.

To start only the API without Make, generate a Session encryption key in the current shell first:

```bash
export RELEASEHUB_SESSION_ENCRYPTION_KEY="$(openssl rand -hex 32)"
go run ./cmd/releasehub api --config ./configs/config.yaml
```

The key must contain at least 32 bytes and remains available only in the current shell. Keep `oidc.redirect_url` empty when OIDC is disabled; otherwise the API treats OIDC as partially configured and stops. When both `argocd.address` and `argocd.token` are empty, API starts without an Argo CD client and Argo CD-dependent operations return `503 Service Unavailable`. Worker still requires complete Argo CD configuration. Override `DEV_API_ADDRESS`, `DEV_API_PROXY_TARGET`, `DEV_WEB_PORT`, `DEV_WEB_URL`, or `SERVER_CONFIG` when the defaults do not match the local environment.

## Secrets and External Dependencies

The deployment templates require an existing Kubernetes Secret named `releasehub-secrets` with at least these keys:

- `RELEASEHUB_DATABASE_PASSWORD`
- `RELEASEHUB_REDIS_PASSWORD`
- `RELEASEHUB_OIDC_CLIENT_SECRET`
- `RELEASEHUB_ARGOCD_TOKEN`
- `RELEASEHUB_SESSION_ENCRYPTION_KEY`

Secrets must not be stored in values, the Kustomize base, container images, or Git. Non-secret settings belong in the ConfigMap. Before enabling Worker, configure a valid Argo CD address and token plus the allowed ECR repositories in one AWS account and region.

Sessions, the queue, Audit, and Outbox records are currently stored in PostgreSQL. The schema still requires a valid `redis.address` and reserves pool fields, but the runtime does not create a Redis client. Do not treat Redis as the current session or queue store.

## Logging and Observability

`log.format` accepts `json` or `console`. API and Worker application logs include their component category. Successful `/healthz`, `/readyz`, and configured metrics-path probes do not emit access logs, request metrics, or traces. Failed responses retain diagnostic observations. When enabled, tracing exports through OTLP gRPC; ReleaseHub does not expose a separate tracing HTTP endpoint.

## Worker and Notifications

| YAML field                          | Default | Purpose                                                                   |
| ----------------------------------- | ------- | ------------------------------------------------------------------------- |
| `worker.reconcile_interval`         | `30s`   | Reload Argo CD Applications and detect candidates or configuration drift  |
| `worker.deployment_poll_interval`   | `1s`    | Poll for claimable deployment jobs                                        |
| `worker.job_lease_duration`         | `30s`   | Lifetime of job leases and fencing ownership                              |
| `worker.job_retry_delay`            | `5s`    | Delay before an infrastructure failure is requeued                        |
| `worker.application_lock_duration`  | `30s`   | Lifetime of Application operation locks                                   |
| `worker.max_parallel_deployments`   | `10`    | Maximum Applications executed by one Worker; a Plan may set a lower limit |
| `notifications.retention`           | `168h`  | Retention for in-app notifications; Audit is unaffected                   |
| `notifications.projection_interval` | `1s`    | Interval for Outbox projection and availability of SSE events             |

Phase one must run exactly one Worker replica. Multiple Worker replicas would each apply `max_parallel_deployments`, so the platform-wide limit could not be guaranteed. Web Nginx proxies the exact SSE path `/api/v1/notifications/events` with buffering and caching disabled. After reconnecting with `Last-Event-ID`, the client fetches data again under the user's current permissions.

## Environment Deployment Schedule

A Deployment Schedule is an Environment policy stored in PostgreSQL. It is not a Server YAML field or environment variable. Select an Organization, Project, and Environment, then manage the policy on the Plans page. When no policy exists, the API returns an unrestricted default with `enabled=false`, `timeZone=UTC`, and `version=0`. Disabling a policy keeps its document but applies only the Request Version `scheduledFor` earliest-start constraint.

Policy fields and limits:

| Field                      | Meaning and limits                                                                                                                        |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                  | Enables the weekly-window and blackout gate. An enabled policy requires at least one weekly window.                                       |
| `timeZone`                 | An IANA time zone such as `Asia/Taipei`. Weekly windows use this local time zone.                                                         |
| `weeklyWindows`            | At most 32 entries. `dayOfWeek` is `0` through `6` for Sunday through Saturday. Windows on the same day cannot overlap or cross midnight. |
| `startMinute`／`endMinute` | Local minute of day. The ranges are `0..1439` and `1..1440`. Starts are inclusive and ends are exclusive.                                 |
| `blackouts`                | At most 64 entries. Start and end are RFC 3339 absolute times normalized to UTC by Server. Starts are inclusive and ends are exclusive.   |
| `version`                  | Server-managed optimistic version. An update sends the prior version as `expectedVersion`.                                                |

Reading a policy requires `deployment_request.view` or `deployment_schedule.manage` on the target scope. Updating it requires `deployment_schedule.manage`. PUT also requires a Session, CSRF token, `Idempotency-Key`, and `expectedVersion`. The same actor and idempotency key replay the original result only when the payload matches. A stale version or a key/payload conflict returns `409 Conflict`. A missing Environment and an unauthorized Environment both return a masked `404 Not Found`.

Every successful update writes the policy, Audit action `deployment_schedule.updated`, and an Outbox event in one database transaction. Audit metadata contains only the version and weekly-window and blackout counts, not the complete policy.

## Startup Validation

The Server fails before creating a runtime when the database, Redis address, session encryption key, or component-specific OIDC, Argo CD, or AWS settings are missing. Before deployment, verify that:

1. The ConfigMap contains no secret values.
2. Every required key exists in `releasehub-secrets`.
3. PostgreSQL is reachable and versioned migrations have completed according to [Database Initialization](database-initialization.md).
4. When OIDC is enabled, its redirect URL matches the externally reachable ReleaseHub URL.
5. Argo CD and ECR identities follow least privilege.
6. Migrations created `deployment_schedule_policies`, `deployment_schedule_commands`, and the `deployment_schedule.manage` permission. This feature adds no Server runtime configuration field.
