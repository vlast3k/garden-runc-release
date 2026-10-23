#!/usr/bin/env bash
set -euo pipefail

source_root=$(cd "$(dirname "$0")/.." && pwd)
repository_root=$(git -C "$source_root" rev-parse --show-toplevel)
prefix=$(git -C "$source_root" rev-parse --show-prefix)
handler=vendor/code.cloudfoundry.org/garden/server/request_handling.go
fixture_revision=${1:-90175b24cfcdaafc1f5e26a060251423a53805a3}
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

for layout in standalone nested archive; do
  root="$fixture/$layout"
  guardian="$root"
  if [[ "$layout" == nested ]]; then
    guardian="$root/src/code.cloudfoundry.org/guardian"
  fi
  mkdir -p "$guardian/script" "$guardian/patches" "$(dirname "$guardian/$handler")"
  if [[ "$layout" != archive ]]; then
    git init -q "$root"
  fi
  git -C "$repository_root" show "${fixture_revision}:${prefix}${handler}" > "$guardian/$handler"
  cp "$source_root/script/apply-garden-stream-patch.sh" "$guardian/script/"
  cp "$source_root/patches/garden-stream-out.patch" "$guardian/patches/"
  bash "$guardian/script/apply-garden-stream-patch.sh"
  if ! grep -Fq 'panic(http.ErrAbortHandler)' "$guardian/$handler"; then
    printf '%s\n' "FAIL: $layout skipped the HTTP handler" >&2
    exit 1
  fi
  (cd "$guardian" && git apply --reverse --check patches/garden-stream-out.patch)
  before=$(git hash-object "$guardian/$handler")
  bash "$guardian/script/apply-garden-stream-patch.sh"
  [[ "$before" == "$(git hash-object "$guardian/$handler")" ]]
  printf '%s\n' invalid-source > "$guardian/$handler"
  if bash "$guardian/script/apply-garden-stream-patch.sh" > "$root/rejection.log" 2>&1; then
    printf '%s\n' "FAIL: $layout accepted incompatible vendor source" >&2
    exit 1
  fi
  printf 'PASS: %s application, idempotence, incompatible-source rejection\n' "$layout"
done