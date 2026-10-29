# Grouped Pea Contract

With an alternative runtime (validated: runsc), Guardian selects image-backed peas through
`Containerizer -> PeaCreator -> RunContainerPea -> Nerd`. The requested image is
prepared by the configured volume provider; named users resolve securely inside
that image. Garden pea/parent annotations are translated internally into runsc
subcontainer identity, and RunPea performs Start after successful Create.
Grouped children use image-mapped gofer namespaces and the parent's virtual PID
namespace, not the Sentry's nobody-only host user namespace.

Grouped children share the sandbox's Sentry threads and memory, so the sandbox's
host cgroup bounds parent and children together. Garden accepts nil or all-zero
`OverrideContainerLimits` (Envoy and declarative healthchecks) and rejects
nonzero per-pea CPU or memory limits before preparing the image. Unlike runc,
healthcheck usage therefore counts against the application's limits.

Attach validates the parent, targets the child task, and uses a single FIFO
reader with output fanout. Run, attached callers and restart cleanup share Wait
and task/image cleanup. Stdin remains startup-owned: Attach closes Garden HTTP's
always-present input pipe without forwarding bytes, permitting output-only
clients. Other nonnil input readers are rejected; attachment does not reopen or
close the task's initial stdin. Output writers retain synchronous ProcessIO
backpressure. Metadata remains until
image deletion succeeds, allowing cleanup of a missing-task orphan without
inventing an exit code. Runtime-deletion failure retains image and metadata.

Pea Create IO follows the OCI terminal flag. Nonterminal startup input completion
releases the shim stdin keepalive only after the last FIFO write. Terminal input
completion must retain it until exit/deletion: the stock shim's console master
is bidirectional, and closing it also loses output and delivers SIGHUP. Output-only
Attach therefore does not terminate a terminal when its HTTP input pipe closes.

Grouped peas require `use_containerd_for_processes`; Guardian refuses to start
an alternative runtime without it. Stock dispatch is unchanged.

Native prepared-image, real GrootFS and fresh-client restart matrices exercise
this path with stock controls. They do not establish independent child resource
enforcement or CF accounting.

The earlier parent-rootfs exec compatibility mode (`execPeas`) was removed; it
was never selected by production wiring. Its contract, CF caller analysis and
validation record remain in Git history before this removal.
