# CFAR-1536 Migration Ledger

Implementation and live validation remain in progress. Repaired source candidate
`2a7cf010d090b1f8a5fb6e48fe78c106cce9ff9a` is committed on the default-based
`cfar-1536-canonical-garden` branch, not yet published or reviewed. Native full
backend gates and clean provenance pass. Its native source release has been
uploaded and all four t09a cells now use it. Runsc rollback/re-upgrade rehearsal
and durable regeneration parity pass; landscape commit `6753857` retains the
final version. Resource enforcement/accounting and ASG controls pass on both
backends. Final 30-minute soak and three mixed fault-matrix repetitions pass.
Required full criterion reconciliation and reviewed source/parent adoption
remain incomplete.

## Exact Inputs

| Input | Identity |
| --- | --- |
| Canonical Garden | `90175b24cfcdaafc1f5e26a060251423a53805a3` |
| Owning-parent Garden pin | `4c90944f9337bf0ec4e29c9d1580cc462b241333` |
| Team default fetched during work | `bee74aabb8b8ff70d78042394c2e2e6b2b740f96` |
| Previously packaged Guardian | `b55c6c6df4b9c78b70136c7597a5ccf92035d449` |
| Independent Guardian port input | `e4e266e0dad0e2957f5920b927a7f86776f447ff`, compared with `5852620c` |
| Inline Guardian tree including restored contracts and Git-free compiler verification | `d66fc7838ccaf9bbadeadb258182fbc587fadff0` |
| Final repaired inline Guardian tree, source candidate `2a7cf010` | `26b9f00d4f64d99cea6c4688394f9997971cacf3` |

### Native BOSH Compilation Attempt

Source checkpoint `2a4305fc313a6b04728144dff4bd7d0813738a69` produced
`garden-runc/1.103.0-cfar1536.2a4305fc`, archive SHA-256
`090a8538ee95467046f25af8ef4946bc3e7ec9d6764d0971e19aa1d48653a739`.
Upload task `78714` and stock manifest dry-run `78723` passed on t09a.
Canary task `78752` failed during Guardian compilation, before instance updates:
`script/apply-garden-stream-patch.sh: line 16: git: command not found`.
The temporary hold on stock control `d9556df9-c219-48b5-846b-ecb70c4c6f88`
was removed after this failed attempt. This attempt does not establish runtime
compatibility or acceptance.

The applicator now verifies exact SHA-256 identities of both the already-patched
handler and patch when Git is absent. It cannot apply an unpatched handler in
that environment and fails closed on altered or incompatible inputs. The actual
script passes restricted-PATH tests for standalone, nested and archive layouts,
including those three negative controls. Git-based source application remains
unchanged. The repaired native compilation passed in task `78777` below.
| Go source package fingerprint | `803d6abe1fb7886295f7a5a3a0a47b6d51c577af33635c2bb5e0cbfe0a11365a` |
| Go source package SHA-256 | `b49ef5a44acad90bd5e2504d2235a6630c3add2fb7c63848c90c4b46f16f9f83` |
| Inner Go 1.27.3 linux/amd64 archive SHA-256 | `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40` |

The inline tree was computed with a disposable index, not the real index. Full
provenance verification requires the final committed tree and clean source.
Preserve team-default changes during default-based integration and review.

## Checks and Scope

### Behavior Disposition Against Canonical Input

Paths below are under `src/code.cloudfoundry.org/guardian` unless stated
otherwise. Classification compares canonical `90175b24` with candidate
`74005dfa`, plus the explicit `2a7cf010` runtime-options, init-I/O and
cancellation-drain repairs recorded below. Test names and source presence alone
do not establish execution.

| Integration contract | Disposition | Exact owning source / retained test anchor |
| --- | --- | --- |
| Create -> Silk -> Start | Required port | `gardener/gardener.go` adds `Containerizer.Start` after network preparation; `rundmc/runcontainerd/runcontainerd.go` delegates Start; stock `RunRunc.Start` is a no-op; Gardener and Nerd tests |
| Bundle-rootfs user lookup | Required call-site port; existing helper reused | `RunContainerd.Exec` uses `bundle.Spec.Root.Path` instead of `/proc/<pid>/root`; `users.LookupUser` itself is unchanged; runcontainerd and named-user pea tests |
| Exec streaming / Garden HTTP abort | Required port plus host-fixture adaptation | `rundmc/streamer.go`, stream tests, owned `patches/garden-stream-out.patch` and applicator; Garden 0.9.0 is not an upstream-fix exemption |
| Containerd metrics | Required port and dependency adaptation | New `runcontainerd/containerd_statser.go`, v1/v2 decoder tests, `guardiancmd` selection; explicit stats1/stats2 package manifest patterns |
| Supported pea contract | Required port | `Containerizer.validateExecPea` validates parent CF assets before clearing image/mount request; `pea_contract_test.go`, `runpea_test.go`, native Nerd pea cases |
| Stock/default and runsc opt-in policy | Required port | `runtime_policy.go`, startup/factory tests and Nerd policy payload tests; unsupported runtimes fail closed |
| CREATED-task cleanup7 | Required retained port, not removal | `Nerd.Delete`, `parentShimPID`, cleanup/process identity tests and alternative-shim CREATED-task spec; CFAR-1508 remains separate |
| Runtime state routing | Existing selected abstraction retained; no separate guard in `state.go` | `runrunc/state.go` is byte-identical in independent Guardian, canonical and candidate and delegates to `StateCommand`. `guardiancmd` selects RunContainerd when containerd is enabled; its State delegates to Nerd's task API. Native focused state suites pass; raw `state-routing-native.log`. This is not a claim that an additional guard was ported |
| gVisor V1 exact ID | Unchanged external component | Parent gVisor pin `34a45484ba288254b0535499f1e5f33c3416b539`; `state_file_exact_match_cf_gate_test.go` |
| gVisor V2 missing parent | Unchanged external component | Same immutable gVisor pin; `state_file_missing_parent_cf_gate_test.go` |
| gVisor V4 peer addresses | Unchanged external component | Same immutable gVisor pin; `network_peer_addr_cf_gate_test.go`; Silk/loopback remain integration acceptance inputs |
| CPU topology cap | Unchanged external runtime plus preserved render and task propagation repair | Same runtime hashes; candidate `2a7cf010` preserves Container.Runtime.Options. Final packaged fresh CF sandbox has the typed options and actual cap-2 argv recorded below. This proves propagation, not a hard bandwidth limit or a cap discriminator on two-vCPU hosts |
| Authoritative release config | Required BOSH port | `jobs/garden/spec` and owned templates, Garden RSpec; runtime pre-start remains unchanged and performs no cross-job Garden config rewrite |

Independent Guardian changes from packaged `b55c6c6d` to parent `e4e266e0`
include only two functional files: `rundmc/streamer.go` and
`rundmc/streamer_acceptance_linux_test.go`; three other changed files are bot
configuration. Committed `74005dfa` production streamer is byte-identical to the newer
independent input. Its acceptance fixtures differ only by `fixtureCommand`, which
resolves host `/bin/tar` and `/bin/mkdir` through PATH on Nix OCI. Container
production executable paths and assertions are unchanged at that checkpoint.
The subsequent cancellation-drain repair below changes both files and is
committed in `2a7cf010`; its packaged release is installed on all four cells.

