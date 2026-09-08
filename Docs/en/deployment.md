# Deployment

## Build Images

```bash
docker build -t releasehub-server:local Server
docker build -t releasehub-web:local Web
```

Both images run as non-root users. API, Worker, and Migrate share the Server image and select their component with the first command argument.

## Helm

Create `releasehub-secrets` first. Then override connection addresses, AWS scope, image repositories and tags, and ServiceAccount annotations:

```bash
helm upgrade --install releasehub Deployments/helm/releasehub \
  --namespace releasehub --create-namespace \
  --values values.production.yaml
```

Helm creates Migrate as a `pre-install,pre-upgrade` hook. A migration failure must prevent rollout of the new API and Worker.

`configuration.worker` configures reconciliation, queue polling, leases, retries, Application locks, and Application concurrency. `configuration.notifications` configures notification retention and projection. Keep `worker.replicas` at `1` during phase one.

## Kustomize

Create a production overlay that patches the ConfigMap, ServiceAccount annotations, and image names. Never write environment secrets into the base.

```bash
kustomize build Deployments/kustomize/base | kubectl apply -f -
```

The Migrate Job uses an Argo CD `PreSync` hook at sync wave `-10`; other workloads use the default wave. A non-Argo CD deployment must run Migrate and wait for success before updating API, Worker, and Web.

Kubernetes or an ALB can probe `/healthz` and `/readyz` on Web and API. Those paths do not emit access logs. Web must resolve the API service as `releasehub-api:7580`.

See the [Operations Runbook](operations-runbook.md) for queue incidents, `Partial Failed`, manual unlock, Argo CD outages, and Forward Rollback.

## Deployment Manifest Validation

After installing Helm, Kustomize, kubeconform, Trivy, and yq, run:

```bash
make verify-deployments
make verify-deployment-security
```

The first target verifies that Helm and Kustomize render successfully and produce equivalent workloads, Services, Worker settings, and notification settings. The second validates Kubernetes schemas with kubeconform, blocks High or Critical misconfigurations with Trivy, and verifies that the SSE proxy disables buffering and caching while retaining its long-lived connection timeout.
