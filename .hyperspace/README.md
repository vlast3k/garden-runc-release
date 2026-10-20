# Completion-Focused PR Reviews

This repository enables automatic Hyperspace reviews on non-draft PR creation
using the adjacent self-contained completion-audit review focus. Automatic
summaries, description rewriting, Sonar Fix and Pipeline Fix remain disabled.
Documentation, tests and evidence remain in review scope.

The bot reads configuration from the PR head commit, not always from the default
branch. Existing branches need these files before requesting `/review`.
Subsequent pushes do not automatically trigger another review; post `/review`.

This is audit-only guidance, not an execution permission or enforced security
boundary. Suggestions do not establish test execution or runtime acceptance.
No runtime sources, release artifacts or submodule pins are changed by activation.

The config and focus match the policy tested in
[parent activation PR #2](https://github.tools.sap/cp-runtime/gvisor-cf/pull/2).
The parent's controlled live smoke test was closed unmerged in
[PR #24](https://github.tools.sap/cp-runtime/gvisor-cf/pull/24).
Those results are prior policy evidence, not proof of this repository's webhook.
Repository-specific checks and activation results are recorded in its activation PR.