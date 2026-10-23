#!/usr/bin/env bash
set -euo pipefail
backend=${1:?stock or runsc required}
case "$backend" in
  stock) scenarios=(early-close) ;;
  runsc) scenarios=(lifecycle early-close cancel late-exit mixed) ;;
  runsc-scoped) scenarios=(cancel late-exit mixed) ;;
  runsc-mixed) scenarios=(mixed) ;;
  *) exit 2 ;;
esac
helper=/var/vcap/data/cfar1536/garden-client-acceptance
expected=16ad7c24e18371f304b22fb4f26bcf123cb0e475b13f2dd5e66850055a3bb6c5
actual=$(sha256sum "$helper")
[[ ${actual%% *} == "$expected" ]]
guardian_pid=$(pgrep -x gdn)
[[ "$guardian_pid" =~ ^[0-9]+$ ]]
sha256sum "/proc/$guardian_pid/exe"
if [[ "$backend" == runsc* ]]; then
  curl --max-time 5 -fsS http://127.0.0.1:17019/debug/vars -o /dev/null
fi
for scenario in "${scenarios[@]}"; do
  for repetition in 1 2 3; do
    printf '\nCASE backend=%s scenario=%s repetition=%s\n' "$backend" "$scenario" "$repetition"
    exercise_bound=40s
    outer_bound=100s
    if [[ "$scenario" == mixed ]]; then
      exercise_bound=360s
      outer_bound=420s
    fi
    timeout --signal=TERM --kill-after=5s "$outer_bound" "$helper" \
      -scenario "$scenario" -handle "cfar1526-cfar1536-final-$backend-$scenario-$repetition" \
      -reap-bound 30s -exercise-bound "$exercise_bound" -guardian-pid "$guardian_pid"
  done
done
printf 'PASS: final packaged stream matrix backend=%s\n' "$backend"