| Check / expected predicate | Actual | Evidence level |
| --- | --- | --- |
| Unmodified job rendering, all examples pass | 52 examples, 0 failures | macOS Ruby control |
| Quota missing-behavior control | 26 intended failures; 2 preservation controls pass | Pre-port rendering negative control |
| Runtime missing-behavior control | 4 intended failures; 2 controls pass | Pre-port rendering negative control |
| Combined candidate job rendering | 86 examples, 0 failures | macOS Ruby only |
| HTTP applicator standalone/nested/archive layouts, idempotence and rejection | All 3 layouts pass | Actual applicator, not Git exit alone |
| Native Gardener private namespaces, zero skips, 180s per-binary/150s Go timeout | Canonical 191/191; port 221/221, zero skips | Earlier exploratory Go 1.27.2, not release toolchain |
| Stock and candidate focused packages with Go 1.27.3 | Both pass `./guardiancmd ./rundmc/runcontainerd ./netplugin`, count=1, timeout=10m | Native OCI Linux x86_64 |
| Candidate streaming race tests with Go 1.27.3 | 11 top-level tests pass, zero failures, count=1, timeout=10m | Actual Garden transport, fake runtime; not live CF |
| Build commands from real extracted BOSH Guardian package | gdn/dadoo/execas/socket2me build; gdn also builds with daemon tag | Native Go 1.27.3, after stats manifest repair |
| Exact packaged C inputs, static flags unchanged | nstar and init build; static x86-64 ELF verified | Native Noble GCC 13.3.0, isolated diagnostic directory |
| Schema-1/schema-2 provenance regressions | 78 tests pass, 176.968s | Local fixtures; 13 schema-2 controls include valid baseline and tampering |
| Schema-1/schema-2 provenance regressions | 79 tests pass, 170.361s | Local fixtures; 14 schema-2 controls include valid baseline, tampering and missing-module rejection |
| New Go verifier against real source package | Zero failures, exit 0 | Actual outer/inner hashes, lock/index and VERSION; toolchain portion only |
| Real CLI on uncommitted candidate without hydrated package | Exit 1; rejects dirty source, unmatched committed tree and missing package input | Expected negative control, not a source verification PASS |

Streaming selection was `^(TestExec|TestGardenClient|TestStreamer|TestSignal)`
in `./rundmc`, with `-mod=vendor -race -count=1 -timeout=10m -v`.
Focused packages used `-mod=vendor -count=1 -timeout=10m`. Source references
and selector presence are not execution proof. Backend Nerd/cgroup gates and
full parent/Linux checks remain required.

Final local provenance raw log is
`~/.cache/gvisor-cf/cfar1536-provenance-final.log`, SHA-256
`7725531596f2c96473c421c9d7db38e5a4acd40b9cea0f0b0a60104cc173e1ef`.
The corresponding CLI rejection log is
`~/.cache/gvisor-cf/cfar1536-uncommitted-gate.log`. Both commands completed with
the expected exit status; test success is distinct from the expected source-gate
failure. Preserve these logs with final publication evidence.

## Raw Native Evidence

Paths below are relative to t09a OCI
`/mnt/landscape-lod-cfk8s-t09a/gen/cfar1536/`.

| File | SHA-256 |
| --- | --- |
| `stock-release-source-5c115a47.tgz` | `5c115a47ff823739ca7facfa9a203bb3ea3dfb92786669a96c6fad137257e2b0` |
| `garden-stock-exploratory.tgz` | `2565859909cc84a629ba70f589d382dd0b75f662afce34b81e9a9b481a4a967f` |
| `candidate-release-source-4f827814.tgz` | `4f827814eb7e909148be020fd1e382dfd63b72eab7664373b627029d132f3102` |
| `garden-port-exploratory-4f827814.tgz`, broken package | `7c352f8734bdb380122528933de5377645bcac2b764bb824d55b17e77a5074c8` |
| `guardian-spec-stats-repair.yml` | `d5d968a54c5f1aaf63dbde68cf6c41e0db4965fbb55bfd14173a10d799da15e0` |
| `garden-port-stats-repaired.tgz` | `684e997eec00c2f33c5566e012b221de54f2c8c391abfe9c4c569a2afdb8cd3a` |
| `candidate-streaming-go1273.log` | `ab28c86f823d3ac1535f0f6cbb2bd1acc238e621c78ea0131d093f19e4b0b1d8` |
| `candidate-focused-go1273.log` | `e3ea0e05223c9e2a326f9ff0316c56d092399cb7bc27846f25dfb4ef895fbb11` |
| `stock-focused-go1273.log` | `60dd98aad46d98e71658a810ca372c5423cd95e497354a3fb86c4a110112859e` |
| `candidate-release-build-attempt1.log` | `d4db35e373e22b07bee98193f7f6d9b4afb94acec9370198fb8e4630c5199f54` |
| `candidate-release-build-attempt2.log` | `6f399f388a4b19591f30173366631ff78eb9e74528e058efb919a58bf802b2b2` |
| `candidate-nstar-build.log`, OCI linker failure | `413d4183599d9ad8dc910935485a3741aadac369ffaacc7319bf092b253286bb` |
| `candidate-c-native-noble.log` | `bde7b3f050641b0fde662cb626b79681a69ceef046f83f978983ba429e82417e` |
| `guardian-c-inputs.tgz` | `5dc25118096a7035ded8a9c3b2939577d0569c6bc6036aecd6014e2fb69f06d7` |
| `gardener-private-ns-baseline-candidate-v3.log` | `13e887409e800481c419b1e5224e671d214abd8ebbfcaf510f3dcf3ec3d2cda8` |

The repaired release is the frozen source plus the separately verified stats
specification. Restored contract docs and later provenance helpers are NOT in
that earlier archive. Rebuild the final complete committed candidate; exploratory
`create-release --force` source archives are not promotion artifacts.

## Failed Attempts and Repairs

1. Nested Git patch checks silently skipped the vendor patch. Correct root/prefix
   handling repaired the actual HTTP abort regression; source/archive layout
   tests now exercise the applicator and incompatible-input rejection.
2. OCI lacks `/bin/tar` and `/bin/mkdir`; host fixture commands now resolve via
   PATH. Production container paths were not changed.
3. Canonical and port Gardener tests hit OCI namespace restrictions. Private
   cell namespaces permit both suites. The first runner failed before tests on
   missing `/run/netns`; version 3 creates it privately and verifies cleanup.
4. First package-only build failed on absent `cgroups/v3/cgroup1/stats` and
   `cgroup2/stats`. Two explicit `.go` manifest patterns repair the real package;
   whole-source compilation alone did not reveal the omission.
5. OCI static linking failed with `cannot find -lc`; `<nixpkgs>` is also absent.
   Noble prerequisites were verified before isolated static compilation; no
   packages were installed and static flags were not weakened.
6. Provenance replay: 65 tests with one obsolete standalone-path error. New
   schema-2 baseline exposed macOS path normalization. First combined 78-test
   attempt had one stale unsupported-schema expectation. Repairs and the final
   78-test pass do not erase those failed attempts.

## Live Checkpoint, 2026-10-23

Candidate release `1.103.0-cfar1536.74005dfa`, archive SHA-256
`9e718c4bd4251281668e1bbbec62999331fa0c85dbda55f61164fb4b17e09436`,
uploaded successfully in exact task `78774`. The upload observer incorrectly
reported an old compilation failure; querying task `78774` confirmed success.
Do not retry a mutation based only on a mismatched observer result.

On `lod-cfk8s-t09a`, task `78777` compiled Guardian on Noble and updated stock
cell `178aba96-7728-4b96-a6a3-725748160b8a`; its control was explicitly held.
After three official-client lifecycle passes, task `78868` drained and updated
the remaining stock cell. Both have Guardian SHA-256
`9ee0a4b78ff11a93254913993bc62ee35bec54060a10128469945abfd2e6595f`
and unchanged Rep SHA-256
`170444ea493b46d295a8984862feae4890701cde1a7b423ad8de44d6410bc963`.
All Monit processes were running. The release-only scope verifier compares the
complete candidate against a baseline allowing only Garden version replacement
and removal of its old URL/SHA; Diego `2.138.0` and all properties are unchanged.

