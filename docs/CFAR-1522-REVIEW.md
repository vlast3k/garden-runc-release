# CFAR-1522 Garden Provenance Review

Observation date: 2026-10-15. Garden-only takeover, authorized by the user.
Branch: `cfar-1522-provenance-contract`.
Base: `25438bdf5eb5bcd05976811cdadf7389b2cafbaa`.
The resulting local commit is the commit containing this report; no self-referential
release SHA is embedded in the contract. No push, merge, parent pin update, Jira
write, live operation, release upload or new runtime build was performed.

## Ownership And Hypothesis

The pre-existing uncommitted provenance, tests, module configuration and Go
packaging changes were taken over and preserved. Guardian was clean at
`a4cd5ddbfd9032369f503845fd3b51d45485402a`; only the explicitly approved final
revision `2f4941f8ed804d966c6962075b2a84d74a4f88c9` was fetched and checked out.
Garden's existing `a2d6ae9c8f5b8462d3467e94ac6fab69f9cd7a93` dependency was initialized,
not upgraded. Runtime `4e2e630ab186e40fb1e0d47f51f390d2b5096087` is the
user-supplied coordination revision, not modified or independently accepted here.
All fork defaults are `main` per the takeover instruction; no default was changed.

Hypothesis: exact immutable source, repository ownership, clean initialized
worktrees and artifact identities establish local consistency without historical
commit topology. They do not establish behavior, publication or promotion.
The first added CLI fixture demonstrated all four release/Guardian hidden-index
flag cases incorrectly succeeding; rejecting the flags made the same fixture pass.
Offline, independent-history and unrelated-patch-history fixtures then passed
without weakening pin, object-existence, owner, metadata or artifact checks.

Excluded: cleanup7 implementation changes/removal, Guardian edits, runtime edits,
live infrastructure and parent-wide fork-audit or coordination changes.

## Criteria Matrix

| Criterion | Evidence | Result / Boundary |
| --- | --- | --- |
| Minimal versioned source/artifact contract | `PROVENANCE.json`, `scripts/verify_provenance.py` | Release identity, exact Guardian gitlink/HEAD, allowlisted repositories/main, exact Go archive size/hash, patch references and behavior declarations. |
| Fail closed on uninitialized or wrong source | Real CLI fixtures | Missing gitlink/object, wrong HEAD/root/remote, symlink escape, dirty source and hidden index flags rejected. No parent Git identity fallback. |
| Fail closed on malformed metadata | Real CLI fixtures | Duplicate JSON keys, wrong schema/types, unknown fields, missing revision, missing cleanup gate and invalid owner/status rejected. Metadata cannot declare a gate passed. |
| Artifact integrity | Real CLI fixtures | Missing archive, size/hash tampering and symlink escape rejected. Literal fixture archive size/hash are independent expected data. |
| Equivalent history | Real CLI fixtures | Independent source history and non-ancestor patch reference accepted when the exact declared source agrees. No network, ancestry list or ticket condition. |
| Read-only execution | Every CLI fixture | Snapshot source plus Git metadata/index/refs/objects before/after; literal successful stdout and empty stderr asserted. Optional Git writes disabled. |
| Rendered Garden configuration | Frozen RSpec run below | 84 examples, 0 failures. No spec/template changes in this slice. |
| Cleanup7 retained | Exact Guardian pin and required gate | Production cleanup unchanged. Real runsc CREATED-task gate **NOT RUN**; matching runsc/shim unavailable. |
| Stock Nerd evidence | Guardian `docs/CFAR-1522-REVIEW.md` at approved pin | Inherited report: 47 Ginkgo specs passed at `4909bdb8a7111d69db3b418f69523681d9539597`, runsc-only spec skipped. Not rerun here and not runsc proof. |
| Actual release provenance | Real CLI before/after commit | **BLOCKED**: Go archive not hydrated. Pre-commit dirty source and old committed gitlink also correctly fail. Committing resolves those two checks, not missing bytes. |
| Historical evidence | `git diff --quiet` against base | `CFAR-1502-PROVENANCE.json`, `scripts/verify-cfar1502.py` and `Gemfile.lock` unchanged. Historical verifier retains its 17 required commits as historical evidence only. |
| Publication/adoption/native build/canary | Outside Garden-local scope | Not performed; parent owner coordinates reviewed publication and integration. |

