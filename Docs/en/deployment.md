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

## Kustomize

Create a production overlay that patches the ConfigMap, ServiceAccount annotations, and image names. Never write environment secrets into the base.

```bash
kustomize build Deployments/kustomize/base | kubectl apply -f -
```

The Migrate Job uses an Argo CD `PreSync` hook at sync wave `-10`; other workloads use the default wave. A non-Argo CD deployment must run Migrate and wait for success before updating API, Worker, and Web.

Kubernetes or an ALB can probe `/healthz` and `/readyz` on Web and API. Those paths do not emit access logs. Web must resolve the API service as `releasehub-api:7580`.
