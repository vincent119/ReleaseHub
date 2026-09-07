# Architecture

ReleaseHub provides four independent workloads from two container images:

| Workload | Image | Responsibility |
| --- | --- | --- |
| Web | `releasehub-web` | Static frontend, BFF API reverse proxy, and quiet health checks |
| API | `releasehub-server` | Gin HTTP API, OIDC sessions, RBAC, and management operations |
| Worker | `releasehub-server` | Argo CD reconciliation, configuration drift, and ECR digest resolution |
| Migrate | `releasehub-server` | Versioned SQL migrations before application deployment |

API and Worker use separate PostgreSQL connection pools and share the Redis session infrastructure. Worker reads Applications through the Argo CD gRPC API. ECR access is read-only. Migrate is a separate one-shot Job; API and Worker never run migrations during startup.

The current boundary covers platform foundation, Application discovery, onboarding, configuration drift, and target image digest snapshots. ReleaseHub does not modify a GitOps repository, control Argo CD Image Updater, set image overrides, or trigger a production release.

