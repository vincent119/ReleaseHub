# Architecture

ReleaseHub provides four independent workloads from two container images:

| Workload | Image | Responsibility |
| --- | --- | --- |
| Web | `releasehub-web` | Static frontend, BFF API reverse proxy, and quiet health checks |
| API | `releasehub-server` | Gin HTTP API, OIDC sessions, RBAC, and management operations |
| Worker | `releasehub-server` | Candidate reconciliation, deployment queue processing, Argo CD execution, and notification projection |
| Migrate | `releasehub-server` | Versioned SQL migrations before application deployment |

API and Worker use separate PostgreSQL connection pools and share the Redis session infrastructure. Worker reads Applications through the Argo CD gRPC API. ECR access is read-only. Migrate is a separate one-shot Job; API and Worker never run migrations during startup.

ReleaseHub detects production release candidates from Argo CD Application target manifests and live state, creates an immutable Deployment Request Version, and calls Argo CD Sync according to the Deployment Plan after the Release Workflow approves it. Before deployment, a hard refresh verifies that the approved revision and image digest have not drifted. Worker then reconciles the execution result.

The GitOps repository and Image Updater keep their existing responsibilities. Image Updater or application CI writes new image versions to the GitOps repository, and Git remains the desired state consumed by Argo CD. ReleaseHub does not write Git, control Image Updater, set image overrides, or use Argo CD history rollback. Restoring an earlier artifact uses Forward Rollback: the source process publishes a new version that follows the normal Deployment Request, approval, and deployment flow.
