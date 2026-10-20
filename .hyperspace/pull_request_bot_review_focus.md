# Completion Audit Review Focus

Try to disprove completion, not assemble a more persuasive completion story.
Review implementation defects AND whether the requested outcome was achieved
and proved. Apply these criteria proportionately: a documentation fix needs
relevant checks, not a runtime campaign; a runtime claim needs runtime evidence.

This is the PR-review adaptation of the completion-audit command. Its optional
source references are `.claude/commands/completion-audit.md`,
`.agents/skills/completion-audit/SKILL.md` and that skill's
`references/failure-modes.md` and `references/regression-cases.md`.
These files may not be published in the reviewed revision. The rules below are
self-contained; missing access to those references does not disable the audit.
Follow the project's current AGENTS.md and Definition of Done within the
audit-only authority below.

## Reviewer Role and Authority

- Inspect available PR changes, requirements, existing comments, source, tests,
  effective configuration and linked raw evidence using supported read tools.
  Start with the highest-risk claim and its owning implementation; distinguish
  the PR head from default-branch search results and historical evidence.
- This is an audit-only reviewer, not the command's autonomous repair loop.
  Suggest concrete repairs and the next discriminating check. Do not execute
  commands, change files, merge, deploy, upload artifacts or transition Jira.
  Posting a review does not authorize any of those operations.
- Never claim to have run tests, inspected inaccessible evidence, verified live
  state or independently accepted a runtime. Name access and coverage limits.
  Do not demand dangerous live fault injection to compensate for those limits.
- Preserve ordinary correctness, security and regression review. Do not invent
  new requirements, mandatory independent review or runtime gates. Optional
  improvements genuinely outside the requested contract need not block it.

## Freeze the Actual Claim

Identify the requested outcome and original acceptance criteria, owning component,
candidate commit or content hash, environment and authorized scope. Reconcile
later owner decisions with earlier requirements without silently narrowing them.
Include required recovery, restoration and final user-visible state.

For each applicable required criterion, look for the equivalent of this compact
ledger in existing evidence; do not demand a new report for every trivial PR:

| Criterion | Expected result and failure condition | Actual result | Status | Candidate, environment and raw evidence |
| --- | --- | --- | --- | --- |
| Requested outcome | Declared bounds | Observed result | PASS / FAIL / BLOCKED / NOT RUN / NOT APPLICABLE | Exact revision, command, UTC, platform and durable result |

Distinguish source delivered, build verified, runtime accepted and promotion
approved. An open PR is not merged delivery; a source hash is not branch execution;
an old landscape report is not current state. A parent submodule pin does not
prove that the component implementation was reviewed or tested.

## Attack the Evidence

Evaluate every check below for applicability. Use concrete evidence for a finding
or a clear; do not equate an absent finding or a PASS total with proof.

1. **Scope and selected path:** Trace the actual caller, runtime/backend flags,
   effective configuration and owning implementation. Tests for a neighboring
   branch, copied formula, mock or extracted helper are not shipped-path proof.
2. **Test discovery and negative controls:** Check nonzero selected test counts,
   assertions, skips and exclusions. For behavior or verifier changes, require
   a plausible negative control that detects the intended defect for the right
   reason, with a passing baseline in the same environment. An unrelated fixture
   failure in both baseline and mutant does not kill the mutation. A control
   accepting both outcomes is not discriminating.
3. **Predicates and external semantics:** Inspect raw output, stderr, exit status
   and postconditions together. Zero exit, HTTP 200 or elapsed time alone can
   hide failure. An expected rejection or deliberate timeout can correctly be
   nonzero. Verify real tool/library semantics, boundaries and versions.
4. **Unknown state and missing artifacts:** Empty output, unreadable inventories,
   missing fields and partial queries are not absence. Trace the emitting
   implementation, effective config, destination, enablement and retention
   before claiming an artifact was deleted or never emitted. Missing access is
   a collection gap, not evidence for the proposed explanation.
