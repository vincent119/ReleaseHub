# Deployment

## Purpose and Prerequisites

This document guides platform operators through building and deploying ReleaseHub. Before starting, provide PostgreSQL, a Kubernetes namespace, `releasehub-secrets`, an Argo CD service account, and an ECR scope reachable through Pod Identity or IRSA. Provide an OIDC client only when OIDC login is enabled. Keep all production settings in private values or a Kustomize overlay, and never commit secrets.

## Build Images

```bash
docker build -t releasehub-server:local Server
docker build -t releasehub-web:local Web
```

Both images run as non-root users. API, Worker, and Migrate share the Server image and select their component with the first command argument.

## Helm

Create `releasehub-secrets` and a production values file that overrides connection addresses, AWS scope, image repositories and tags, and ServiceAccount annotations. The repository does not provide production values. Set the following variable to an operator-owned file:

```bash
releasehub_values_file=/path/to/releasehub-values.production.yaml
test -f "$releasehub_values_file"
helm upgrade --install releasehub Deployments/helm/releasehub \
  --namespace releasehub --create-namespace \
  --values "$releasehub_values_file" \
  --wait --timeout 10m
```

Helm creates Migrate as a `pre-install,pre-upgrade` hook. A migration failure stops the release; do not bypass it or start the new API and Worker separately. After success, inspect the release and workloads:

```bash
helm -n releasehub status releasehub
kubectl -n releasehub get deployment,pod \
  -l app.kubernetes.io/name=releasehub
```

`configuration.worker` configures reconciliation, queue polling, leases, retries, Application locks, and Application concurrency. `configuration.notifications` configures notification retention and projection. Keep `worker.replicas` at `1` during phase one.

## Kustomize

Create a production overlay that patches the ConfigMap, ServiceAccount annotations, and image names. Never write environment secrets into the base. First verify that the overlay renders:

```bash
kustomize build /path/to/releasehub-production-overlay >/tmp/releasehub-production.yaml
```

The Migrate Job uses an Argo CD `PreSync` hook at sync wave `-10`; other workloads use the default wave. When Argo CD manages the overlay, it continues to the other workloads only after Migrate succeeds.

Do not pipe the complete Kustomize base or overlay directly to `kubectl apply`. Plain `kubectl` does not execute Argo CD hook semantics and cannot guarantee that migration finishes first. The repository currently provides no staged Kustomize pipeline for a non-Argo CD deployer. If another deployer is required, create a pipeline that runs Migrate, waits for Job completion, and only then updates API, Worker, and Web. Stop when migration fails or times out.

Kubernetes or an ALB can probe `/healthz` and `/readyz` on Web and API. Successful probes do not emit access logs; failed responses retain diagnostic observations. Web must resolve the API service as `releasehub-api:7580`.

See the [Operations Runbook](operations-runbook.md) for queue incidents, `Partial Failed`, manual unlock, Argo CD outages, and Forward Rollback.

## Deployment Manifest Validation

After installing Helm, Kustomize, kubeconform, Trivy, and yq, run:

```bash
make verify-deployments
make verify-deployment-security
```

The first target verifies that Helm and Kustomize render successfully and produce equivalent workloads, Services, Worker settings, and notification settings. The second validates Kubernetes schemas with kubeconform, blocks High or Critical misconfigurations with Trivy, and verifies that the SSE proxy disables buffering and caching while retaining its long-lived connection timeout.

Deployment-manifest validation does not prove that external integrations are usable. Before production, validate OIDC, Argo CD hard refresh and Sync, ECR lookup, notification SSE, migration failure, and Worker restart in an isolated environment.