## Dependency Patch Chain

Guardian's approved `go.mod` replaces Garden with `../garden`, but its default
Go module build uses its committed vendor tree. With local Go 1.27.2:

```sh
cd src/guardian
GOPROXY=off GOTOOLCHAIN=local go list -f '{{.Dir}}' code.cloudfoundry.org/garden/server
git apply --reverse --check patches/garden-stream-out.patch
cd ../garden
git apply -p4 --check ../guardian/patches/garden-stream-out.patch
```

All three checks passed. The first resolves to
`src/guardian/vendor/code.cloudfoundry.org/garden/server`, where deferred reader
close and `http.ErrAbortHandler` after a partial stream failure are already present.
The existing Guardian patch is retained; no release packaging patch is needed for
the default vendor build. The standalone Garden gitlink lacks this fix and accepts
the patch dry-run. An explicit `-mod=mod` build is not covered by default-vendor
evidence and must not silently replace it. No nested source was patched here.

The Go packaging edits were already user-owned: both package spec and extraction
command now name `go1.26.3.linux-amd64.tar.gz` exactly instead of wildcards. The
existing version and blob manifest agree: size `66862230`, SHA-256
`2b2cfc7148493da5e73981bffbf3353af381d5f93e789c82c79aff64962eb556`.
The archive bytes are absent, so this is manifest consistency, not binary proof.

## Commands And Environment

macOS arm64; Python 3.14.7 CLI fixtures; Ruby 4.0.7; local Go 1.27.2. From the Garden
worktree, unless a directory change is shown:

```sh
python3 -B -m unittest discover -s scripts/tests -p 'test_*.py'
RUBYOPT=-rostruct BUNDLE_FROZEN=true ruby -e 'require "bundler"; def Bundler.local_platform; Gem::Platform.new("arm64-darwin-21"); end; require "bundler/setup"; load Gem.bin_path("rspec-core", "rspec")' -- spec/jobs/garden_spec.rb
python3 -B scripts/verify_provenance.py --root "$PWD"
git diff --check
git diff --quiet 25438bdf -- CFAR-1502-PROVENANCE.json scripts/verify-cfar1502.py Gemfile.lock
git -C src/guardian diff --check
git submodule status
```

CLI suite: 46 tests passed, including actual subprocess exit codes and diagnostics.
RSpec: 84 examples, 0 failures. Ordinary frozen Bundler first rejected Darwin 25;
overriding RubyGems' local platform hid native extensions. The accepted command
overrides only Bundler's platform selection in memory, not the shared lockfile.
Installed only the lockfile-pinned `ffi` 1.16.3 extension for Ruby 4. The RSpec run
emitted an ostruct warning but no failures. No dependency versions were changed.
Python/JSON editor diagnostics reported no errors.

The source checker returns exit 1 with a missing hydrated archive diagnostic.
This is an expected negative result, not source acceptance. Do not manufacture
archive fixtures in the real release, waive hydration, or label this check passed.
The CLI's exit 0 means contract consistency only; exit 2 means metadata invalid.

## Remaining Behaviors

