# ReleaseHub

[繁體中文](README.zh-TW.md)

ReleaseHub is a release-governance platform that uses Argo CD as its deployment executor. It gives platform teams one place to manage Application onboarding, production Deployment Requests, approvals, deployment order, execution state, and audit evidence. The GitOps repository remains the source of desired state: ReleaseHub does not write Git, control Image Updater, or replace the existing GitOps flow with image overrides.

## Current Capabilities

- Discover and onboard Argo CD Applications carrying the managed label.
- Create immutable Deployment Request Versions from Argo CD target manifests and ECR digests.
- Use Release Workflows for approvals and manual or automatic transitions.
- Use Deployment Plan DAGs for Application dependencies and concurrency limits.
- Perform hard refresh, content-drift checks, pinned-revision Sync, and result reconciliation.
- Handle `Partial Failed`, failed-Application retry, terminate, manual unlock, and Forward Rollback.
- Govern access and evidence through OIDC, Group／Role／Scope／Deny, in-app notifications, and Audit.
- Bootstrap a local `admin` account from `manager_password`, with a mandatory password change after the first login.

## Development Verification

### Prerequisites

| Tool | Source of requirement |
| --- | --- |
| Go `1.26.6` | `Server/go.mod` |
| Node.js `22.22.0` or later | `Web/package.json` |
| pnpm `11.19.0` | `Web/package.json` |
| Docker | PostgreSQL Testcontainers integration tests |
| Helm, Kustomize, kubeconform, Trivy, and yq | Deployment manifest validation |

Run from the repository root:

```bash
cd Web
pnpm install --frozen-lockfile
cd ../Server
go mod download
cd ..
make verify
```

A successful `make verify` exits with code `0` after checking generated OpenAPI artifacts, Server and Web tests, lint, builds, bilingual document pairs, and Helm and Kustomize schema and security settings. Run `make help` to list individual targets.

A running environment also requires PostgreSQL, Argo CD, AWS ECR, and Kubernetes. OIDC is optional when local authentication is enabled. Continue with [Configuration](Docs/en/configuration.md) and [Deployment](Docs/en/deployment.md).

For local initialization, set `manager_password: admin` to create `admin / admin`. The first login must change this password before any other authenticated operation is allowed. See [Configuration](Docs/en/configuration.md#initial-manager).

## Repository Structure

- `Server/`: Go API, Worker, and Migrate commands.
- `Web/`: React web application and Nginx runtime.
- `API/`: Source OpenAPI contract.
- `Deployments/`: Helm and Kustomize deployment definitions.
- `Docs/zh-TW/` and `Docs/en/`: Bilingual technical documentation.

## Documentation Paths

- First deployment: [Configuration](Docs/en/configuration.md) → [Deployment](Docs/en/deployment.md) → [Operations Runbook](Docs/en/operations-runbook.md)
- Platform and data flow: [Architecture](Docs/en/architecture.md)
- Application integration: [Argo CD](Docs/en/argocd.md) → [Amazon ECR](Docs/en/ecr.md)
- Identity and sessions: [OIDC](Docs/en/oidc.md)

## Phase-one Limits

- One Argo CD instance.
- One AWS account and one region.
- Exactly one Worker replica, preserving the platform-wide Application concurrency limit.
- Repository checks do not replace end-to-end validation or a production drill in an isolated Argo CD and ECR environment.

## Development Commands

```bash
make generate       # Regenerate Go and TypeScript OpenAPI code
make lint-openapi   # Validate the OpenAPI contract
make test           # Run Server and Web tests
make build          # Build Server commands and Web
make verify         # Run the full repository verification
```

## License

The licensing model has not been selected, so the repository does not currently include a `LICENSE` file.