Task `78897` then drained and updated only runsc canary
`02728eb7-9307-437d-aadc-76c71687f0cc`, retaining its control. Guardian/Rep hashes
match stock. Three complete archive and three lifecycle scenarios passed, each
with owned-handle refusal, phase deadlines and inventory-restoring cleanup.
Archive checks include exact payload bytes, leading-dash names, spaces,
malformed-input rejection, missing-path caller error and subsequent controls.
The separate lifecycle case checks exec exit `23`, exact stdout and normal-file
archive bytes. Its wrong-exit negative control and the unchanged full archive
oracle passed locally under the race detector. The diagnostic is an uncommitted
parent change, explicitly labeled dirty; binary SHA-256
`16ad7c24e18371f304b22fb4f26bcf123cb0e475b13f2dd5e66850055a3bb6c5`.

Runsc release `0.6.0-cfar1529.1` is unchanged. Runtime hashes match old/new cells:
runsc `6598ee29bd69dc79962a76ece6556baba90a7195b5be7802b0752114cc234349`,
shim `38b2b637e7d93987e77d1501cbee203958c128a230ca5c43ad0fd9aaad4f6db0`,
sentry `f09bc86b8ae86d17d2f800b0e71c767e51ac3cd2a0a68cbe1856cd78760200c2`.
The sentry is under `/var/vcap/packages/runsc/gvisor-bin`, not PATH. Failed
inspection attempts at other locations are retained, not artifact failures.

The full archive case failed on BOTH old stock control and new stock canary:
Nstar returns no regular file for a leading-dash single filename. This is
inherited stock behavior, not repaired or relabeled as PASS in this migration.
The runsc exec-streaming path passes the unchanged full case. After failed stock
cases, the new canary was empty and the control retained its original 28 handles.

Raw evidence is retained under landscape `gen/cfar1536/`: task logs
`stock-second-78868.log`, `runsc-canary-78897.log`, stock/runsc artifact-health
logs, `stock-canary-lifecycle-three.log`, `runsc-canary-archive-1.log`,
`runsc-canary-repeat-cycles.log`, control/stock archive failures, and runtime hash
and cleanup inventories. Task `78949` completed the second runsc update using
normal drain; both runsc cells have the same verified Guardian/Rep/runtime hashes
and all Monit processes running. These client checks alone are not the full CF
application, soak or rollback matrix.

### Permanent Configuration and Regeneration

Dedicated product branch `cfar-1536-t09a-garden`, draft PR
`cloudfoundry/product-cf-hcp#119152`, has commits
`8f852b3b7e2777e7d9c13cfa917e20ab7ef3a5c0` and
`03c4c93b747ed60c6543228c3d8aedc4db7c4a45`. Shared product defaults were not
changed. The post-interpolation release pin helper rejects invalid, missing and
duplicate release records. The exact native suites passed 33 tests and 13
subtests, including actual BOSH interpolation of stock/runsc manifests.

The initial full regeneration check exposed two-to-one instance drift and five
gVisor-property differences. The product ops repair preserves deployed debug
false, CPU cap 2, proxy true, image excluded from quota and evacuation timeout
300 seconds. Landscape commit `2cb3462f11a47a7adbf6ac827c1885a751a1bc4e`
persists that exact product plus four total default cells and shard size two;
Garden/runtime pins remain `1.103.0-cfar1536.74005dfa` and
`0.6.0-cfar1529.1`. The preceding landscape release-pin commit is
`11f49421e6c012f17baea63b9103a4569fa0c6e2`.

Fresh context, base writer and both `deploy_cf_cells --dry-run --shard` actions
passed. Full parsed manifests, not a release-only projection, compare equal to
both currently deployed manifests: two instances each, unchanged Diego
`2.138.0`, and the new Garden. Raw result is
`gen/cfar1536/regeneration-parity-v2.json`. No deploy was made from the drifting
manifests. The rejected whole-list IaC edit (rc 69) changed nothing; documented
`--array-item 0.cells` and `0.override_shard_size` edits succeeded.

### Actual CF Workflow Evidence

Owned spaces `cfar1536-stock` and `cfar1536-gvisor` in org
`cfar-1499-gvisor` use verified default/null and gVisor isolation bindings.
Identical staticfile fixtures staged and started in 35 and 49 seconds, below the
declared 120-second staging/start limits. HTTP healthchecks, 192 MiB web memory,
512 MiB disk and the 32 MiB `cfar1536-audit` sidecar are visible in actual API
records. Installed proxy ports are present on both processes. Stock app
`745725f8-548e-47ad-b5ac-f2770e599718` ran at `10.0.73.0`; gVisor app
`205b3a2c-db22-4765-9ef5-f89267803266` ran at `10.0.73.2`.

Three consecutive real CF SSH cycles per segment verified the sidecar file,
exact separated stdout/stderr and exit 23. Direct gVisor SSH also verified
named user `vcap`/uid 2000, DNS and outbound certificate-validated TLS. The initial
pipe-captured stock SSH harness timed out twice (40 seconds, kill after five),
while byte-equivalent direct CLI commands passed. Explicit CLI-supported
`--disable-pseudo-tty` and stdin `DEVNULL` corrected the capture harness; all six
cycles then passed in four seconds without changing assertions or deadlines.
Both failed raw attempts remain retained, not reclassified as runtime passes.

Route sampler source SHA-256
`3abab961c5a23365bafa432e3af28fc7a6a2b21431e0e126cde7dc9e058b8327`
used exact fixture bytes, HTTP and certificate-validated HTTPS on both segments,
five-second per-request timeouts and ten-second intervals. The full window
passed: 181 samples, 724 requests, elapsed `1800.069733273005` seconds and zero
failures. Raw evidence is `gen/cfar1536/route-soak-30m-v1.jsonl`; the actual
sampler loop and wrong-body/non-200/extra-byte controls pass six local tests.
Raw CF matrix and fixture are in `gen/cfar1536/cf-initial-matrix.json`,
`cf-initial-matrix-v2.log`, versioned scripts and `web-fixture.tgz` (SHA-256
`2a3aa05349f40e99033d2aa46ced4bf4fbd058a7bc4fa5818b55f4d110f7f32b`).
Restart, enforcement/failure cleanup, ASG controls and rollback remain required;
these checks do not complete AC6.

Subsequent restart diagnostic `cf-restarts-v1.py`, SHA-256
`14436762798f0973bde2a41fc4aab63177b486f44867659e55fd0c057f864b88`,
declared three cycles per segment, 120 seconds per restart, 40 seconds per SSH
and five seconds per route request. All six cycles passed: stock
`13.051/12.854/13.156` seconds and gVisor `16.372/18.987/19.187` seconds.
Every cycle checked RUNNING API state, exact route body, sidecar sentinel,
exact separated SSH streams and exit 23. Raw JSONL SHA-256 is
`f48c8e3f244cd435bf8622dd7b17d5e8f6ed97e30bf5941a65e777decdc0af7f`.
This proves workload restart recovery, not the separately required complete
orphan-process inventory. Enforcement/failure cleanup, ASG controls and rollback
remain open.

### Native Parent Gate Attempts

