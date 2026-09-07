#!/bin/sh
set -eu

status=0

for document in Docs/zh-TW/*.md; do
  name=${document##*/}
  if [ ! -f "Docs/en/$name" ]; then
    echo "missing English counterpart: Docs/en/$name" >&2
    status=1
  fi
done

for document in Docs/en/*.md; do
  name=${document##*/}
  if [ ! -f "Docs/zh-TW/$name" ]; then
    echo "missing Traditional Chinese counterpart: Docs/zh-TW/$name" >&2
    status=1
  fi
done

exit "$status"

