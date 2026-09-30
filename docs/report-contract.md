# Report Contract: Schema Version 2

This contract describes IaCLens's current output, not planned features. The
[JSON Schema](../schemas/report-v2.schema.json) validates JSON reports and the
extracted tool-owned YAML payload. CI checks existing synthetic example reports
and fresh offline/prepare CLI output against it. It does not validate the separate
prepare request-plan document.

## Versioning And Consumption

CLI version, report schema version and ruleset format version are separate:

- `iaclens --version` identifies the executable, commit and source commit date.
- `iaclens.schema_version` is **2** for this report contract.
- `iaclens.ruleset.version` is **1**, the ruleset format version, not a policy revision counter.
- `ruleset.digest` identifies the exact effective rules bytes. Keep it when comparing assessments.

Consumers must reject unsupported schema versions, validate required fields and
enums, and tolerate unknown additional fields. Compatible optional fields may be
added without changing schema 2. Removing/renaming required fields, changing their
meaning/type, or adding incompatible enum values requires a new report schema.
Before CLI 1.0, breaking changes require a minor CLI release and migration notes;
patch releases preserve this consumer contract. Pin CLI, schema and policy for
automation. Do not parse human messages, YAML formatting, object key order or
array position as stable identifiers.

## Envelope And Identity

| Field under `iaclens` | Meaning |
| --- | --- |
| `schema_version` | Integer `2`. |
| `repository.root` | Local resolved Git worktree path. Fixture reports may normalize it to `source`. Not a portable repository ID. |
| `repository.name` | Worktree directory basename; not necessarily a GitHub repository name. |
| `source_digest` | Lowercase SHA-256 of Go JSON serialization of the scanned inventory, including complete collected file text. Not a Git commit or a semantic Terraform hash. |
| `ruleset.name`, `version`, `digest` | Effective policy name, format version and lowercase SHA-256 of its exact bytes. Whitespace/comments change the digest. |
| `units` | Extracted Terraform units and symbols. |
| `analysis` | Classifications, ownership, checks, completed model evaluations and summary. |

The report does **not** currently include source commit SHA, remote URL, CLI build
version or run timestamp. Capture those separately when provenance matters. The
build date printed by `--version` is the source commit date, not the scan time.
Source and ruleset digests can change even when a visible result looks equivalent.
Reproduce the source-digest algorithm only with compatible scanner behavior;
prefer treating it as an opaque input identifier.

## Units And Symbols

Every directory with selected `.tf`/`.tf.json` files is a unit. Join inventory
`units` to `analysis.units` by `path`, not by order. Paths are Git-root-relative,
with `/` separators and `.` for the root. The CLI scans the containing Git root
even when `--source` points into a subdirectory. Duplicate unit paths and missing
cross-array matches are invalid; consumers should check these cross-record
invariants in addition to JSON Schema validation.

Inventory units contain `path` and `symbols`. A symbol has `kind`, `file`, and a
one-based `line`, plus optional `type`, `name`, `attributes`, `literals`, and
recursive `children`. `attributes` retains source expressions; `literals`
contains only strings conclusively evaluated without runtime context. Neither is
a full Terraform evaluation. Unknown/uncollected facts are not evidence of absence.
Empty optional symbol maps/arrays are omitted. `symbols` itself is an array,
including `[]` when no selected symbols are present.

Analysis units contain:

| Field | Values / rules |
| --- | --- |
| `path` | Matches an inventory unit. |
| `role` | `component` or `example`; separate from execution kind. |
| `owner` | Present for examples: path of owning component or `.` for repository ownership. |
| `owner_scope` | Present for examples: `component` or `repository`. |
| `classification.kind` | `module`, `deployment`, `unknown`. |
| `classification.method` | `override`, `go`, `conflict`, `unavailable`, `prepared`, or `jev`. |
| `classification.reason` | Human explanation, not a machine-stable code. |
| `classification.answer` | Optional actual model answer, including below-threshold answers. |
| `checks` | Array of applicable/withheld check results; may be empty. |

An example can classify as a deployment while remaining owned by a reusable
module. Examples do not inflate independent component counts. `--kind` overrides
only the root Terraform unit; nested classifications do not inherit it. A
rootless repository requires `auto` or per-path policy overrides.

## Checks And Coverage