| Behavior | Owner / Rationale | Test Anchor / Upstreamability / Removal Gate |
| --- | --- | --- |
| Exact source and archive integrity | Garden release; prevent ambiguous builds | CLI fixtures; reusable downstream release tooling. Replace only with equivalent reviewed integrity checks. |
| Runtime/GrootFS config rendering | Garden release; authoritative BOSH properties | `spec/jobs/garden_spec.rb`; upstreamable property contracts, no changes here. Remove only with caller/parser/quota compatibility evidence. |
| Stream-out HTTP failure propagation | Guardian vendored Garden; preserve CFAR-1517 partial-stream errors | Existing patch and Guardian streaming tests; upstream Garden candidate. Remove only when selected dependency provides equivalent tested behavior. |
| CREATED-task cleanup7 | Guardian / CFAR-1508; avoid leaked tasks/processes | Required Nerd alternative-runtime test anchor. Run real matched runsc/shim lifecycle with process and metadata disappearance before considering replacement. |

## Audit And Handoff

This is a Garden-only audit, not the parent's cross-repository inventory. Relative
to base, production Go, generated fakes and render tests are unchanged. Build/config
changes comprise two exact-archive replacements, two module URL/branch replacements,
the Guardian gitlink adoption, and two Python ignore lines. New provenance code,
CLI tests and documentation are classified separately, not production runtime LOC.
Measured relative to base: production Go `+0/-0`, generated `+0/-0`, Python CLI
tests `+583/-0`, provenance verifier/data `+518/-0`, build/config including gitlink
and ignore rules `+7/-5`. Documentation is separate and its exact line count is
available in the same numstat output; it is not runtime complexity.
No code was relocated and no runc source/package was changed. Reproduce added and
deleted counts with `git diff --numstat 25438bdf HEAD`; net is added minus deleted,
not the total churn. The parent owner retains the combined fork classification.

No independent subagent review was available in this session; the matrix records
direct checks only. Any delegated/reviewer follow-up must preserve **BLOCKED**
hydration and **NOT RUN** runsc states, not convert them to acceptance.
Rollback source inputs are the base commit and previous Guardian pin; reverting
source is not a live rollback authorization. Next discriminating checks: hydrate
the exact recorded Go archive through the approved blob workflow and rerun the
unchanged read-only CLI on the committed clean release; then execute the separately
required real-runsc CREATED-task lifecycle gate with release-matched binaries.

## Independent Review Repair

Observation date: 2026-10-15. Authorized takeover of the clean existing worktree
at `4cc0277b1e6cf50ddac05a0d67d135eb6b9e4feb`, on the same branch. This section
supersedes the original 46-test result for the verifier. The repair is the local
commit containing this section; no push, merge, parent edit, Jira write, template
change, infrastructure operation or artifact hydration is included.

The independent review identified eight executable false passes. The original
verifier matched YAML text without checking Ruby's parsed values, accepted a
matching extraction command inside unreachable shell code, and skipped empty Git
identities through truthiness guards. The repair changes only:

- `scripts/verify_provenance.py`: compare structured YAML values, validate the
	supported Go packaging command sequence, and compare Git identities even when
	empty or unavailable. Git command errors remain explicit failures.
- `scripts/yaml_to_json.rb`: Ruby Psych subprocess helper; inspect the YAML AST
	recursively for duplicate scalar keys, require one document and a mapping,
	reject merge keys, and materialize with `Psych.safe_load` without aliases or
	permitted classes. No YAML line matching remains in the verifier. The requested
	runtime helper was not present in either available runtime checkout, so this
	uses the same AST/safe-load helper pattern rather than claiming a byte-for-byte
	copy of unavailable source.
- `scripts/tests/test_verify_provenance.py`: eight regression cases and five
	supplementary tests, all exercising the actual CLI subprocess. Every new
	invalid-input test first verifies its own valid fixture. Invalid tracked input
	is committed before checking rejection, so dirtiness is not the rejection
	mechanism. The existing snapshot assertion covers source and Git metadata
	before and after every accepted or rejected verifier invocation.
- This report. Parent build-document corrections remain with the parent owner.