The native OCI checkout is parent `fda47b8cbe12f544efec3f0afaabcb597bb333d9`
with all four exact gitlinks plus explicit diagnostic overlays. Both Ruby suites
(13 tests, 100 assertions), both Go modules with race/vet and real Linux
process/socket cleanup controls pass. Three portable Bash fixture/runner edits
were needed because this Nix OCI has no `/bin/bash`; existing negative controls
remain intact. Isolated Ruby gem installation initially failed because the Nix
wrapper overrides GEM paths; the observed unwrapped Ruby resolved that issue.

The initial full `verify-project.sh --linux` attempts did not pass: task-local checksum-verified Bazel
8.3.1, Nix loader/C++ library, JVM and batch/interpreter adaptations reached
target analysis, but embedded `process-wrapper` cannot execute without its FHS
loader. Attempts 1-7 and their raw failures remain under
`gen/cfar1536/parent-verify-linux-attempt*.log`. No gVisor source pin or system
path was changed to bypass this prerequisite. Native Noble validation starts
the exact Go 1.27.3, Bazel 8.3.1 and extracted Ruby 3.2.3 successfully; its first
attempt stopped before tests because Git is absent. A task-local extracted Git
prerequisite repair passed those prerequisites. Noble v2 retained a pager job-control
stop; v3 exposed optional Minitest plugin discovery, repaired by `MT_NO_PLUGINS=1`.
Noble v4 passed Ruby, Go race/vet and real process/socket cleanup, then failed before
the Bazel test executed because `/usr/bin/aarch64-linux-gnu-gcc` was absent.
Both architectures are registered in the pinned gVisor MODULE. A read-only APT
simulation identified 25 crosscompiler dependencies. V5 downloads/extracts them
under the owned diagnostic directory and exposes them with a private mount
namespace `/usr` overlay; host `/usr` and package state remain untouched. The
same unmodified full gate passed in Noble v5. Bazel executed
`//pkg/shim/v1/proc:proc_test`, passing in 3.3 seconds; Bazel elapsed time was
1391.368 seconds. Raw `parent-verify-linux-noble-v5.log` SHA-256 is
`315f6399bff662bddf544b0f0d50b160cf64873d9dd7ca46681b4702d8412617`.
Postconditions verified the writer lock absent, host crosscompiler absent, and
all stock-cell Monit processes running. No system packages were installed.
This gate exercised parent `fda47b8` with its existing four pins and declared
dirty parent overlays, not a future updated Garden gitlink. The gate explicitly
does not establish live CF acceptance or release artifact behavior.

### Enabled Native Nerd Gate

The unchanged candidate test binary, SHA-256
`38460705474fe2eda3b3323a3eb18c3b95720d528d3ab035691f40665b0bd0fd`,
ran with actual installed containerd/runc in private mount/network/PID namespaces
on stock cell `178aba96-7728-4b96-a6a3-725748160b8a`. The declared watchdog was
480 seconds, Go/Ginkgo deadlines 7 minutes. Rootfs archive SHA-256 is
`ae8ff4cff2a71a4fa2cec1c6cf061ccd0eef9c9f81f22420a21a9a090d3c1bc8`.

Attempt v1 ran 47 of 48 specs: 43 passed, four failed and one skipped. The
archive failure was `/bin/mkdir` exit 127: loader EACCES for `libselinux.so.1`;
the pea cases returned 127 or missed their readiness marker. Direct chroot as
UID/GID 5000 passed with that rootfs. Actual sudo Bash umask was `0077`, reducing
test-created `0755` fixture directories to `0700`; archive modes and LSM profile
were not the problem. Both initial test and cleanup results remain retained.

Runner v2 sets `umask 0022` only inside the private test namespace, preserving
the outer root-only diagnostic directory and unchanged binary/assertions.
All 47 stock-applicable specs passed in 286.230 seconds; all four auxiliary
proc helper tests and runtime-policy payload tests passed. One spec explicitly
requires an alternative Runtime-v2 shim and remains unexercised by this stock
run. Before namespace teardown, process/shim/netns/cgroup cleanup checks passed:
`NERD_POSTCONDITIONS test_rc=0 cleanup_rc=0`. This does not establish alternate
shim coverage. Raw v2 log SHA-256 is
`0614684f3df917bd34d5215cb2e44b7675fc4c7e0cde5424c8abb910ed3e8614`;
raw v1 SHA-256 is
`0ab2e489369cdd1d7df8527fbd92835770a0a62fc660d9e54f2ed46333af1829`.
Both logs are under `gen/cfar1536/nerd-native-stock-v*.log`.

### Alternative Runtime Cancellation Repair

Original full runsc v1 reached the archive case's immediate unread StreamOut
Close and hung in containerd DeleteProcess while the shim waited for output I/O.
The Go 11-minute timeout returned `test_rc=2 cleanup_rc=1` before namespace
teardown. Later host cleanup does not convert that cleanup failure to PASS.
Raw `nerd-native-runsc-v1.log` SHA-256 is
`a26dd613bfb92215b5eefa10d8a5eca74ba018e6f1f374f132f967c3ec099152`.

The uncommitted `streamer.go` repair wraps the StreamOut pipe writer: after
consumer closure (`io.ErrClosedPipe`), runtime output continues draining instead
of abandoning the containerd FIFO. Other write errors and tar Wait errors retain
their original propagation. New `TestExecStreamCloseDrainsRuntimeOutput` fails on
the original implementation with `io: read/write on closed pipe` and passes on
the repair, with a one-second Close bound. Original negative-control log SHA-256:
`9d2e1a592aa3d757294ce433f5ec050f092f7a57c8ee5f416f2e2f565325382c`.

Repaired native Nerd binary SHA-256
`113a2cfa1c751eca4f54a9e2b77a320d043a14e0468740de02a5041a0e69ebbf`
passed the same focused real runsc archive case in 14.848 seconds, cleanup 0.
Its full runsc v3 completed 48/48 specs: 28 PASS, 20 FAIL, zero skipped,
644.809 seconds, `test_rc=1 cleanup_rc=0`. Raw log SHA-256 is
`110978065cbad7d25c7144a548d9b6acc8d6b9b0c4831da815fed06f8195a2c0`.
This focused result is not a full gate PASS. Missing explicit Start in running
init, Exec, GetProcess, GetContainerPID and OOM fixtures is repaired separately;
already-started init-I/O cases exposed the distinct root cause below.

The broader native race v1 exposed fake producers that ignored SignalKill and
relied only on write errors to terminate. Raw `streamer-drain-race.log` SHA-256:
`0d076139bc16add6cc87f695c8b6a85b9cefa524d5bd48051c6700ee76435172`.
The fixtures now stop on kill and Wait also waits for producer exit; all existing
deadlines, producer completion and exactly-one-kill assertions remain. Identical
native race selection v2 passed all 12 top-level tests in 2.911 seconds, including
the new drain regression and actual Garden HTTP cancellation/churn paths.
Test overlay SHA-256:
`4d71644d999e574ab18b97b16b3660ad07d13e72191faeeb0cdd98b8d400158a`.

### Effective Runsc Options and Init I/O Repairs

These repairs are uncommitted and not present in deployed `74005dfa`.
Pinned shim `NewContainer` reads task flags only from a
`runtimeoptions.v1.Options` protobuf containing `config_path` field 2.
The global service config discovery and the rendered file alone do not apply
its runsc flags to new tasks. The first TaskOptions repair passed a gRPC fixture
but real installed containerd `b7a25261` rejected it with
`invalid task create option for io.containerd.runsc.v1` (v5, three failures).
That is a retained failed attempt, not an accepted payload path.