Each check has `id`, `engine` (`go` or `jev`), `status`, `severity`
(`info`, `warning`, `error`), and `message`. Optional `evidence` contains readable
references such as `main.tf:12`; its text format is not a structured location API.
An optional `answer` is the actual semantic response. Check IDs are stable within
the selected ruleset, not universal across user policies.

| Status | Meaning |
| --- | --- |
| `pass` | The selected predicate or accepted model decision passed. |
| `fail` | It failed under this policy. Analysis itself may still have succeeded. |
| `unchecked` | Classification, static value or accepted model evidence was insufficient, or no model call was made. |
| `not_applicable` | A deterministic all-items assertion found no selected blocks. |

Disabled checks are omitted. Checks for a different conclusively known kind are
also omitted. If kind is unresolved, kind-specific checks can appear unchecked.
Therefore absent checks, empty arrays, unknown classification and unchecked
results must never be synthesized into passes. Severity does not change CLI exit
status. The CLI does not emit a universal compliance score.

## Summary And Model Evidence

`analysis.component_counts` includes module/deployment/unknown counts of
independent components. `example_count` is separate. `classification_complete`
requires at least one known component kind and no unknown components. An
example-only repository remains incomplete.

`codebase_kind` is module/deployment when all components resolve to that one
kind, mixed when both known kinds exist, and otherwise unknown. A known mixture
can remain mixed even when additional components are unresolved. `kind_scope`
explains that examples are excluded; treat it as display text.

`jev_calls` is omitted when no calls completed. Entries record unit `path`, stage
(`classification` or `checks`) and `result` with model, answers and usage. Usage
is a provider-supplied integer map and may be null; no billing calculation is
guaranteed. HTTP retry attempts are not separate decision records.

An answer has `type: choice`, `choice`, a probability map and confidence in
`[0,1]`. Classification choices are module/deployment/unknown; check choices are
pass/fail/unknown. The client verifies expected answer IDs/options and approximate
probability normalization. An unknown or below-threshold answer does not become
a passing check. Typed probabilities are not proof of correctness. Human messages
come from policy/code, not generated remediation prose.

## Files, Errors And Process Status

- `--format json --out report.json` writes the report to that file. Stdout is a human progress summary; errors are plain stderr text, not JSON.
- YAML is the default. Only text between `# BEGIN IACLENS` and `# END IACLENS` is replaced. Surrounding text is preserved without being validated as YAML. Parse the owned payload, not necessarily the entire file.
- Successful writes use a same-directory temporary file and rename. This is not a multi-file transaction or a general durability/concurrent-writer guarantee.
- Operational errors return exit **1**; flag parsing errors can return **2**. Successful analysis returns **0**, including reports containing failing checks. Help/version/config output can also exit 0 without a report.
- An error can leave the previous report untouched. Use a fresh output path for every attempt and consume it only after successful exit. Do not use file existence as proof that the latest run succeeded.
- `prepare` writes its request plan before writing the report. If the latter fails, the request plan may already exist. Keep these artifacts private; they include source-bearing request state.

For CI gating: require successful process exit, validate schema and expected
provenance, require sufficient classification/check coverage, then apply an
explicit policy to the check statuses/severities. Do not gate on exit 0 alone.

## Limits And Examples

No Terraform execution, provider-schema fetching, remote-module evaluation,
state/plan analysis or persistent model-response cache occurs. Model requests
have a 100,000-byte guard and a 90-second HTTP timeout per attempt. Only HTTP 503
is retried, up to three attempts with one- and two-second delays. One run can
contain several sequential requests; there is no 90-second whole-run deadline.
A live service failure aborts report generation rather than publishing a partial
successful report. Use separately keyed offline output when you need a baseline.

The [example catalog](../examples/README.md) describes six synthetic repositories.
The [custom-policy offline report](https://github.com/Ledermayer/iaclens/blob/main/examples/custom-policy/results/offline/report.json)
demonstrates a deliberate policy failure; the
[module-library report](https://github.com/Ledermayer/iaclens/blob/main/examples/module-library/results/offline/report.json)
demonstrates multiple components and owned examples. For a pinned consumer, use
reports/schema from the same release tag rather than following `main`.

See [rules](../rules/README.md) for effective policy semantics and rule
authoring, including how repository-local configuration replaces the embedded
default policy and how example ownership is resolved.
[Security](../SECURITY.md) before sharing reports or enabling live mode.