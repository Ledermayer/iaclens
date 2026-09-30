# Dependency Updates

Dependabot continues to propose weekly Go module and GitHub Actions updates.
The `Dependabot merge policy` workflow automatically requests protected squash
merges for eligible updates. It does not create releases or change repository
visibility.

## Merge Policy

| Update | Decision |
| --- | --- |
| Known stable patch/minor update | Merge automatically after current-head full CI and native merge requirements pass. |
| Major update | Also require a non-dismissed approval on the current head from a human with current write, maintain, or admin permission. |
| Unknown dependency, prerelease, new/deleted file, toolchain/directive change, or unrelated code change | Hold for manual review and merge. |
| Grouped update | Check every member; any major requires approval and any unsupported member holds the entire PR. |

Eligible PRs must be open, non-draft, target this repository's protected `main`,
and contain exactly one API-verified Dependabot commit. Metadata comes from the
signed commit, not an editable PR title, label, or body. Manually edited or
multi-commit dependency PRs remain manual. The successful PR CI run must also have
Dependabot's authenticated triggering identity; a human-signed commit with a
spoofed Dependabot author cannot grant eligibility.

Let Dependabot perform branch updates, or request `@dependabot rebase`; do not use
GitHub's **Update branch** button for PRs managed by this controller. That button
adds a human merge commit, which intentionally fails the single-bot-commit rule.
CI keeps bot-authored PRs offline and never commits reports to them, even when a
human triggers the run. Non-main manual dispatches also remain offline.

Go changes may update stable versions of dependencies already declared in the
base revision. The declared dependency set, Go/toolchain directives, replacements,
and other content must remain unchanged. Checksum differences must belong to the
versions actually updated. An update that introduces a new transitive dependency
requires review rather than silently expanding the trusted dependency set.

Actions updates may change full SHA pins of GitHub-owned `actions/*` references
already present in existing workflow files. Commands, permissions, triggers,
inputs and other workflow structure cannot change. Informational comments and
formatting are not execution policy. Version severity is taken from the verified
Dependabot metadata, while Go version severity is also checked against the files.

The controller re-reads the PR, CI and approvals immediately before a merge and
requires the base to remain the exact main revision whose full CI was verified.
It passes the expected head SHA to GitHub's merge API. GitHub enforces the existing
required Validation gate, up-to-date branch requirement and resolved conversations.
No admin bypass or automatic approval is used. A changed head or dismissed approval
is not reusable; Dependabot rebases can require a new approval for major updates.
The API's expected-head guard is atomic; approval dismissal and permission changes
are checked immediately beforehand but are not an atomic condition of that API.

## Execution And Recovery

Production runs use only code at `github.workflow_sha` on `main`. The write job
never checks out a PR, downloads its artifacts, or executes proposed dependency
code. It reads bounded files through the API and parses them as data. PR previews
have a read-only token and the command defaults to dry-run.

The workflow runs after CI completes and at minutes 23 and 53 each hour. It merges
at most one PR per run, and waits for successful full CI on current `main` before
considering another. The schedule handles approvals arriving after tests, missed
events, concurrency coalescing and the workflow-chain depth limit.

After a bot-token merge, the controller explicitly dispatches CI on `main` because
the push event may not trigger it. If main advances before dispatch, CI validates
the current main revision, not a claimed historical revision. No release relies
on that assumption: the release workflow always validates its explicit tag.
If dispatch is interrupted, reconciliation detects missing CI and retries dispatch.
An existing failed run holds further merges and requires investigation/rerun.

Set repository variable `DEPENDABOT_AUTOMERGE_ENABLED=false` to pause automatic
merges. Read-only previews remain available. The workflow's manual `apply` input
defaults to false; applying is accepted only from the main-branch controller.
No personal access token, App installation, or new action allowlist is required.

If an existing dependency PR contains a human merge commit or CI-generated report
commits, inspect its changes before requesting recovery. Ordinary Dependabot
rebasing may refuse branches with extra commits. `@dependabot recreate` overwrites
edits and therefore requires explicit approval after confirming what would be
replaced. A recreated head needs new full PR CI and, for major updates, renewed
human approval. Do not bypass a missing Validation gate or attach success to an
unverified revision to recover a PR.

Native queued auto-merge is deliberately not enabled: with zero globally required
approvals, queued enrollment can outlive a head-specific major-update approval.
The workflow instead makes a fresh protected, commit-specific squash request when
ready. This retains automatic merging without leaving a pending authorization.

## Grouping

Grouping is not enabled by this change. Recommended follow-up: separate weekly
patch/minor groups for Go modules and GitHub Actions; keep major upgrades and
security updates individual. The controller already evaluates all metadata entries
so grouped updates cannot hide a major or unknown member.

## Release Policy

RC releases and the first stable `0.1.0` remain explicit tag decisions. No
dependency merge creates a tag or runs the Release workflow automatically.

- Actions-only updates normally do not justify a CLI release.
- Updates to libraries compiled into the CLI belong in the next planned release.
- Reachable security fixes in shipped binaries should receive a prompt release.
- Breaking behavior or supported-toolchain changes require an explicit version decision.

After `0.1.0`, batched patch-release automation can be proposed separately. Merging
a dependency fix does not patch the binaries users have already downloaded.