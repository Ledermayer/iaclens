# Contributing

All changes to main must go through a pull request, including maintainer changes.
Create a branch, run `go test -race ./...` and `go vet ./...`, and open a PR. Merge
only after required Validation gate passes and conversations are resolved.
Use squash merges with a descriptive Conventional Commit title.

The repository currently has one maintainer, so branch rules require a PR and
passing checks but not an approval the author cannot give themself. Add an
independent required review when another maintainer is available. CODEOWNERS
identifies the current reviewer; no administrator bypass is configured.

Live examples use a metered LLM credential from the live-examples environment.
Same-repository branches are a trust boundary: only trusted maintainers should
have write access. Never execute fork PR code with pull_request_target or expose
secrets to unreviewed code. Fork and Dependabot PRs run offline checks; their merge
gate enforces the selected CI/offline validation route. Live assessments are advisory and may
be skipped or fail without blocking a merge. Maintainers can review/promote a
fork onto a same-repository branch when a live assessment is useful.

Read [examples/README.md](examples/README.md) for the runner and
[docs/result-storage.md](docs/result-storage.md) for result ownership.

Rule and classification changes must include meaningful example expectations.
Do not loosen a threshold simply to make a live test green. Investigate raw
answers and retain uncertainty when evidence is insufficient. Live model results
may drift even with a pinned model; rerun only after examining a failure.

Default rules are versioned in rules/default.yaml and embedded in release binaries.
Generated examples/*/results files are committed separately by Actions; expected.json files are
reviewed inputs, not snapshots of every probability from the last run.

See [validation routing](docs/validation-routing.md) for change selection and results-commit verification.
