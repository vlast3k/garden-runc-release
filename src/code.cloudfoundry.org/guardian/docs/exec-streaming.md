# Exec Archive Streaming

Alternative runtimes use sandbox exec, not host `/proc` traversal. The supported
CF rootfs must provide `/bin/mkdir` and `/bin/tar`, including `--` option
termination. Arbitrary Docker images without these tools are not supported by
this path. Stock runtimes continue to use Nstar.

Empty users resolve to `root`; numeric and named users are passed to the runtime's
bundle-rootfs user lookup. File paths archive the basename. Directory paths
without a trailing slash include the directory; paths with a trailing slash
archive its contents under `.`. Names containing spaces or beginning with a dash
are passed as literal arguments, never through a shell.

StreamIn returns mkdir/tar launch, wait and exit failures with available stderr.
StreamOut returns launch failures immediately and reports later process failures
through the returned reader. Over Garden HTTP, failures before output use the
existing JSON error response. Failures after output abort the response so existing
clients receive a body read error rather than clean EOF. Exact late diagnostics
remain in the server log; partial archives must be discarded.

Closing a StreamOut reader before completion closes the pipe and sends a kill
signal to that archive exec only. Its existing waiter reaps the process. Close is
idempotent; closing after completion does not signal any process. The container
and its init task remain running. Signal failures are returned to the closer.

## Garden Dependency Patch

The transport fix is owned by Garden's `server/request_handling.go`. Until an
upstream dependency incorporates it, `patches/garden-stream-out.patch` records
the minimal downstream patch against the vendored Garden revision
`3c8c94cab4c6`. It changes no client protocol and closes readers on every path.
After regenerating vendor dependencies, run:

```bash
bash script/apply-garden-stream-patch.sh
go test -race -count=1 ./rundmc -run 'TestExec'
```

The application script accepts an already-applied patch, applies an exact matching
base, and rejects incompatible dependency changes. Remove the patch only when a
replacement Garden revision passes the API regression tests.

## Validation Boundaries

The command-backed tests exercise real host mkdir/tar through the actual Garden
HTTP handler and official client; they do not prove sandbox user switching.
Containerd-enabled Nerd tests exercise the supplied `GARDEN_TEST_ROOTFS` in a
real runtime. Record the rootfs hash and origin: the local runner's Ubuntu image
is not evidence for a shipped CF rootfs. Live runsc acceptance and the shipped CF
rootfs checksum round trip require separately approved integration validation.