| Regression | Invalid input now rejected |
| --- | --- |
| Duplicate quoted release key | An apparently correct `final_name` followed by `"final_name": wrong`. |
| Unused package archive | Expected archive under `unused`, with a different archive in actual `files`. |
| Duplicate blob archive | A quoted duplicate archive entry overwrites the apparent size/hash. |
| Dead extraction | Correct `tar xzf` inside `if false`, followed by executed `tar -xzf wrong.tar.gz`. |
| Empty release origin | Successful Git config query returns an empty URL. |
| Empty recorded Guardian URL | Empty URL in committed `.gitmodules`. |
| Empty recorded Guardian branch | Empty branch in committed `.gitmodules`. |
| Empty Guardian origin | Successful Guardian Git config query returns an empty URL. |

Packaging validation intentionally supports only the existing seven straight-line
commands, parameterized by the validated archive name, with blank/comment lines
ignored. Shell control flow, extra commands and alternative extraction spellings
fail closed; this is not an arbitrary-shell interpreter. A Bash fixture sources
the supported script with mocked tar/cp/mkdir/sed commands and asserts the actual
tar arguments. The same harness proves the dead-code example executes only the
wrong archive even though Bash returns zero. The fixture archive is deliberately
not a real Go distribution; these checks do not prove BOSH compilation.

Supplementary CLI coverage includes quoted/flow-style valid YAML, second
documents in all three YAML inputs, quoted duplicate package keys, malformed or
non-string `files` values without tracebacks, and missing Git identities (the
`None`/command-error path). Blob size types and `sha256:` hash format are checked
before comparison.

### Repair Validation

macOS arm64, Homebrew Python 3.14.7 and Ruby 4.0.7. Commands run from this worktree:

```sh
python3 -B -m unittest discover -s scripts/tests -p test_verify_provenance.py
RUBYOPT=-rostruct BUNDLE_FROZEN=true ruby -e 'require "bundler"; def Bundler.local_platform; Gem::Platform.new("arm64-darwin-21"); end; require "bundler/setup"; load Gem.bin_path("rspec-core", "rspec")' -- spec/jobs/garden_spec.rb
ruby -c scripts/yaml_to_json.rb
git diff --check
python3 -B scripts/verify_provenance.py --root "$PWD"
```

Results: **59 CLI tests passed** (128.539 seconds); **84 RSpec examples, zero
failures**; Ruby syntax OK; diff whitespace check clean; editor diagnostics clear
for the three code files. The RSpec run retains the existing Ruby 4 ostruct
warning and the in-memory Bundler platform override; dependencies and lockfile
were not changed.

Discriminating old-source control: `git archive 4cc0277b
scripts/verify_provenance.py` extracted the original verifier into an owned
temporary directory. The new test module's `VERIFIER` was pointed at that copy,
and `unittest.TestSuite` selected exactly the eight regression tests above.
All eight baseline invocations passed, and all eight invalid fixtures incorrectly
returned exit zero: **8 tests, 8 expected failures, zero errors** (19.537 seconds).
The harness explicitly required eight `0 != 1` assertion failures, then removed
the extracted copy. The active worktree verifier was never replaced or mutated.

The real CLI correctly returns exit 1 before commit for dirty release source and
the missing hydrated Go archive. On the clean committed repair it was rerun:
exit 1, with the missing archive as the only failure and no dirty-source error.
Archive hydration, actual release acceptance, native Linux
packaging and real-runsc CREATED-task validation remain **BLOCKED / NOT RUN** as
described above; no fixture bytes were placed into the real release. Next check:
hydrate the exact recorded archive through the approved workflow and rerun the
read-only CLI on the clean committed source. No live or publication approval is
implied by these source tests.

## Second Review Repair

Observation date: 2026-10-15. Garden-only repair from clean local commit
`29ec8a4c958447dfd7af041e246ffe6710a9cb76` on the existing branch. The resulting
revision is the local commit containing this section. No push, merge, parent pin
edit, runtime/Guardian edit, Jira write, artifact hydration or live operation was
performed. This section supersedes the preceding 59-test count.