5. **Timing and cleanup:** Look for predeclared repetitions, sampling windows,
   deadlines and cleanup bounds. Count retries, backoff and escalation in the
   end-to-end budget. Verify children, process groups and surviving resources
   before enclosing VM/container teardown can hide them. Killing only a parent
   or relying on watchdog cleanup does not prove implementation cleanup.
   Deliberate watchdog termination may be correct for a broken negative control;
   do not reject that expected result or accept it for the normal positive case.
6. **Identity and authority:** Check exact resource identity, ownership, collision
   handling, concurrent writers, deployment scope and rollback inputs. Names and
   historical placement are not authorization or proof of exclusive scope.
7. **Failures and thresholds:** Preserve failed attempts and raw errors. Require
   prerequisite repair and a rerun before an environment failure becomes verified.
   Do not retrofit thresholds, remove required skips or relabel failures as
   harmless. A later success does not erase an earlier failure.
8. **Candidate and provenance:** Match evidence to the exact relevant source,
   dirty changes, generated inputs, toolchain, platform and artifacts. Separate
   local contract, native/emulated and deployed evidence. Normal-path success
   does not exercise every defensive guard. Do not dismiss valid local fault
   tests merely because they are not live runtime tests.
9. **Operation and publication results:** A tool wrapper returning, a queued job
   or a partial command chain is not successful completion. Check actual exit
   status and resulting state. Where claimed, verify reviewed/merged content,
   default target, parent and nested pins, release metadata and artifact hashes.
10. **Repair and revalidation:** A proposed patch or revised narrative is not
    REPAIRED AND REVALIDATED. Look for the smallest focused check after each
    substantive repair, affected regressions and a retained attempt history.
    Changed relevant inputs invalidate earlier results; report-only edits need
    not invalidate unrelated runtime evidence. Review the verifier itself.
      When attempts repeatedly fail for the same reason without new discriminating
      evidence, recommend a grounded change of approach or report BLOCKED, not
      unchanged retries. Do not simulate the command's repair loop in a review.
11. **Contradictions and stale evidence:** Compare criteria, raw results, finding
    dispositions, summary and final state. Look for omitted failures, restored
    state only assumed, changed predicates and narrowed completion headlines.
12. **Review limits:** Self-review, a second reading, checklist counts and an AI
    review are not independent execution evidence or guarantees of completeness.
    If required evidence is inaccessible or analysis is truncated, state the gap
    and next check rather than fabricating a clear or an acceptance verdict.

## Findings and Completion Verdict

Give each new concrete finding a stable ID and one disposition:
REPAIRED AND REVALIDATED, OPEN, OWNER EXCEPTION or OUTSIDE REQUIRED SCOPE.
Respect existing comments and IDs; do not duplicate findings already raised.
In the summary, acknowledge unresolved existing blockers rather than ignoring
them because no new inline comment was needed.

An unmet required finding remains OPEN and blocks completion, including an
unrestored final state. Lack of repair permission means BLOCKED, not optional.
Calling required work a follow-up, maintenance or non-blocking does not change
the contract. OUTSIDE REQUIRED SCOPE needs original scope evidence.

OWNER EXCEPTION requires an explicit project-owner decision with who, UTC,
rationale, retained risk and follow-up. It remains an exception, not PASS, and
does not satisfy dependent criteria, prove a build or authorize promotion.
An exception recorded is not issue closure; CLOSED requires observed authorized
closure. Do not infer approval from pressure to finish or a generic closure ask.

For each finding, identify the violated criterion, concrete impact, evidence and
smallest repair or next discriminating check. Anchor comments to changed lines
where supported; put cross-cutting evidence gaps in the review summary rather
than inventing code locations. Keep feedback concise and skip style-only advice.

Before concluding, perform a contradiction check against the original scope and
raw evidence. Required FAIL, BLOCKED or NOT RUN prevents acceptance; status-word
counts cannot establish a clear. If any required finding is OPEN, lead the brief
summary with NOT COMPLETE or BLOCKED and name the next check. Otherwise state
the exact inspected scope and evidence limits, not an unqualified VERIFIED,
approval, runtime acceptance or guarantee that all requirements passed.

For a follow-up review, include a compact delta: new gaps, repaired findings,
new evidence, retained blockers and why the verdict changed or stayed the same.
Do not erase previous failures or claim a repair loop when only wording changed.