The corrected repair sets Container.Runtime.Options; runsc TaskOptions remain
nil and stock NoNewKeyring/UID/GID TaskOptions remain unchanged. Probe v6 shows
actual argv with `--sandbox-cpu-num-cap=2 --network=sandbox --overlay2=none
--directfs=false` and produces debug files. It still returned one PASS/two FAIL,
cleanup 0. The native payload test now covers five stock/runsc/annotation cases,
all passing; raw log `nerd-sandbox-policy-native.log` SHA-256 is
`7ea0ff22bda64dabd1f711ec7f5ba92ac9423cf0c213b2b18db781c99a767c89`.

The stdout-negative v7 used private syscall tracing: the init's first
`write(1, "hi\n", 3)` and stderr writes returned EPIPE, then the shell exited 1.
The gofer subsequently exited cleanly after EOF; the disconnect was not proof
of a gofer failure. Pinned shim `newInit` selects Create-time I/O only for a CRI
sandbox annotation, whereas runsc treats an unannotated OCI spec as a root
sandbox. Fixture-only explicit annotation v8 made the identical stdout oracle
pass in 7.568 seconds, cleanup 0. The production runsc policy now supplies that
annotation only when absent; stock and existing explicit annotations are
unchanged. The fixture-only annotation was removed again.

Native production binary `78fbd22fa09ce35234f3b6fbaa4484d274ce0a6b43ba5bcc19dcad88a7d95026`
passed init Start/stdout/stderr in v10 (3/48 selected, 27.055 seconds, cleanup 0).
The selector did not match the exact stdin title; stdin remains for the next
focused run. V9 failed before tests with ENOENT from the Nix dynamic loader;
the identical source was rebuilt natively with CGO_ENABLED=0. V9 is not a
behavioral failure and is not erased by v10.

| Raw log | SHA-256 |
| --- | --- |
| `nerd-start-debug-v5.log`, rejected TaskOptions | `f4a9305ee5b057026a7de61032db08a2a3169d2d9d2ce16905fb2ccbb8f52e5a` |
| `nerd-start-debug-v6.log`, effective options but init I/O failure | `95b7971ad0c34233ba416dc1fc31fc6bcfa0142218ec6c6d7844021b94d3f52d` |
| `nerd-start-debug-v7.log`, EPIPE negative | `15c8e9c9eb84776a3fcb47fc3259ef3b9577413bd7952caf9f1a4db9abad4a20` |
| `runsc-stdout-syscalls-v7.log` | `ac32a5136f809b8927475ad62227ad6bfd738b38de98b5b03e65c73bac898318` |
| `nerd-start-debug-v8.log`, annotation probe PASS | `a60045ee563f025b4d03b27e14d1df87d77d0cf23a321e3fa105d0b7eaf3c77b` |
| `nerd-start-debug-v9.log`, loader failure | `38aaf7e5921abf06f5fe234620f03931eb3109f8318a07e7a1d5d9063fe8488e` |
| `nerd-start-debug-v10.log`, production init I/O PASS | `d2c787a37a0ed60f37a2378f8bce14bb220536641f0a43c258d2fe2e642a1c68` |

### Final Native Fixture and Identity Repairs

Runsc v11 passed seven of eight focused cases; GetContainerPID compared a host
sandbox PID to guest `ctr ps` output. V12 retains the stock PID oracle and, for
runsc, verifies the returned host PID has exact `runsc-sandbox`, `boot` and
container ID argv. All eight selected cases, including stdin, passed in 70.260
seconds with cleanup 0. Full v13 then ran all 48 specs: 43 PASS, five FAIL,
zero skips in 371.127 seconds, cleanup 0. Three Wait and one Signal fixture
omitted explicit Start; the final failure was pre-delete identity capture.
V14 adds only those two setup Start calls and passes all four Wait/Signal
cases in 34.345 seconds, cleanup 0. This is focused evidence, not a full pass.

The identity collector returned before reading the executable for an empty
cmdline. Failure-only v15 diagnostics reproduced a sleeping child of the sentry:
`comm=exe`, `wchan=futex_wait_queue`, with executable resolving to the installed
`gvisor_sentry`. Pinned gVisor systrap `newSubprocess` calls `unmap`, which
removes non-stub address regions; ptrace has the same design. The collector now
reads executable and revalidates PID/starttime/parent even with empty argv.
It permits argv-free identities only with the task's exact executable and
transitive ancestry from the proven task, not merely from its shim. No PID is
discarded. The original exact shim ID, bundle/gofer proof and post-delete
PID/starttime disappearance checks, including zombies, remain required.

Native helper controls pass, including rejection of wrong executable,
shim-only ancestry, missing starttime, zombie state and argv-free task root.
A macOS package run failed to compile Linux-only cgroup methods; Linux native
controls and static cross-compilation pass. V16 passes the actual CREATED
case in 7.686 seconds with cleanup 0. Binary SHA-256 is
`48a94fd94015bc4d6d32ab257c45c351b3c54b0eb97bb7925306c3117da330a1`.
Full runsc v17 passed 48/48 with zero skips in 371.838 seconds; stock v4
passed 47 applicable specs with one explicit N/A skip in 277.149 seconds.
Both used that identical binary and reported cleanup 0.

Stock v3, on the preceding `8ae423a2` binary, passes all 47 applicable specs in
280.020 seconds; one explicit alternative-runtime-only spec is skipped and
cleanup is 0. It does not establish the final fixture/collector candidate.
These were uncommitted and absent from deployed Garden at that attempt. They
are now committed in `2a7cf010` and exercised on the final packaged canaries.

| Raw log under `gen/cfar1536/` | SHA-256 |
| --- | --- |
| `nerd-native-runsc-v13.log`, five retained failures | `ef00783dcf965cb9fa420ad7fe0f242acdb1e4eb86b0b126ea58e5b596a9d4e5` |
| `nerd-wait-runsc-v14.log`, four PASS | `9cf254793e093cd4624fe67e43b6e121b026c2b1e62ea2b2950723147208baff` |
| `nerd-identity-runsc-v15.log`, diagnostic failure | `a6a3d86a62831cb141c5563b94625fc7557e02135aaef11784c42b6e84355738` |
| `nerd-native-stock-v3.log`, 47 PASS | `cd2babd7d988bf5e1e02a778b4197f151ab29746e6114324d093f285b1a4fa8a` |

### Final Reviewed Oracle Candidate

Review found two additional oracle defects: unrelated kernel threads can lack
an executable link, and the annotation-preservation expected map shared the
mutated input. The collector now permits a stat-only identity only for an
unselected process; selected shim/task/gofer/descendant identities still require
the existing strict proofs. Real temporary-proc fixtures cover an argv-free
stub and a kernel thread without an executable link. The payload test captures
the sentinel value before Create and compares it unconditionally. No production
behavior was changed by these final test repairs.

On 2026-10-23, final inline Guardian tree
`26b9f00d4f64d99cea6c4688394f9997971cacf3` was exercised by the native Linux
Go 1.27.3 CGO_ENABLED=0 binary SHA-256
`3117b7094f5b8225a1b2506348c76d1af0345019ad6ac3ddb921f77c395615c5`.
The predeclared full gates used the same installed containerd/runtime and
rootfs as the preceding attempts, private namespaces and writer locks, runsc
720-second and stock 600-second outer watchdogs, and cleanup assertions before
namespace teardown. Runsc v18 passed 48/48 specs, zero skipped, in 368.986
seconds. Stock v5 passed 47/48 in 281.793 seconds; the single explicitly
alternative-runtime-only spec is N/A for stock. Both returned test_rc=0 and
cleanup_rc=0. Five proc helper tests and all five runtime-policy payload cases
also passed. The final native oracle/payload selection, adjacent
guardiancmd/runcontainerd/netplugin packages and streaming/transport race
selection passed on this source before the two full gates.

