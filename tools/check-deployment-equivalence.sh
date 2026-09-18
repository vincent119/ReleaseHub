#!/bin/sh
set -eu

work_directory=$(mktemp -d)
trap 'rm -rf "$work_directory"' EXIT

helm template releasehub Deployments/helm/releasehub --namespace releasehub >"$work_directory/helm.yaml"
kustomize build Deployments/kustomize/base >"$work_directory/kustomize.yaml"

project_workloads() {
  yq eval-all -o=json '
    [select(.kind == "Deployment" or .kind == "Job") |
      {
        "kind": .kind,
        "component": .metadata.labels."app.kubernetes.io/component",
        "image": .spec.template.spec.containers[0].image,
        "args": .spec.template.spec.containers[0].args,
        "liveness": .spec.template.spec.containers[0].livenessProbe.httpGet.path,
        "readiness": .spec.template.spec.containers[0].readinessProbe.httpGet.path,
        "runAsNonRoot": .spec.template.spec.securityContext.runAsNonRoot,
        "readOnlyRootFilesystem": .spec.template.spec.containers[0].securityContext.readOnlyRootFilesystem
      }
    ] | sort_by(.component)
  ' "$1"
}

project_services() {
  yq eval-all -o=json '
    [select(.kind == "Service") |
      {
        "name": .metadata.name,
        "port": .spec.ports[0].port,
        "targetPort": .spec.ports[0].targetPort
      }
    ] | sort_by(.name)
  ' "$1"
}

project_runtime_configuration() {
  yq eval-all -o=json '
    select(.kind == "ConfigMap" and .metadata.name == "releasehub-config") |
      .data."config.yaml" |
      from_yaml |
      {
        "worker": .worker,
        "notifications": .notifications,
        "argocd": .argocd
      }
  ' "$1"
}

project_workloads "$work_directory/helm.yaml" >"$work_directory/helm-workloads.json"
project_workloads "$work_directory/kustomize.yaml" >"$work_directory/kustomize-workloads.json"
project_services "$work_directory/helm.yaml" >"$work_directory/helm-services.json"
project_services "$work_directory/kustomize.yaml" >"$work_directory/kustomize-services.json"
project_runtime_configuration "$work_directory/helm.yaml" >"$work_directory/helm-runtime.json"
project_runtime_configuration "$work_directory/kustomize.yaml" >"$work_directory/kustomize-runtime.json"

diff -u "$work_directory/helm-workloads.json" "$work_directory/kustomize-workloads.json"
diff -u "$work_directory/helm-services.json" "$work_directory/kustomize-services.json"
diff -u "$work_directory/helm-runtime.json" "$work_directory/kustomize-runtime.json"
