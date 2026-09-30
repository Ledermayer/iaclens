# Change-based validation

The CI workflow exposes one required **Validation gate**. It selects a route before
setting up Go or running any examples.

| Changed inputs | Required work | Examples |
|---|---|---|
| Go, go.mod/go.sum, embedded rules, scripts, workflow/build inputs | Tests on Linux/macOS/Windows and release packaging first | All, using the tested Linux binaries |
| Particular example source, rules.yaml, expected.json | Build CLI/runner only | Only affected examples |
| Generated reports mixed with docs or example changes | Rerun every changed report's owning example, plus changed source examples | Union of affected examples |
| Documentation | Change classification only | None |
| Generated reports only | Verify permitted paths and successful direct parent | None |
| Unknown shared inputs | Full CI | All |

Offline assertions are required; live failures and timeouts are advisory. Live
runs have a three-minute limit per example. All results are retained as artifacts;
a successful gate permits committing selected reports to the feature branch.
Unchanged examples' report files are left untouched. Offline failures retain
artifacts without publishing a new commit on top of an unvalidated parent.

Full CI also runs pinned `govulncheck` on Linux; a reachable dependency
vulnerability fails the required test job. Release validation repeats this check
before packaging. Documentation/results-only routes do not run a new dependency
scan. Every release archive includes generated dependency and Go redistribution
notices, and packaging verifies their contents in all six archives.

## Baseline and evidence

The planner searches the branch's first-parent history for a successful Validation
gate from this CI workflow. The run's validation-plan artifact must match both that
commit and the PR's current base SHA. Changes since that proven ancestor determine
the route. Without reusable evidence, the planner compares against the merge base
and validates the complete PR diff. Moving the PR base invalidates cached evidence.
The search is bounded to 50 ancestors / 100 runs; evidence is retained for 90 days.

A results-only commit must have a directly validated parent and may only change
report.json, report.yaml, run.json, or summary.md inside that parent's selected
examples/results/offline or examples/results/jev directories. Commit messages,
bot identity, and manually supplied flags are not evidence of successful tests.

Adding a documentation change does not preserve that exemption: mixed changes
rerun the affected examples even when parent evidence exists. Shared code still
selects full validation. All files under an example's source directory, including
Markdown, count as fixture inputs before documentation exclusions are applied.

The publisher uses `[skip ci]` to suppress push/PR triggers and explicitly dispatches
the same CI workflow on the new head. That dispatch performs only planning and the
required gate. It does not compile, test, call Jev, or publish again. This gives the
latest commit its required check without repeating expensive CI. Because GitHub
can require a check on the synthetic PR merge commit after a skipped PR workflow,
that same dispatch publishes its successful decision there for every route. It
checks the current PR head, base, and both merge parents exactly before doing so.
A `pull_request` run must not publish that decision: its head is the pre-results
parent, so attesting it would hide the generated commit. The publisher never
copies a success to an unverified source or changed base. The main branch
requires only Validation gate; that gate enforces all work selected for its route.

Fork/Dependabot PRs can pass offline validation without live credentials or report
publishing. Publishing uses a separate job with write permissions, executes only
inline workflow commands, and never force-pushes over newer changes.
