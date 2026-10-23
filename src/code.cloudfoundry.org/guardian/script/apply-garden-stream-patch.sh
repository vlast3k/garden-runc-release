#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
patch="$PWD/patches/garden-stream-out.patch"
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