| Raw log under `gen/cfar1536/` | SHA-256 |
| --- | --- |
| `nerd-native-runsc-v17.log`, 48 PASS | `25738d7e353fe513725e042119d746bd3042e6527d4da2225e17111b0795ba38` |
| `nerd-native-stock-v4.log`, 47 PASS | `cea875727ed6c78b236dd747714f31c3471e1c8e6256855fe6a4af68b1895b72` |
| `nerd-native-runsc-v18.log`, final 48 PASS | `32f2543c1f772b7174ff485a999edd598bdc6233a265663d5f484f31cc937841` |
| `nerd-native-stock-v5.log`, final 47 PASS | `a0c6dc490f9d408f4df51802e5445e59510c6c6b485738e2099fb6e542f1f919` |
| `final-oracle-native.log` | `ae02a66071c626206c05bb12aacd7207109d62bffe307ed85a4988c011d3eed2` |
| `final-focused-native.log` | `316b17545346f0849edbb6b5ce50c0b56c888fbb529cd7b9ee643ef19a8f6045` |
| `final-streaming-race-native.log` | `6cd250d27b3b184488a6dd57da7c426e873dfa97fc766fb111a223afa887fb35` |

These are component-native results, not whole-task acceptance of the deployed
release. The durable version is now `1.103.0-cfar1536.2a7cf010`; final packaged
progress is recorded below.
Earlier failed attempts remain applicable to their original candidates and are
not erased by the final passes. A local provenance rerun was interrupted by
terminal reuse; a local rendering rerun stopped before tests because the shell
could not find its locked gems. Neither attempt is a passing check.
The serial provenance suite subsequently passed all 79 tests in 163.700 seconds.
The cached locked bundle uses Ruby 4.0; attempts to preload its absent `ostruct`
failed, and a run without that preload confirmed the actual dependency in
`bosh-template`. Installing only `ostruct` 0.6.3 in the private gem cache and
exposing its lib directory through RUBYOPT restored the unchanged renderer:
86 examples, zero failures. No Gemfile dependency or system gem was changed.

### Final Packaged Canary Progress

Source commit `2a7cf010d090b1f8a5fb6e48fe78c106cce9ff9a` freezes the tested
Guardian tree. The final renderer also passes 86 examples using the preexisting
host-specific test bundle with BUNDLE_FROZEN=true; the release lockfile remains
unchanged. A frozen run against the release lockfile itself rejected the local
Darwin-25 platform before examples, which is retained as an environment attempt.

The first native clean provenance attempt rejected a missing canonical upstream
object: the feature-branch bundle did not contain the separately imported,
non-ancestor `90175b24` history. Supplying its complete actual Git object history
fixed that input gap without changing source or assertions. Clean native
verification then passed for source `2a7cf010`, inline tree `26b9f00d`, exact
Go source package and canonical upstream identity. The first 138 MiB history
upload exceeded its 150-second request deadline while making chunk progress;
cancellation stopped the child and the destination was verified absent. A
fresh size-appropriate 600-second transfer passed all 14 chunk checks and final
SHA-256. This is bounded transport recovery, not a runtime gate PASS.

Native `bosh create-release --force --version 1.103.0-cfar1536.2a7cf010`
created the archive with SHA-256
`fdf4a145410358c4dc60ffa550df62d930ae6abaaaa7f8c1c5fdc05836222ef5`.
Fresh private manifests, all four exact healthy identities and no active BOSH
writer were verified before upload task `80335`. Actual parsed candidate
manifests differ only in the Garden release pin. Diego `2.138.0`, runsc release
`0.6.0-cfar1529.1`, all properties and two instances per shard are unchanged.

Task `80338` compiled Guardian package
`a6a4a6ed49206c0fbe784b5fa10d257b04b35caa0c3f5e7cdf075298a640df16`
and updated only stock canary `178aba96` with normal drain. Its control `d9556`
was unignored immediately afterward. Task `80386` updated only runsc canary
`02728eb7`, also with normal drain; control `998d2002` was likewise unignored.
Installed Guardian matches on both canaries:
`699db693f05eab651561cfde95a02a648cd527134c5b0b79d5bd9da5b8a09311`.
Rep remains
`170444ea493b46d295a8984862feae4890701cde1a7b423ad8de44d6410bc963`.
The runsc/shim/sentry hashes remain the exact values above; matched prewarmer is
`3315d7ad7c2d3751d349e4976fa02da1da6fd7e41746c35622304e5fabe4fce0`.
The initial hash probe incorrectly used a bin/ prefix and failed for two paths;
the packaging-grounded package-root probe passed. It is not a missing-artifact
failure or a waived runtime gate.

The same official Garden caller binary `16ad7c24` passed three consecutive
lifecycle checks on each final canary, each with a 120-second outer timeout,
five-second escalation and separate 30-second phase/cleanup bounds: exact exec
exit 23/stdout, normal-file archive payload and restored handle inventory.
Runsc additionally passed the full unchanged archive oracle, including literal
leading-dash and space paths, malformed-input and missing-path controls and
subsequent successful control archives. This does not waive inherited stock
leading-dash Nstar behavior or establish ASG/resource enforcement.

Raw deployment logs and repeated caller logs are under
`gen/cfar1536/final-rollout-2a7cf010/`. Runsc rollback task `80420` restored
`74005dfa` and its exact Guardian hash; re-upgrade task `80430` restored the final
hash. Fresh lifecycle sandboxes passed after both directions with normal drain.
Tasks `80464` and `80500` then updated stock control `d9556` and runsc control
`998d2002` sequentially with normal drain. All four cells use final Guardian
`699db693`; no cells remain ignored. Stock control caller initially lacked its
helper (exit 127); dedicated executable data staging repaired that prerequisite
and the lifecycle retry passed. Final runsc control lifecycle passed three
repetitions on 2026-10-23 at approximately 16:57 UTC.

Landscape commit `6753857e3389cce0144a9b8dc341282b7a9c1e3e` changes only
`config.garden_release_version`, is pushed, and preserves final Garden on
regeneration. Actual parsed whole-manifest equality passes for both shards,
not merely release-list equality. `create_manifest` emits without persisting;
the shard-owned `deploy_cf_cells --dry-run --shard` writes each shard and clears
the common output directory, so each generated file was frozen before rendering
the next. The first parity attempt failed for the removed first file; frozen
per-shard parity passes. Raw `regeneration-parity-final.json` SHA-256 is
`522400b5d4cdf703dd6bd691de1ab4a21966d1e496daf4597aeb6e7745b3655d`.

### Final CF Fixture and Enforcement Results

The original Staticfile 1.6.39 plus sidecar matrix passed three stock and two
runsc restart cycles, then failed its third runsc cycle at the declared
120-second deadline: nginx retained `((LISTEN_DIRECTIVE))` in its config. Both
web and sidecar source the same staged profile and mutate that config. The
actual profile SHA-256 is
`10ea72d238ef6be86adeb13d7e8b52dc34e478ecf5f835ae2bb145a0a17a0a43`.
Three bounded profile-race reproducer attempts retained all serial controls;
one concurrent attempt exposed an unresolved token, while the other two did
not reproduce it. This is intermittent evidence, not a deterministic universal
root-cause proof. Original matrix and logs remain under `final-cf-matrix-v1/`;
the original Staticfile case is not relabelled PASS.

