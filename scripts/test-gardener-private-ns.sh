#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == --guest ]]; then
  root=$2
  [[ "$(readlink /proc/self/ns/mnt)" != "$(readlink /proc/1/ns/mnt)" ]]
  [[ "$(readlink /proc/self/ns/net)" != "$(readlink /proc/1/ns/net)" ]]
  mount -t tmpfs -o mode=0755 tmpfs /run
  mkdir -m 0700 /run/netns
  ip link set lo up
  cd "$root/native-stock-a2bc4214/guardian/gardener"
  timeout --kill-after=10s 180s "$root/gardener-stock.test" -test.v -test.timeout=150s
  cd "$root/native-candidate-http-fixture-2d8d1036/guardian/gardener"
  timeout --kill-after=10s 180s "$root/gardener-candidate.test" -test.v -test.timeout=150s
  [[ -z "$(ip netns list)" ]]
  exit 0
fi

[[ "$EUID" == 0 ]]
root=/var/vcap/data/cfar1536/gardener-native
[[ "$(uname -m)" == x86_64 ]]
[[ -d "$root" && ! -L "$root" ]]
mkdir "$root/.writer-lock"
trap 'rmdir "$root/.writer-lock"' EXIT
cd "$root"
sha256sum -c gardener-test-binary-hashes.txt
unshare --mount --net --propagation private /bin/bash "$0" --guest "$root"