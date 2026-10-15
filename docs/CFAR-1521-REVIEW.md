# CFAR-1521 Source Review

Reviewed 2026-10-15: PR #1 head `a976d5a1565a455b059ac6dfcd4cb5a0b2dafaa6`
against `c0f0e52f51fa7679ae26cbe9e4231edac7099efd`, plus the owner-local fixes in
this commit. No deployment, release upload, pipeline control or Jira transition.

## Findings and Repairs

- Both templates correctly place the property under `create`. Added strict native
  Boolean validation: numbers, strings, collections and multiline YAML inputs now
  fail rendering with an actionable property name instead of deferring failure to
  GrootFS or introducing extra YAML fields. Negative render tests failed before
  the repair; the complete job suite now passes 84 examples.
- Clarified the actual quota contract. True globally excludes image bytes for both
  stores, including Garden total-scope requests. False preserves existing behavior:
  an explicit per-request exclusive scope can still set the CLI exclusion flag.
  Positive writable-layer quotas remain. This is an explicit accounting choice,
  not a generic alternative-runtime requirement. Custom image plugins may ignore it.
- BOSH resolves null to the declared false default before ERB; that behavior has
  an explicit regression. Both stores have true/false/default coverage.

## Primary Source and Checks

GrootFS `23f6a02bfe0dc8fe8d04461e7d6f8b912470c623` owns the Boolean at
`commands/config/builder.go` under `CreateConfig`. The builder preserves YAML when
the CLI flag is absent; the pinned Guardian image command creator supplies true
for exclusive scope and no false flag for total scope. The overlay XFS driver
still sets positive writable quotas. These are source facts, not live enforcement.

Executed on macOS arm64, Ruby 4.0.7, Bundler 2.5.23, Go 1.26.2:

```bash
BUNDLE_FROZEN=true ruby -rostruct -rlogger -rstringio -rpathname -e '
  gem "bundler", "2.5.23"
  require "bundler"
  Bundler.define_singleton_method(:local_platform) { Gem::Platform.new("arm64-darwin-21") }
  require "bundler/setup"
  load Gem.bin_path("rspec-core", "rspec")
' -- spec/jobs/garden_spec.rb
python3 scripts/verify-cfar1502.py
cd src/grootfs
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=vendor go test ./commands/config
```

All passed: 84 render examples, provenance check and pinned config suite. Frozen
Bundler needed an in-memory platform override because the lockfile lists Darwin
21 rather than Darwin 25; no dependency files were changed. Initial platform and
Ruby positional-hash fixture failures were repaired before behavioral validation.
The first strict-validation run also exposed BOSH null/default semantics; its
expectation was corrected to test the actual default rather than pretend null
reaches the template. Independent review additionally ran the 84 examples directly
and found no source blocker.

Review covered failure handling, correctness, data/config integrity, both-store
consistency, API precedence, security/type injection, diagnostics, ownership and
maintainability. No new shared state or runtime loop. No fail-open input bypass
was retained; no default accounting or runtime selection change.

## Remaining Acceptance

This release still pins nested Guardian `c30c6c4c98c8312b47549d2e2a0a4a4d6c6889a0`;
this PR does not package the parent's newer Guardian commits. The runtime companion
must be adopted with the owning Garden properties and compatible release pair.
Actual large-image/small-writable-quota writes, caller-visible over-quota failure,
full candidate/rollback deployment manifests, native builds and live runsc/CF
acceptance remain CFAR-1523 gates. Source merge alone does not establish them.