A sidecar-safe diagnostic Go HTTP fixture preserves exact payload, 192 MiB
memory/512 MiB disk, 32 MiB sidecar, routes, HTTP healthcheck, proxy and SSH
assertions. Its local handler test includes exact root payload and missing-path
404 controls and passes with the race detector. Both pushes completed within
120 seconds; explicit route remapping repaired the fixture's `--no-route`
removal of old routes. Final matrix v2 passes all six fresh restart cycles
(three per backend, approximately 13.2-13.7 seconds each), exact HTTP bytes,
sidecar marker, SSH exit 23 with exact streams, DNS and nonzero metrics. Matrix
SHA-256 is `85f132c140b3e40670f02aa8bbc6e9043d2f008f7b4f8d5d937a3e1e1ecc0002`.
The replacement does not clear the original Staticfile limitation.

Fresh CF runsc sandbox `6c43890d-3727-4c78-44e1-6a36` has actual runtime
`io.containerd.runsc.v1`, typed `runtimeoptions.v1.Options` carrying
`/etc/containerd/runsc/config.toml`, and actual sandbox argv including
`--sandbox-cpu-num-cap=2`, `--cpu-num=2`, `--network=sandbox`, `--directfs=false`
and `--overlay2=none`. Both stock and runsc reporting nproc=2 alone is not a
cap discriminator on these two-vCPU hosts. This is not a CPU bandwidth claim.

ASG checks pass on both backends: public HTTPS exact payload succeeds; each
opposite cell's private TCP/22 target is reachable from the host; three bounded
sandbox connections to that same target are rejected. The exact per-container
ASG REJECT counter increases from 0 to 3 on each backend. No ASG policy changed.
Raw `final-asg-v1.log` SHA-256 is
`2c91bde36b113ff08315e26b7c3eaf57ca6f15a2de1a854c7506c0ad299b02bc`.

Resource probe v3 uses the official pinned Garden client and exact Linux binary
`d8c0ad07a98b2170f92d7664696dbe04f419a7623010e5fb88cd32759d387ebd`.
Declared disposable-container limits are 384 MiB memory and 128 MiB exclusive
disk, outer bound 150 seconds plus five-second kill escalation. Positive held
64 MiB allocation, 16 MiB write and five-second busy loop produce asserted
memory/disk/CPU counter increases on both backends. A bounded 256 MiB write
fails with ENOSPC at 134152192 bytes. A 768 MiB allocation increments only the
owned cgroup's `oom_kill` counters 0 -> 1 and emits Garden `Out of memory` on
both backends. Stock exec reports 137; runsc exec reports 128 with WaitPID EOF.
Pinned shim `proc.InternalErrorCode` and `checkProcesses` explain the latter:
OOM normalization to 137 applies to init, not exec. The oracle requires that
exact backend-specific signature AND independent cgroup increase AND Garden
event; unrelated errors, missing/wrong cgroups and successful allocation are
rejected in local race-tested controls. Both destroys and exact handle inventory
restoration pass. Raw log hashes:

| Resource attempt | SHA-256 / disposition |
| --- | --- |
| `resource-stock-v3.log` | `f692733f2f03b84a3fb427e2702964dfcce8664bc2cc556cb6be0d7c101b084b`, PASS |
| `resource-runsc-v3.log` | `14d38b7ae9a853a9a3b9784f2337c34d8828419d848ca630483c26c6d4d1e5e9`, PASS |
| v1 unsupported `preloaded:` image URI | FAIL before create; GrootFS nil transport panic, handle absent. Source-proven empty-scheme installed rootfs tar repairs fixture input, not quota thresholds |
| runsc v2 universal exit-137 oracle | FAIL; retained raw EOF/128 result and successful cleanup. v3 independent kernel/event assertions establish actual OOM rather than accepting generic 128 |

Final packaged per-container cancellation and late-exit checks each pass three
repetitions with exact exec identity, live parent, post-fault control archive
and cleanup. The older host-global early-close check rejects occupied baselines
of 5 and 51 execs on the two runsc cells; it is not passed or weakened. Staging
first selected a stale generic helper, then the exact `garden-client-lifecycle`
binary `16ad7c24`; the prefix guard also rejected an initial unqualified handle.
These prerequisite attempts are preserved. The first mixed run used an invalid
40-second launcher budget against its existing 80-second grace baseline and
failed before cycles; a source-grounded 360-second exercise/420-second outer
runner passes three repetitions, with all original assertions unchanged. Each
repetition runs three cycles of archive/cancel/late-exit against one parent plus
an independent control parent, for 27 mode cycles overall. Exact parent identity,
exec disappearance, I/O, handlers, FD/goroutine/grace settling and final handle
cleanup pass before teardown. Raw mixed log SHA-256 is
`8fc3a12b0a1463f02e48b92d21b3830e4b948900163eee74aa19576d6f91f170`.
Raw scoped
log `stream-runsc-scoped-final-v2.log` SHA-256 is
`8813a81bb58c01e9615dd5e73a4eec0ea49b311c82c0e8905512bd5f344ff89f`.

The final route soak passes its full declared 1800-second window: elapsed
1800.070181 seconds, 181 samples, 362 requests, zero non-200 results, and every
response has exact payload SHA-256
`80097eecb2a798755b10bc1d4b0056e27654d9aa980205e0e80a5ada497006a2`.
Raw `route-soak-final-2a7cf010-v1.jsonl` SHA-256 is
`045d0644dc93f4287675fc4a87230b92572db09fbada59c4cb55295043ee1f07`.
Post-test BOSH JSON reports all four exact cells running; landscape HEAD remains
`6753857` with clean worktree. Attempted t09a-only hibernation deferral failed before
EventBridge mutation: IaC rc 113, Terraform root_user STS 403
`InvalidClientTokenId`. Timer state remains unverified and was not described as
disabled. The failure is retained; no blind credential/action retry was made.

### Exact Installed Parser and Residue Checks

Reproducible probe sources and local build/control commands are retained under
`scripts/validation/cfar1536/`; they are outside the shipped Guardian tree.

Config probe v3 imports the actual candidate `guardiancmd.GdnCommand`,
`ServerCommand`, go-flags INI parser and GrootFS config builder. It never
executes the server. Binary SHA-256 is
`dccbf3eaa34aeb3533a200d05853dadaebe68cc086df7d5974853cb0a6841a75`.
Four controls accept explicit stock/runsc settings and reject an unknown
property and invalid boolean. Both installed stock and runsc config checks
pass within 20 seconds. Stock runtime type is empty, containerd process handling
is false, and both GrootFS quota exclusions are false; runsc selects
`io.containerd.runsc.v1`, true process handling and true exclusions. These match
the Garden job-level manifest properties, not unrelated instance-group keys.
Raw `config-stock-v3.log` SHA-256 is
`ffaf0906f360924ab07b204c8f1d07cb4ba0d2bb398169fd54650ba19b0c467a`;
`config-runsc-v3.log` is
`765e288463f1d43f19405c3410d61debc13514432bed0256e1c975d7f2a44bd5`.
V1's nil command initialization panic and v2's incorrect stock quota expectation
remain failed diagnostic attempts; neither changed installed configuration.

After the fault matrix, a separate read-only audit on runsc control `998d2002`
requires no owned containerd task or metadata, runtime argv, depot bundle, runsc
state or cgroup residue and no inventory read errors. It parses the actual
containerd TOML `state` path and has a 30-second outer bound. V2 passes with all
five inventories empty. Source SHA-256 is
`92669e2f495919cc842ce5dec760daf936bb2e30de8559062c4a35f6492936c9`;
raw `orphan-runsc-control-v2.log` SHA-256 is
`c69dab3fa9718bde32e3efe58d32b78c1a17723ee21110858dee2e66dc36ccd7`.
V1 failed closed on a generic runtime-v2 state path; the actual installed state
root is `/var/vcap/sys/run/containerd/state`, not `/run/containerd`.
This is post-destroy residue evidence only. It does not replace native
pre-delete PID/start-time identities or mixed-matrix exec/I/O cleanup before
teardown, and argv inventory alone cannot identify an argv-free sentry stub.

