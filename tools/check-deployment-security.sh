#!/bin/sh
set -eu

for required_tool in helm kustomize kubeconform trivy; do
  if ! command -v "$required_tool" >/dev/null 2>&1; then
    printf 'required deployment validation tool is unavailable: %s\n' "$required_tool" >&2
    exit 1
  fi
done

work_directory=$(mktemp -d)
trap 'rm -rf "$work_directory"' EXIT

helm template releasehub Deployments/helm/releasehub \
  --namespace releasehub >"$work_directory/helm.yaml"
kustomize build Deployments/kustomize/base >"$work_directory/kustomize.yaml"

kubeconform -strict -summary \
  "$work_directory/helm.yaml" \
  "$work_directory/kustomize.yaml"
trivy config --exit-code 1 --severity HIGH,CRITICAL Deployments

grep -Fq 'location = /api/v1/notifications/events {' Web/nginx/nginx.conf
grep -Fq 'proxy_buffering off;' Web/nginx/nginx.conf
grep -Fq 'proxy_cache off;' Web/nginx/nginx.conf
grep -Fq 'proxy_read_timeout 3600s;' Web/nginx/nginx.conf
