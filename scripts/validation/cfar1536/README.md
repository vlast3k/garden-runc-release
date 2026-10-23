# CFAR-1536 Diagnostics

These are acceptance diagnostics, not shipped Guardian production source. The
resource probe creates and destroys one container; the config and orphan probes
are read-only. Run live probes only on an authorized, verified t09a target, as
root, with no concurrent writer and an outer timeout. They do not deploy, repair
configuration, kill unrelated processes or waive failed acceptance criteria.

## Build and Local Controls

From the release root, build using the exact candidate Guardian vendor tree and
the recorded Go 1.27.3 toolchain, not a new dependency resolution:

```bash
cd src/code.cloudfoundry.org/guardian
diagnostics=../../../scripts/validation/cfar1536
go test -mod=vendor -race -count=1 -timeout=60s \
  "$diagnostics/resource-probe/main.go" "$diagnostics/resource-probe/main_test.go"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -trimpath \
  -o /absolute/diagnostic-output/config-probe "$diagnostics/config-probe/main.go"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -trimpath \
  -o /absolute/diagnostic-output/resource-probe "$diagnostics/resource-probe/main.go"
```

On native Linux, `config-probe -backend stock -self-test` exercises four actual
parser controls without installed files. Full checks accept only `stock` or
`runsc` and compare all installed INI properties through Guardian's actual parser
and both quota files through GrootFS's actual builder. Expected quota exclusions
are the declared t09a shard settings, not universal Garden defaults. Use a
20-second outer deadline. No server command is executed.

## Resource Contract

Use `timeout --kill-after=5s 150s resource-probe -backend stock -handle UNIQUE`
(or `runsc`). Require an owned, unique `cfar1536-` diagnostic handle and preserve
the initial inventory. The probe refuses existing handles. It uses the installed
rootfs tar, 384 MiB memory, 128 MiB exclusive disk and 256 CPU shares. Positive
allocation/write/busy-loop thresholds and bounded over-limit writes are printed
in the raw output. A runsc exit 128 is insufficient: the exact WaitPID/EOF
signature, independent owned cgroup `oom_kill` increase and Garden OOM event
must agree. Final exit success also requires Destroy and restored inventory,
not merely the earlier `PASS` line. A forced outer timeout may bypass deferred
cleanup: inspect and remove only the exact owned handle before another attempt.

## Post-Destroy Inventory

`timeout 30s python3 orphan-audit.py` requires Python 3.11+ and the actual
installed containerd configuration. It fails on missing required roots, failed
containerd queries, unreadable inventories or owned residue. It searches only
the CFAR-1536 diagnostic prefixes encoded in `owned`; other workloads are neither
reported nor removed. Empty argv cannot identify an argv-free sentry, so this
does not replace the native pre-delete PID/start-time checks. It also cannot
prove pre-teardown exec/I/O cleanup; use the packaged mixed matrix for that.

## Packaged Stream Matrix

`stream-matrix.sh` is the frozen final launcher. It requires the exact official
caller binary hash printed in the script, a unique numeric Guardian PID and,
for runsc, reachable debug counters. `runsc-scoped` selects cancellation,
late-exit and mixed cases; `runsc-mixed` selects only the three mixed repetitions.
The broad `stock` and `runsc` selections include the older host-global
early-close case, which did not pass on the occupied final cells. Do not claim
those broad selections pass from the scoped results. Case-specific exercise,
reap and outer bounds are explicit in the script; preserve the caller's complete
cleanup output and final status.

## Staticfile Repair Fixture

`staticfile-fixture` keeps buildpack 1.6.39, its actual nginx executable and a
real CF sidecar. The buildpack's deprecated but source-tested custom-config hook
accepts its token-free nginx.conf. This prevents concurrent profile substitution
from losing unresolved tokens; it does not fix the buildpack globally. The
literal port 8080, staged public document root and profile hash were verified
for these exact t09a fixtures. Do not treat them as portable app defaults.

On native Linux, run `test-staticfile-profile.py --profile-evidence PATH` with
the original `profile-exact.txt` evidence. It requires the exact recorded profile
hash and checks its original-token substitutions before 30 serial / 200
concurrent-pair stability controls. Use a 120-second outer deadline. GNU sed is
required; the macOS BSD sed attempt fails. Expected existing-FIFO diagnostics
are distinguished from unexpected utility failures.

`staticfile-matrix.py` and `staticfile-cleanup.py` freeze the two exact disposable
app GUIDs from this run. They are audit/reproduction inputs, not reusable launch
commands for new app identities. The matrix requires three 120-second-bounded
restarts per backend, exact payload and config/profile hashes, HTTP 404 control,
real sidecar and SSH streams/exit. Cleanup verifies the API hostname, app GUIDs,
exclusive route destinations and preserved original app GUIDs before deletion,
then verifies absence and original route bytes. Run only after explicit target
and identity validation. The completed fixtures have already been removed.

The migration ledger retains the original successful binary hashes and raw-log
hashes. The Go files here are gofmt-normalized copies of those diagnostic
sources. Build paths are different, so do not claim binary hash equality without
measuring it. The inline Guardian runtime tree remains unchanged.