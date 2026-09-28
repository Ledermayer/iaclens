# Result storage decision: provisional

iaclens is a CLI repository, not a fleet metadata database. The output destination
belongs to the caller (`--out`, `--format`), not to an implicit data/ directory.
The CLI should remain usable in a developer shell, a PR, or a separate metadata
factory without requiring a write token or pushing analysis commits.

## Current implementation

Example results live locally under examples/<name>/results/<mode>/. JSON and YAML
represent the same single run; JSON supports assertions and integrations, YAML is
for review. run.json records the tested revision, workflow SHA, run ID/attempt,
time and validation result. Reports carry source/ruleset digests, check IDs,
classification provenance and returned model answers. Temporary absolute paths
are normalized; raw source spans/expressions in the report remain visible.

Results are committed to the PR branch under each example’s results/ folder. CI uploads the folders as artifacts with 14-day retention
and summarizes them on the workflow run visible from the PR. Curated expected.json
files are tracked separately. They assert contracts (classification, ownership,
profile routing, specific check statuses), not exact model probabilities.

This deliberately keeps the latest example evidence beside its fixture for PR
review. It introduces generated diffs in source history; it is a provisional
choice, not a fleet results database. Publishing is followed by lightweight path/parent-evidence
verification without another publication, preventing feedback loops.
A fresh run replaces its local mode folder's known files; prior CI attempts have
separate artifact names. It does not rerun Jev to obtain a second output format.

## Alternatives to evaluate

| Storage | Suitable use | Main trade-off |
|---|---|---|
| Caller-selected local files / stdout (stdout not implemented yet) | Developer workflows, piping into other tools | Caller manages retention and sharing |
| GitHub Actions artifacts + summaries (current PR choice) | Per-PR evidence tied to a run and revision | Retention expiry; requires GitHub access while private |
| Small committed golden fixtures | Deterministic regression contracts | Review noise if used for nondeterministic raw model output |
| Separate metadata/results repository | Fleet catalogs and durable reviewed YAML history | Requires publishing permissions and ownership/versioning policy |
| Object storage keyed by source/ruleset/model/run | Immutable large-scale history and downstream queries | Requires access control, retention and indexing |
| SQLite or service database | Local history or interactive fleet queries | Schema migrations and operational coupling; poor fit for Git diffs |
| SARIF and GitHub code scanning | File/line findings in review UI | Classification inventories do not fit well; private availability depends on GitHub plan |

Recommendation for now: keep the CLI output-agnostic, example reports on their feature branch, PR evidence
also in Actions artifacts, and reviewed expectations separate from generated reports. Do not create a
fleet data/ catalog inside iaclens. A later metadata factory can consume its JSON
or YAML without changing the analysis engine.

Before making the repository public, review tracked fixtures and release history.
Private analysis reports from real users should stay in their repositories or
artifact stores; use synthetic/public examples in this repository. Keep credentials
out of reports, and treat Terraform expressions as potentially sensitive caller data.

Future decisions: retention horizon; run identity across model/ruleset revisions;
source-code inclusion/redaction; timestamp policy; atomic latest/history layout;
report schema migration; whether to add SARIF alongside full structured reports.
