# ReleaseHub

## Badges

![Go](https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go&logoColor=white)
![Node.js](https://img.shields.io/badge/Node.js-%3E%3D22.22.0-339933?logo=nodedotjs&logoColor=white)
![pnpm](https://img.shields.io/badge/pnpm-11.19.0-F69220?logo=pnpm&logoColor=white)

ReleaseHub is a release-governance platform that uses Argo CD as its deployment executor. It gives platform teams one place to manage Application onboarding, production Deployment Requests, approvals, deployment order, execution state, and audit evidence. The GitOps repository remains the source of desired state: ReleaseHub does not write Git, control Image Updater, or replace the existing GitOps flow with image overrides.

For local initialization, set `manager_password: admin` to create the initial `admin / admin` account. The first login must change this password before other authenticated operations are available. See [Configuration](Docs/en/configuration.md#initial-manager).

Full documentation is available in:

- [English](README.en.md)
- [繁體中文](README.zh-TW.md)

## Development Verification

Run from the repository root:

```bash
cd Web
pnpm install --frozen-lockfile
cd ../Server
go mod download
cd ..
make verify
```

This workflow validates generated OpenAPI artifacts, Server and Web tests, lint, builds, bilingual document pairs, and Helm and Kustomize deployment definitions. For prerequisites and the complete validation scope, see the [English documentation](README.en.md#development-verification) or [繁體中文說明](README.zh-TW.md#開發驗證).

## Documentation Paths

- First deployment: [Configuration](Docs/en/configuration.md) → [Deployment](Docs/en/deployment.md) → [Operations Runbook](Docs/en/operations-runbook.md)
- Platform and data flow: [Architecture](Docs/en/architecture.md)
- Application integration: [Argo CD](Docs/en/argocd.md) → [Amazon ECR](Docs/en/ecr.md)
- Identity and sessions: [OIDC](Docs/en/oidc.md)

Running the full service also requires PostgreSQL, Argo CD, AWS ECR, and Kubernetes. OIDC is optional when local authentication is enabled. Follow the Configuration and Deployment documentation to prepare the environment.

## Repository Structure

- `Server/`: Go API, Worker, and Migrate commands.
- `Web/`: React web application and Nginx runtime.
- `API/`: Source OpenAPI contract.
- `Deployments/`: Helm and Kustomize deployment definitions.
- `Docs/`: Traditional Chinese and English technical documentation.
