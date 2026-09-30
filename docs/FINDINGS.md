# Release Readiness Findings

Audit owner: Ledermayer. Baseline: `v0.1.0-rc.2`.

| Finding | Resolution in this branch | Regression evidence |
| --- | --- | --- |
| Reachable GO-2026-5970 in `golang.org/x/text v0.31.0` | Upgrade to fixed `v0.39.0`; block full CI and release validation on `govulncheck` findings. | Vulnerability scan and existing Go tests. |
| Generated reports mixed with Markdown bypassed parent-evidence validation | Rerun report owners for mixed changes; keep the verified results-only route restricted to results-only commits. | Routing tests cover both path orders, cross-example changes, parent evidence, and shared-input precedence. |
| Release archives omitted third-party redistribution notices | Generate notices and exact-version source links for the six-target dependency union, including Go, and verify inclusion in every archive. | Notice collection tests and the six-platform release packaging job. |

These changes require a passing pull request before merge. They do not modify
published RC1/RC2 assets or establish that a future release is ready without its
own checks. The report schema and analysis policy are unchanged.

## Dependabot CI Identity

A human updating Dependabot PR #2 passed the old actor-only CI trust checks.
CI consequently ran live examples and added a generated-results commit with
`[skip ci]`; GitHub did not associate the follow-up checks with the PR head, and
the multi-commit branch was ineligible for automatic merging.

The CI guards now check both trigger actor and PR author, keep missing-author and
non-main-dispatch contexts offline, and never publish reports to bot-authored PRs.
`TestCITrustGuards` evaluates the actual workflow predicates against the failing
case plus trusted-human, fork, bot, dispatch, cancellation and failed-gate cases.
The controller's provenance and approval requirements remain unchanged. Existing
affected branches require inspected Dependabot recovery, not a protection bypass.