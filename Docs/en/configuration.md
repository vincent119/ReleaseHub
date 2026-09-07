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

