# ReleaseHub

[繁體中文](README.zh-TW.md)

ReleaseHub gives platform teams one place to manage production release candidates, approvals, deployment order, and execution evidence for Argo CD Applications. CI still builds images, the GitOps repository still stores desired state, and Argo CD still applies it. ReleaseHub governs the path between those systems.

## What ReleaseHub Manages

- Discover Argo CD Applications carrying the management label and monitor onboarding drift.
- Build immutable Deployment Request Versions from target manifests, live resource differences, and ECR digests.
- Run approvals and manual or automatic transitions through Release Workflows.
- Control Application dependencies and concurrency through Deployment Plan DAGs.
- Compare revision, resource differences, and image digests again before Sync. Changed content stops the deployment.
- Keep evidence for `Partial Failed`, retry, terminate, manual unlock, Forward Rollback, notifications, and Audit.
- Authorize operations through OIDC, Group, Role, Scope, and explicit Deny rules.

ReleaseHub does not write Git or control Argo CD Image Updater. Automated sync must be disabled for production Applications. After approval, ReleaseHub asks Argo CD to Sync the pinned revision.

## Where to Start

For a new environment, follow [Configuration](Docs/en/configuration.md) and [Deployment](Docs/en/deployment.md). PostgreSQL, Argo CD, AWS ECR, and Kubernetes are required external services. OIDC is needed only when OIDC login is enabled.

For the first Application, read [Argo CD Integration](Docs/en/argocd.md) and [Amazon ECR Integration](Docs/en/ecr.md). After onboarding and binding a published Workflow and Plan to the Environment, use the [Production Release Guide](Docs/en/release-flow.md) for the first governed release.

Use the [Operations Runbook](Docs/en/operations-runbook.md) when a Queue waits, an Execution becomes `Partial Failed`, or an operator must terminate, unlock, or handle an Argo CD outage.

## Verify the Repository

### Tool Versions

| Tool | Version or purpose |
| --- | --- |
| Go | `1.26.6` from `Server/go.mod` |
| Node.js | `22.22.0` or later from `Web/package.json` |
| pnpm | `11.19.0` from `Web/package.json` |
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

Success is an exit code of `0`. The target checks generated OpenAPI artifacts, Server and Web tests, lint, builds, bilingual document pairs, and Helm and Kustomize schema and security settings. Run `make help` to list individual targets.

Local initialization can use `manager_password: admin` to create `admin / admin`. The first login must change that password immediately. See [Initial Manager](Docs/en/configuration.md#initial-manager).

## Documentation

- First deployment: [Configuration](Docs/en/configuration.md) → [Deployment](Docs/en/deployment.md) → [Operations Runbook](Docs/en/operations-runbook.md)
- Production release: [Release Guide](Docs/en/release-flow.md)
- Platform design: [Architecture](Docs/en/architecture.md)
- Application integration: [Argo CD](Docs/en/argocd.md) → [Amazon ECR](Docs/en/ecr.md)
- Login and authorization: [OIDC](Docs/en/oidc.md)
- Database setup: [Database Initialization](Docs/en/database-initialization.md)

## Repository Structure

- `Server/`: Go API, Worker, and Migrate commands.
- `Web/`: React web application and Nginx runtime.
- `API/`: Source OpenAPI contract.
- `Deployments/`: Helm and Kustomize deployment definitions.
- `Docs/zh-TW/` and `Docs/en/`: Bilingual technical documentation.

## Phase-One Limits

- One Argo CD instance.
- One AWS account and region.
- Exactly one Worker replica, keeping the Application concurrency limit platform-wide.
- Repository tests pass, but an end-to-end exercise in an isolated Argo CD and ECR environment is still pending.

## Common Development Commands

```bash
make generate       # Regenerate Go and TypeScript OpenAPI code
make lint-openapi   # Validate the OpenAPI contract
make test           # Run Server and Web tests
make build          # Build Server commands and Web
make verify         # Run the full repository verification
```

## License

The licensing model has not been selected. The repository does not include a `LICENSE` file.
