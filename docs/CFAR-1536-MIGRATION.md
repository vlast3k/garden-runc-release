# CFAR-1536 Migration Ledger

Implementation in progress. The candidate is an uncommitted, isolated worktree,
not a published integration branch. No new release was uploaded or deployed.
Native builds and local contracts do not establish live compatibility.

## Exact Inputs

| Input | Identity |
| --- | --- |
| Canonical Garden | `90175b24cfcdaafc1f5e26a060251423a53805a3` |
| Owning-parent Garden pin | `4c90944f9337bf0ec4e29c9d1580cc462b241333` |
| Team default fetched during work | `bee74aabb8b8ff70d78042394c2e2e6b2b740f96` |
| Previously packaged Guardian | `b55c6c6df4b9c78b70136c7597a5ccf92035d449` |
| Independent Guardian port input | `e4e266e0dad0e2957f5920b927a7f86776f447ff`, compared with `5852620c` |
| Inline Guardian tree including restored contracts and canonical patch-test fixture | `eafb22cd3423f21a757df1be009419098d08e0fa` |
| Go source package fingerprint | `803d6abe1fb7886295f7a5a3a0a47b6d51c577af33635c2bb5e0cbfe0a11365a` |
| Go source package SHA-256 | `b49ef5a44acad90bd5e2504d2235a6630c3add2fb7c63848c90c4b46f16f9f83` |
| Inner Go 1.27.3 linux/amd64 archive SHA-256 | `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40` |

The inline tree was computed with a disposable index, not the real index. Full
provenance verification requires the final committed tree and clean source.
Preserve team-default changes during default-based integration and review.

## Checks and Scope

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

## Required Acceptance Remaining

| Criterion | Status | Required next evidence |
| --- | --- | --- |
| AC1 revisions and complete delta disposition | NOT RUN as a complete criterion | Partial source evidence above; full delta reconciliation and default-based review remain |
| AC2 retained lifecycle/pea/stream/metrics/network/runtime behavior | NOT RUN as a complete criterion | Partial units/transport evidence above; enabled native containerd/runsc, cgroup and cleanup gates remain |
| AC3 package/config/provenance agreement | NOT RUN as a complete criterion | Partial package/toolchain evidence above; clean-source gate, complete native BOSH compilation and effective parser remain |
| AC4 component and parent regressions | NOT RUN as a complete criterion | Partial regression evidence above; final-candidate suites, parent verify-project and native --linux remain |
| AC5 new stock Garden with unchanged Diego/Rep | NOT RUN | Stock canary; no upgrade requirement demonstrated yet |
| AC6 live matrix/soak/rollback | NOT RUN | All ticket workflows; 3 consecutive lifecycle runs, 30-minute soak, cleanup bounds, controls and rollback rehearsal |
| AC7 reviewed publication and parent adoption | NOT RUN | Exact reviewed component/default revision, parent pin and tested-tree comparison |

Stock diagnostic cell was `cf_cells_1`,
`diego-cell/178aba96-7728-4b96-a6a3-725748160b8a` on t09a only. Hash-verified C
inputs and inert outputs remain under root-only mode-0700
`/var/vcap/data/cfar1536/guardian-c-build`; they were not executed or installed.
Both stock cells reported process_state `running` after compilation; this is
not application-level health proof. Before upload/deploy, revalidate exact live
target, active writer, placement, effective config, rollback and cleanup plan.