### Prospective Parent Pin Native Gate

The prospective `2a7cf010` Garden gitlink passes
`bash scripts/verify-project.sh --linux` on native Noble x86_64, Go 1.27.3,
Ruby 3.2.3. This is an isolated diagnostic parent checkout at `fda47b8`, with
the six declared caller/regression overlays recorded above and the new Garden
gitlink staged in its index, not a merged parent revision. The active parent
worktree and index were untouched. All four submodules match the diagnostic
index; exact overlay file hashes are printed in the log.

Ruby counts are 8 runs/48 assertions and 5 runs/52 assertions, no skips or
failures. Both actual Go race/vet suites and real process/socket cleanup pass.
Bazel executes the actual `//pkg/shim/v1/proc:proc_test`, 1/1 PASS in 3.3 seconds,
with `--nocache_test_results`. Component release gates are explicitly separate;
this command does not run `--release` or replace the native Nerd results.
Private cross-compiler overlay and tmpfs mounts disappear with the namespace;
the host cross compiler is absent afterward. The launcher uses an exclusive
diagnostic writer lock and a 1900-second outer bound; its trap removes the lock.
Frozen launcher SHA-256 is
`52af718ce6146a9efc45e81598c4d6c44e7fe1063e1b939b3fc40bab81ab4b8a`.
Raw `parent-candidate-2a7-linux-v1.log` SHA-256 is
`31ecb10ce178f37e70d9682a3551d5ed9c8a8adc65adebf0361da933c966ebe4`.
After reviewed component publication, the exact final parent candidate still
needs validation; a staged pin does not establish published adoption.

## Required Acceptance Remaining

### Current Self-Audit

This is disclosed agent self-review, not independent approval. Runtime source
remains `2a7cf010`, inline tree `26b9f00d`; subsequent evidence/diagnostic edits
do not alter the shipped Guardian tree or installed artifact identities.
The completion-audit failure-mode catalog was assessed as follows:

| Modes | Disposition / discriminating evidence |
| --- | --- |
| 1, 13, 19 scope, thresholds, authority | OPEN: original Staticfile failure and final reviewed adoption are retained below; no owner exception, criterion waiver or Jira closure is inferred |
| 2, 4 selected owner and real caller | Verified for recorded subsets: actual Nerd backend, pinned official Garden caller, installed typed runtime options, real kernel OOM/event and effective parser checks; fake-backed streaming tests remain labelled local evidence |
| 3, 5, 6 selected tests, controls and predicates | Native 48/47 counts, exact bytes/status, independent OOM, invalid parser controls and per-mode mixed postconditions are recorded; stock alternative-only spec is N/A, not PASS |
| 7, 15 partial state and tool wrappers | V1 orphan inventory failed closed on absent root; V2 actual TOML inventory and raw log pass. Empty tool captures were recovered from persisted files; rc alone was not used as evidence |
| 8, 9 external semantics and time budgets | Pinned shim exec-128 semantics require independent OOM; mixed grace required a source-grounded larger launcher bound, preserving failed attempt; final route soak measured its full 1800 seconds |
| 10, 11 identity and cleanup | Exact native PID/start-time checks and pre-teardown mixed exec/I/O checks complement the separate owned-prefix post-destroy audit; argv-free sentry cannot be cleared by that audit alone |
| 12 environment failures | Native compile, helper hash/staging, parser initialization, quota expectation and parent toolchain failures are retained with prerequisite repairs and reruns; no missing tool is counted as a successful behavior test |
| 14, 16 raw and conflicting evidence | Raw log hashes retained; old checkpoints distinguish prior source from final release; stale CPU, deployment and old-pin parent claims corrected. Full original Staticfile case remains FAIL |
| 17 candidate identity | Runtime `2a7cf010`, test binary `3117b709`, installed Guardian `699db693` and matched runtime artifacts are distinct identities; diagnostic source copies were compared with successful originals |
| 18 publication/adoption | OPEN: no reviewed component default merge or parent adoption yet; prospective staged-pin validation is explicitly not a merged parent revision |
| 20 audit theater / bounded loop | Subagent claims about missing pea/stdin/cleanup were rejected against actual native source/results; its stale fixture-location result was not used. Review is advisory, not executable evidence or merge permission |

| Finding | Original criterion | Disposition / next discriminating check |
| --- | --- | --- |
| F1536-1 original Staticfile/sidecar startup | AC6 restart behavior | OPEN: preserve timeout/profile logs, verify a deterministic app-local repair or source-grounded external-fixture classification; do not repeat unchanged intermittent repro attempts |
| F1536-2 exact final parent candidate | AC4, AC7 | OPEN: prospective pin gate now passes; validate the eventual reviewed parent revision after component publication |
| F1536-3 reviewed component/default adoption | AC1, AC7 | OPEN: publish draft with exact tested runtime tree, review complete current-head findings, repair/revalidate before merge and parent pin adoption |
| F1536-4 reproducible diagnostics | AC3, AC7 evidence | REPAIRED AND REVALIDATED: sources retained outside Guardian runtime tree; resource race controls pass, parser cross-build passes, source comparison, Python compilation and Bash syntax checks pass |

| Criterion | Status | Required next evidence |
| --- | --- | --- |
| AC1 revisions and complete delta disposition | NOT RUN as a complete criterion | Partial source evidence above; full delta reconciliation and default-based review remain |
| AC2 retained lifecycle/pea/stream/metrics/network/runtime behavior | NOT RUN as a complete criterion | Native backend gates, final lifecycle/archive, ASG, enforcement/accounting and cancel/late-exit/mixed pass; complete contract and orphan mapping remains |
| AC3 package/config/provenance agreement | NOT RUN as a complete criterion | Native BOSH compilation, clean candidate provenance and exact installed stock/runsc parser controls pass; final source/publication reconciliation remains |
| AC4 component and parent regressions | NOT RUN as a complete criterion | Final native Nerd/focused/race suites and prospective `2a7cf010` parent --linux gate pass; final published parent candidate validation remains |
| AC5 new stock Garden with unchanged Diego/Rep | PASS for exercised compatibility | Both stock cells retain exact Rep hashes; staging, HTTP healthcheck, proxy/sidecar, three SSH cycles and 30-minute route soak pass with unchanged Diego 2.138.0; whole-task acceptance is separate |
| AC6 live matrix/soak/rollback | NOT RUN as a complete criterion | Final all-cell rollout, rollback/re-upgrade, permanent pin, six safe-fixture restarts, full 30-minute soak, ASG, enforcement and mixed fault checks pass; complete orphan mapping and original Staticfile disposition remain |
| AC7 reviewed publication and parent adoption | NOT RUN | Exact reviewed component/default revision, parent pin and tested-tree comparison |

Stock diagnostic cell was `cf_cells_1`,
`diego-cell/178aba96-7728-4b96-a6a3-725748160b8a` on t09a only. Hash-verified C
inputs and inert outputs remain under root-only mode-0700
`/var/vcap/data/cfar1536/guardian-c-build`; they were not executed or installed.
Both stock cells reported process_state `running` after compilation; this is
not application-level health proof. Before upload/deploy, revalidate exact live
target, active writer, placement, effective config, rollback and cleanup plan.