#!/bin/sh
set -eu

snapshot_directory=$(mktemp -d)
trap 'rm -rf "$snapshot_directory"' EXIT

snapshot() {
  destination=$1
  {
    shasum -a 256 Server/internal/transport/openapi/api.gen.go
    find Web/src/generated -type f -print | LC_ALL=C sort | xargs shasum -a 256
  } >"$destination"
}

snapshot "$snapshot_directory/before"
make generate
snapshot "$snapshot_directory/after"
diff -u "$snapshot_directory/before" "$snapshot_directory/after"

