# Release-local contract consistency

`PROVENANCE.json` schema 2 describes the canonical Garden monorepo layout.
Guardian remains its own Go module at `src/code.cloudfoundry.org/guardian`,
but is inline source rather than a separate Git worktree or gitlink.

```sh
python3 -B scripts/verify_provenance.py --root "$PWD" \
	--toolchain-package /absolute/path/to/golang-source-package.tgz
python3 -B -m unittest discover -s scripts/tests -p 'test_*provenance.py'
```

The toolchain input is the **BOSH source package**, not the raw Go archive or a
compiled BOSH package. Obtain the exact `packages/golang-1.27-linux.tgz` member
from a verified source-release archive. This command neither hydrates packages
nor accesses a director, starts workloads or runs the declared behavior gates.

## Inline Guardian identity

The release must have its fixed team origin and a clean tracked/untracked
worktree, with no hidden `assume-unchanged` or `skip-worktree` index flags.
`guardian.tree` must match the committed `040000 tree` entry at the exact inline
path. A gitlink, nested repository, missing module or symlink source path fails.
Changing the inline source, including its contracts and tests, requires updating
that tree identity. The metadata is outside the declared subtree, avoiding a
self-referential hash. The upstream Garden commit must exist locally.

`ported_revision` names the independent Guardian source reconciled during the
port; it is a historical source reference, not a claimed ancestor of the Garden
monorepo. Reviewed behavior disposition and executable tests are still needed.
Merely recording the old commit does not prove that every change was retained.

## Vendored Go identity

The package name and fingerprint must match `spec.lock`. The matching
`.final_builds/packages/<name>/index.yml` entry must declare that fingerprint
and the exact SHA-256 of the hydrated source package. The package must contain
only the packaging script, compile/runtime environment files and declared Go
archive. Duplicate, unsafe or non-regular members are rejected without extraction.
The verifier then checks the inner archive SHA-256 and `go/VERSION`.

For the selected canonical baseline, these are Go **1.27.3**, source-package
SHA-256 `b49ef5a44acad90bd5e2504d2235a6630c3add2fb7c63848c90c4b46f16f9f83`
and inner-archive SHA-256
`65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`.
The outer package hash also binds the exact packaging and environment scripts;
schema 2 does not substitute the old raw-blob packaging grammar for those bytes.
Missing hydrated bytes remain a failure, never a warning or a metadata-only pass.

YAML parsing retains the single-mapping, duplicate-key, tag, alias and merge-key
rejections. JSON rejects duplicate keys and unknown fields. Git commands ignore
inherited `GIT_*` overrides and disable optional index writes.

## Behavior and delivery boundaries

`behavior_gates` and `downstream_patches` retain ownership, source references and
executable-test anchors. Selector presence does not mean execution or a PASS.
CREATED-task cleanup7 remains required until its separate live replacement gate
passes. Historical standalone evidence is not acceptance of this port.

Schema 1 remains implemented and regression-tested for historical standalone
contracts; its gitlink and raw-Go-blob checks have not been relaxed. The active
monorepo metadata uses schema 2 and cannot pass through the schema-1 checks.

Exit status `0` means local contract consistency only; `1` means source/artifact
checks failed; `2` means invalid or unsupported metadata. A dirty experimental
worktree deliberately cannot pass full source verification. Fixture tests may
pass before publication, but do not change that requirement.

Passing this verifier is not a BOSH compilation, runtime acceptance, publication,
parent adoption or promotion approval. Coordinated metadata/source edits can be
self-consistent; reviewed component and parent revisions provide the external
binding. Keep exact artifact hashes and test environments in the migration ledger.
