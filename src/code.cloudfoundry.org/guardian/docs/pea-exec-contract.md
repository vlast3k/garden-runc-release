# Pea Exec Contract

## Current runsc dispatch (CFAR-1540)

Production runsc construction now selects image-backed peas through
`Containerizer -> PeaCreator -> RunContainerPea -> Nerd`. The requested image is
prepared by the configured volume provider; named users resolve securely inside
that image. Garden pea/parent annotations are translated internally into runsc
subcontainer identity, and RunPea performs Start after successful Create.
Grouped children use image-mapped gofer namespaces and the parent's virtual PID
namespace, not the Sentry's nobody-only host user namespace.

Attach validates the parent, targets the child task, and uses a single FIFO
reader with output fanout. Run, attached callers and restart cleanup share Wait
and task/image cleanup. Attach after Start explicitly rejects nonnil stdin;
output writers retain synchronous ProcessIO backpressure. Metadata remains until
image deletion succeeds, allowing cleanup of a missing-task orphan without
inventing an exit code. Runtime-deletion failure retains image and metadata.

Native prepared-image, real GrootFS and fresh-client restart matrices exercise
this path with stock controls. They do not establish shipped Envoy/healthcheck,
live CF, independent child resource enforcement, CF accounting or packaged
release acceptance. The owning parent's `docs/validation/CFAR-1530-DELIVERY.md`
records candidates and raw results. Stock dispatch remains unchanged. The
explicit exec compatibility mode and its controls remain available for
historical regression tests only; the remainder of this document describes that
limited path, not production runsc dispatch.

When `execPeas` is enabled for an alternative runtime, the candidate implements
the explicitly user-approved **parent-rootfs CF asset compatibility contract**
below. It does not implement the full image-backed pea contract, nor establish
that gVisor cannot support other container designs.

## Approved contract

An image-bearing request is accepted only when:

1. `Image.URI` has a nonempty `preloaded:` suffix or is a canonical absolute
   `/var/vcap/packages/<package>/rootfs.tar` path, with no image username/password.
   These are compatibility markers: no image is fetched, unpacked, or used as a
   new rootfs by this exec path. Neither form proves image use or provenance.
2. `Path` is canonical and beneath `/etc/cf-assets/`, or the request is exactly
   the pinned Linux Envoy `sh` wrapper described below. Every referenced asset
   (both the Envoy executable and config for that wrapper) must be covered by
   the most-specific parent bind mount (later mount wins a tie), whose canonical
   destination is also beneath `/etc/cf-assets/`. The original CF asset aliases
   are preserved, not rewritten to image paths. Noncanonical parent destinations
   overlapping an asset or requested mount are rejected, including aliases that
   would shadow an otherwise valid mount.
3. Every requested bind mount has host origin, canonical absolute source and
   destination, and matches the effective parent mount's source, destination,
   and RO/RW mode. The parent must be a bind mount (`Type: "bind"` or `bind`/`rbind`
   options); the later parent mount at an identical destination is effective.
   A parent mount considered for a requested destination with both `ro` and
   `rw` is rejected as ambiguous in either order, not interpreted as "last
   option wins". Four unit cases cover both orders and both requested modes.
   Otherwise `ro` denotes read-only and its absence read-write. No new mount,
   replacement source, or changed permissions are admitted; identical existing
   RW mounts are allowed.
4. `OverrideContainerLimits` is nil or an all-zero `garden.ProcessLimits`.
   Nonzero CPU weight/shares or memory limits are rejected, not ignored.

The wrapper exception is implemented by `execPeaAssetPaths`. It requires
`Path: "sh"` and exactly two Args: `"-c"` and the following command, with only
`<seconds>` variable:

```text
trap 'kill -9 0' TERM; /etc/cf-assets/envoy/envoy -c /etc/cf-assets/envoy_config/envoy.yaml --drain-time-s <seconds> --log-level critical& pid=$!; wait $pid
```

`<seconds>` must be a canonical unsigned decimal `uint64` value: `0` or digits
without a leading zero, sign, whitespace, or additional shell content. The fixed
prefix and suffix, including spacing, TERM trap, config path, log level, background
launch and wait, must match exactly. This does not admit arbitrary shell commands,
other Envoy arguments, or other shell paths. The parent must also supply a usable
`sh`; validation checks asset mount metadata, not shell or binary availability.

