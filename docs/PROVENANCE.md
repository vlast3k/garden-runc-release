# Release-local contract consistency

`PROVENANCE.json` is the active, versioned release-local source contract. Run:

```sh
python3 -B scripts/verify_provenance.py --root "$PWD"
python3 -B -m unittest discover -s scripts/tests -p 'test_*.py'
```

The verifier checks consistency and fails closed unless the release name, release origin, Go package and
hydrated archive bytes, Guardian gitlink, initialized Guardian worktree root,
Guardian HEAD/origin, recorded `main` branch, declared revision existence, and both
source worktrees agree exactly and are clean. Hidden `assume-unchanged` and
`skip-worktree` index flags are rejected in both repositories. The
hydrated Go archive must be a regular non-symlink file below the approved `blobs/`
root and match the recorded size and SHA-256. Initialize Guardian with
`git submodule update --init src/guardian`.

The Garden and Guardian repository URLs and Guardian `main` branch are fixed
allowlist values, not metadata-selected trust roots. The command is offline and
read-only: it neither fetches nor queries a live branch, runs behavior tests, or
hydrates artifacts. It disables optional Git index writes and ignores inherited
`GIT_*` environment overrides. Publication and parent adoption are separate checks.
Neither current branch reachability nor historical patch ancestry is an acceptance
condition. Equivalent reviewed rebases or squashes can supply new exact revisions;
the recorded gitlink, initialized HEAD, metadata and review evidence must agree.

`behavior_gates` and `downstream_patches` provide structured ownership, revision,
application-check and executable-test anchors. They identify required acceptance
work. The verifier confirms that each selector is present in its anchored file,
but presence does not prove a test ran, passed, or that a source marker provides
the behavior. In particular, the CREATED-task cleanup gate remains required until
its separate lifecycle replacement gate passes.

The nested Guardian revision is the reviewed immutable
`2f4941f8ed804d966c6962075b2a84d74a4f88c9`, not a floating branch update.
The cleanup patch revision is a source reference, not required ancestry or proof
that cleanup ran. Its declared object must be locally available. The executable
cleanup gate remains **NOT RUN** here because matching runsc/shim binaries are
unavailable. The stock Nerd result recorded in Guardian is not runsc acceptance.
See [the Garden review matrix](CFAR-1522-REVIEW.md) for checks and remaining gates.

Exit status `0` means local contract consistency only; `1` means source/artifact
checks failed; `2` means invalid, missing, duplicate-key or unsupported metadata.
Missing artifact hydration is a blocking result, never a successful acceptance
with a warning. The CLI fixtures use disposable repositories and assert literal
expected output plus unchanged source, Git index, refs and object snapshots.

This standalone result is not promotion, publication, or adoption proof. The
release revision is externally bound only when the owning parent repository adopts
this release through its gitlink. Arbitrary coordinated edits to local metadata,
toolchain bytes, and verifier code remain visible review changes, but can still be
self-consistent; checking the release origin and clean worktree does not create an
external trust anchor. The release revision is therefore not self-declared here,
because embedding the commit that contains the declaration would also be circular.

`CFAR-1502-PROVENANCE.json` and `scripts/verify-cfar1502.py` are retained unchanged
as historical evidence. The ticket-specific verifier reproduces its old acceptance
path but is not the current contract.
