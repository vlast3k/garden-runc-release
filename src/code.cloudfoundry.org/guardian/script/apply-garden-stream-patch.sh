#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
patch="$PWD/patches/garden-stream-out.patch"
if ! command -v git >/dev/null 2>&1; then
  printf '%s\n' \
    '2796ebb0cc7d12b9c9ebc57a1b7c93dbeb8126db23b596e771844599cf276c50  vendor/code.cloudfoundry.org/garden/server/request_handling.go' \
    'c75dd16d1f15c29c58a6457a1b45d8eaca46bafe3c8d0a55ad0f4b424911e85d  patches/garden-stream-out.patch' \
    | sha256sum -c /dev/stdin >/dev/null 2>&1
  exit 0
fi
apply_args=()
if repository_root=$(git rev-parse --show-toplevel 2>/dev/null); then
  prefix=$(git rev-parse --show-prefix)
  if [[ -n "$prefix" ]]; then
    apply_args+=(--directory="${prefix%/}")
  fi
  cd "$repository_root"
fi
if git apply "${apply_args[@]}" --reverse --check "$patch" 2>/dev/null; then
  exit 0
fi
git apply "${apply_args[@]}" --check "$patch"
git apply "${apply_args[@]}" "$patch"