Only after validation are `Image` and `BindMounts` cleared for `runtime.Exec`.
Other process fields, including ID, Path, Args, User, Dir, Env, limits and IO,
are forwarded unchanged, as is the returned process. Unsupported requests return
Garden-visible errors before `Exec`, with no pea creation or pea user resolution;
parent-bundle errors are propagated. Missing binaries and unresolved users can
still fail in the runtime: mount coverage does not prove executable availability.

The candidate fixes are [parent asset alias/mount validation](../rundmc/containerizer.go#L254)
and [Nerd failed-Start cleanup](../rundmc/runcontainerd/nerd/nerd.go#L279).
If an exec process is created but `Start` fails, Nerd deletes it before returning
the error; a deletion failure is joined with the start error rather than hidden.

**Unchanged paths:** An empty `ImageRef{}` with nil `BindMounts` bypasses this
validation and executes normally; bind mounts without an image remain rejected.
Stock runc retains image-backed pea creation, requested mounts and limits, and
pea-rootfs named-user resolution.

## Source-verified CF callers

Primary source: `cloudfoundry/diego-release` at immutable commit
`997b400b6269e7e428967dba38de9322ec0e786c`, read from the local checkout with origin
`https://github.com/cloudfoundry/diego-release.git`. Its embedded executor is Git
**TREE** `149323f9fb7925703b73a9599db2004c417be2f2`, not an inferred executor commit.
The links below pin files in that release. These are source facts, not deployed
revisions or native CF acceptance results.

| Caller | Verified producer shape | Candidate behavior |
| --- | --- | --- |
| Declarative healthcheck | [Transformer sidecar][healthcheck]: `/etc/cf-assets/healthcheck/healthcheck`, `Image.URI = sidecarRootFS`, supplied bind mounts, zero override limits; User unset. | Eligible with a nonempty `preloaded:` marker or canonical packaged rootfs path, provided all parent-asset and requested-mount checks pass. |
| Linux container proxy | [Transformer sidecar][proxy]: sidecarRootFS image and supplied mounts; [Unix action][proxy-action] uses `Path: "sh"`, Args `{"-c", <fixed Envoy command above>}`, User unset. Transformer supplies `-c /etc/cf-assets/envoy_config/envoy.yaml`, `--drain-time-s` from integer drain seconds, and `--log-level critical`. | Eligible for the exact wrapper with canonical unsigned drain seconds, an accepted image marker, coverage of both Envoy assets, and identical requested parent mounts. Arbitrary shell forms remain rejected; source and unit coverage are not native proxy acceptance. |
| Ordinary app, SSH RunAction, user-sidecar RunAction | [RunAction dispatch][action], [app/sidecar traversal][sidecars], and [NewRun][new-run] use empty `Sidecar{}`; [RunStep][run-spec] copies its empty image, nil mounts/override and the action's User. | Ordinary empty-image exec is unchanged. No SSH-specific image or bind mounts are constructed here; synthetic image-bearing SSH tests are not producer evidence. |

Parent coverage is independently checked: [store-node dependencies][dependencies]
provide initial mounts, [healthcheck injection][healthcheck-mount] adds the asset
alias, [parent ContainerSpec][parent-spec] receives `n.bindMounts`, and
[transformer config][transformer-config] supplies that list to run-step construction.
[Rep selection][rep-selection] chooses the configured sidecar rootfs path from
preloaded/extra rootfs maps, with [job properties][rep-properties] defining the
selection rules. [StackPathMap][rep-map] maps each stack name directly to
`rootFS.Path`, not to a `preloaded:` URI. The [job template][rep-template] feeds
`preloaded_root_fs` from `p("diego.rep.preloaded_rootfses")` and wires both sidecar
selection properties. [Rep config input][rep-config-input] and
[expected parsed config][rep-config-expected] use
`/var/vcap/packages/cflinuxfs4/rootfs.tar`; the job property describes that same
path shape. [Initializer wiring][initializer-input] passes the selected path to
[WithSidecarRootfs][initializer-option] without adding a prefix. Thus the verified
healthcheck and proxy producers can supply packaged paths accepted by the current
validator. Extra-rootfs or configured values outside the two approved marker
forms are still rejected. This wiring proves neither that an image was loaded nor
that its contents or identity are used by parent-rootfs exec.

The first validator required `preloaded:` and a direct canonical asset Path, so
it rejected the pinned proxy's `sh` form and packaged rootfs paths. That boundary
is superseded by the explicitly approved CF-form additions above, not an active
proxy block. A direct Envoy asset test alone still does not exercise the native
wrapper or establish native CF acceptance.

## Named-user resolution

For accepted exec-peas, names are forwarded to the runtime's **parent bundle
rootfs** lookup; `PeaUsernameResolver` is bypassed. A user present only in the
requested image is not available. Stock runc resolves names against the pea
image through `PeaUsernameResolver`. Numeric `uid:gid` and empty users skip that
resolver in both modes; empty-user runtime defaults still apply. The local
fixture delegates to production bundle-rootfs user lookup, not host-user lookup.

## Support and proof limits

Sharing the parent's rootfs, namespaces and cgroup, without independent pea
filesystem isolation or CPU/memory limits, is an **explicitly approved limitation**
of this compatibility path. This approval is not permission for silent semantic
loss in arbitrary pea requests. Broader image-backed support or relaxing the
validation boundary requires a separate design and approval.

Path cleaning and mount-metadata matching are **lexical checks**, not symlink
confinement, executable-content validation, or an image/artifact provenance
guarantee. A `preloaded:` name is not checked against a loaded image's identity;
a packaged rootfs marker is not opened or checked for existence or contents.

## Recorded validation

Candidate branch: `cfar-1519-validated-exec-contract`, based on
`61e25e29efecfaf892ad8b6efc9b14794966bc6d`. The owning project's handoff records
the resulting source revision, publication and parent-pin adoption separately.
The original fake-only
completion report and any corresponding completion comment are superseded;
no Jira Done status is asserted.

Completed run `run.zjf9vZ`, 2026-10-15, used:

```bash
bash scripts/test-guardian-linux.sh --worktree --package ./rundmc,./rundmc/runcontainerd/nerd --run '^Test(Rundmc|Nerd)$' --test-only
```

Both full touched suites passed: **175/175 Rundmc** and **47/47 Nerd**, each with
zero failures, pending specs, or skips. These are suite totals, not 175 new pea
specs. Rundmc includes **43 rejection cases**, the two verified CF-form positives
(packaged healthcheck and exact packaged-rootfs Envoy wrapper), forwarding, zero
overrides, ordinary exec and stock dispatch. Fake-based units establish
validation/forwarding, not filesystem proof.

The Nerd suite includes **nine real pea-contract specs** in the
[Linux fixture](../rundmc/runcontainerd/nerd/nerd_linux_test.go#L210). Real Garden
Unix-socket transport, actual bundle lookup/exec and containerd **2.2.1** / runc
**1.3.4** are exercised; backend lookup and selected dependencies remain test
adapters. The builder now uses production `processes.UnixEnvFor`, including PATH,
not a fixture substitute that omits the shell search path.

- Parent asset/mount sentinels, stdin/stdout/stderr, env/cwd and exit **17** are
   asserted with preloaded and packaged markers, numeric `4242:4242` and a
   parent-only named UID **4242**. Candidate-rootfs-only UID **5252** cannot
   resolve in the parent and leaves no exec record.
- The exact Envoy `sh -c` wrapper executes a fixture binary against its mounted
   config. That binary checks all six CLI arguments and config contents; real
   stdout/stderr, UID 4242, exact Wait exit **23**, unchanged forwarded fields,
   exec-ID removal and parent health are checked. It is not shipped Envoy or an
   actual CF caller invocation.
- Three rejection specs cover nine mutated/arbitrary shell cases and each
   missing Envoy binary/config mount. Another seven-case spec rejects a distinct
   image, image-only executable, new/changed mounts or permissions, and nonzero
   CPU/memory overrides before Exec over Garden.
- Missing-executable Start failure deletes the exec record. SIGKILL/Wait returns
   **137** and removes the ID. The parent PID and running state remain unchanged,
   only its bundle remains, and another ordinary control process succeeds.

Reported end-to-end wall time is **297s**; `results.log` records **284s** of test
time (Ginkgo 0.817s Rundmc and 263.564s Nerd). `--test-only` skips broad build and
vet. Race-enabled `./rundmc`, `./rundmc/runcontainerd` and `./guardiancmd`
passed in `run.RvuYUN`. Full run `run.uUG88H` passed build/vet but was interrupted
with exit 130 before completing all suites; it is not a passing full gate.
The committed-candidate full gate is recorded separately in the owning-project
handoff, not inferred from these iteration results.

Exact tested overlay SHA-256:
`e62f1aa534eea5813f6dd47a0568857d26f4c9e4f44aa345263bb3b2d324a033`.
Logs and archived source overlay are at
`~/.cache/gvisor-cf/guardian-tests/run.zjf9vZ/` (`results.log`, `source.patch`).
This hash identifies that tested snapshot, not a commit or the subsequently
corrected documentation. The Colima QEMU x86_64 Linux guest used Go **1.27.1**,
kernel **6.8.0-100-generic**, and **cgroup2fs**. Recorded image identities:

- Go: `sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244`.
- Rootfs: `sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55`.

Historical results, not final-overlay passes: `run.5WCjsv` failed distinct-image
rejection (nil error); `run.6LF6US` passed after validation. `run.9SvWqR` passed
4/5 real specs and exposed the failed-Start record leak; `run.pjLeFM` passed 5/5
after Nerd deletion (38 others skipped). `run.eJjn8z` passed 164 Rundmc specs
with alias cases; `run.BmoiVj` passed 171 with CF forms (39 rejections, two
verified-form positives). `run.ddePMC` passed 8/9 real specs: the shell positive
failed because the fixture lacked PATH, not because production rejected the
wrapper. Production `UnixEnvFor` fixes that fixture and the positive passes in
`run.zjf9vZ`. `run.9EyPx8` failed compilation on an unused `goci` import after
helper removal; it was fixed before the completed run. Earlier logs and source
overlays remain under `~/.cache/gvisor-cf/guardian-tests/<run>/`.

## Acceptance boundaries

The candidate image is a separate filesystem fixture, **not a loaded OCI image**.
These results do not prove stock image-backed filesystem isolation, arbitrary
request cancellation, runsc behavior, native CF caller compatibility, a live
deployment, or native Linux artifact/build provenance. Local real-runtime tests
are no longer all blocked; the remaining runsc and native CF acceptance checks
are distinct gates, not conclusions inferred from the caller matrix.
Acceptance with the actual native CF callers, actual runsc, and shipped sidecar
rootfs/artifacts has **not been run** and requires explicit authorization, as do
native build/packaging and live acceptance. Local fixture positives do not close
those gates. Runtime-selection defaults, Nstar streaming, lifecycle ordering
and cleanup7 are retained; broader pea isolation is outside this candidate.

| Criterion | Source/local evidence | Remaining gate |
| --- | --- | --- |
| Producer contract | PASS at the immutable Diego release and embedded executor TREE above. | Deployed source/artifact matching and native CF invocation. |
| Validation, IO, user lookup, cleanup and SIGKILL/Wait | PASS for the units and local runc fixture described above. | Actual runsc/native CF assets; broader cancellation and image isolation remain unproved. |
| Full build/vet/regression gate | Interrupted iteration run; final result recorded in the owning-project handoff. | Completed result tied to its source snapshot. |
| Native build/packaging, actual runsc, live/native CF acceptance | NOT RUN for this candidate. | Explicit authorization and corresponding evidence. |
| Source publication and parent pin | Recorded separately in the owning-project handoff. | Reviewed source PR/revision, then owning-project gitlink update. |

Owning-project evidence handoff: `gvisor-cf`,
`docs/validation/2026-10-15-cfar-1519-pea-contract.md`. It is outside this component;
no parent-relative link or dependency is required to read this contract.

[healthcheck]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/transformer/transformer.go#L663-L693
[proxy]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/transformer/transformer.go#L986-L1024
[proxy-action]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/transformer/transformer_unix.go#L13-L34
[action]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/transformer/transformer.go#L158-L185
[sidecars]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/transformer/transformer.go#L444-L471
[new-run]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/steps/run_step.go#L43-L70
[run-spec]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/steps/run_step.go#L143-L160
[dependencies]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/containerstore/storenode.go#L207-L214
[healthcheck-mount]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/containerstore/storenode.go#L263-L273
[parent-spec]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/containerstore/storenode.go#L400-L409
[transformer-config]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/depot/containerstore/storenode.go#L550-L556
[rep-selection]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/rep/cmd/rep/main.go#L96-L142
[rep-map]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/rep/cmd/rep/config/config.go#L61-L67
[rep-template]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/jobs/rep/templates/rep.json.erb#L103-L105
[rep-config-input]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/rep/cmd/rep/config/config_test.go#L106-L111
[rep-config-expected]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/rep/cmd/rep/config/config_test.go#L236-L237
[rep-properties]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/jobs/rep/spec#L61-L71
[initializer-input]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/initializer/initializer_garden.go#L152-L172
[initializer-option]: https://github.com/cloudfoundry/diego-release/blob/997b400b6269e7e428967dba38de9322ec0e786c/src/code.cloudfoundry.org/executor/initializer/initializer_garden.go#L437-L461