Hypothesis and first check: Python's Unicode line splitting and whitespace
normalization can turn a Bash comment into an apparent extraction command; raw
YAML AST key spellings can differ from the keys Psych constructs. Add actual CLI
regressions first, with clean committed invalid fixtures and valid controls, then
run the narrow selection against unchanged source before repairing it.

The pre-fix run selected four new test methods: **4 tests, 7 failing assertions,
zero errors**. Five invalid inputs falsely returned CLI exit zero: VT-commented
extraction, binary-key overwrite, and binary/bool/int tagged release-name values.
Two more assertions exposed the old raw-duplicate diagnostic for boolean/integer
keys rather than rejection of their non-string types. The explicit-string escaped
duplicate test already passed and is retained as a positive rejection control.
The Bash witness passed before the CLI rejection assertion: the unmodified script
called mocked tar with the exact expected archive; after replacing its extraction
with literal `#\vtar xzf ...`, Bash returned zero and observed **zero tar calls**.

Changes are restricted to:

- `scripts/verify_provenance.py`: preserve decoded source bytes without universal
	newline translation; reject every packaging character except printable ASCII,
	TAB and LF, split only on LF, trim only space/tab margins, and compare the
	existing seven-command sequence exactly. No generic shell parser was added.
- `scripts/yaml_to_json.rb`: reject non-string explicit tags before construction,
	use Psych's restricted scalar scanner for implicit key types, require string
	keys, and compare decoded key values for duplicates before safe loading.
	The requested runtime `load_yaml.rb` reference was not present in the available
	runtime checkout (including tracked filename discovery); this implements the
	requested semantics directly, not a claimed source copy. Runtime is untouched.
- `scripts/tests/test_verify_provenance.py`: six new CLI test methods covering
	the Bash/VT witness, control/non-ASCII characters (including CR before it can be
	normalized), accepted ASCII margins/comments, binary/bool/int key collisions
	and tags, and explicit-string Unicode-escaped duplicate keys. Every verifier
	invocation retains the source/Git snapshot immutability assertion.
- `docs/PROVENANCE.md` and this report: explicit limited-format contract and
	reproducible validation evidence.

### Second Repair Validation

macOS arm64, Homebrew Python 3.14.7 and Ruby 4.0.7. Commands from this worktree:

```sh
PATH=/opt/homebrew/bin:$PATH python3 -B -m unittest discover -s scripts/tests -p 'test_*.py'
PATH=/opt/homebrew/bin:$PATH RUBYOPT=-rostruct BUNDLE_FROZEN=true ruby -e 'require "bundler"; def Bundler.local_platform; Gem::Platform.new("arm64-darwin-21"); end; require "bundler/setup"; load Gem.bin_path("rspec-core", "rspec")' -- spec/jobs/garden_spec.rb
PATH=/opt/homebrew/bin:$PATH ruby -c scripts/yaml_to_json.rb
git diff --check
```

Results: **65 CLI tests passed** (166.898 seconds in the owned iTerm session,
explicit `__GARDEN_ALL_TESTS_RC=0__` and dispatch exit-zero markers); the immediate
six-method focused rerun passed in 31.003 seconds. **84 RSpec examples, zero
failures**; Ruby syntax OK; whitespace and editor diagnostics clean. The existing
Ruby 4 ostruct warning and in-memory frozen-Bundler platform override are unchanged.
An earlier shared-terminal full-suite attempt was interrupted by concurrent
terminal reuse and is not counted as a passing run. The iTerm operation API did
not confirm completion; its completed summary and exit markers were recovered
once from owned-session scrollback rather than inferred from stability.

These are source-contract and render tests, not native Linux packaging or runtime
acceptance. The real release still lacks the hydrated Go archive. That gate
remains **BLOCKED**, and matched runsc/shim CREATED-task validation remains
**NOT RUN**. The next discriminating check remains approved hydration of the exact
recorded archive and the read-only CLI on clean committed source; no acceptance
gate was waived by this repair.