# Terraform rulesets

The Git repository is the analysis root. Rules describe only Terraform source
and directory ownership. No pipeline, hosting-service, Git-history, provider
release feed, or DevOps-governance checks are included.

`default.yaml` is the version-controlled source of the default policy. The same
file is embedded in the binary so installed releases work from any directory.
There are no duplicate copies of its policy in Go.

Load order (whole-file replacement, not merging):

1. `--config /path/to/rules.yaml`, when supplied.
2. `.iaclens.yaml` at the analysed Git repository root.
3. The embedded `rules/default.yaml`.

Use `iaclens --print-default-config > .iaclens.yaml` to start customizing. Paths
in that file are relative to the analysed Git root. A relative `--config` file
path is relative to the command's working directory. YAML unknown fields,
duplicate keys, unknown engines/assertions/selectors, invalid thresholds,
unsupported versions and multiple YAML documents are errors.

## Repository, components, and supporting examples

Every directory containing selected `.tf`/`.tf.json` files is an analysis unit.
The default structure policy marks paths underneath an `examples` directory as
supporting examples. Each belongs to the nearest enclosing component with
Terraform files; if no such component exists, it belongs to the repository.
Explicit `example_owners` assignments support a shared top-level examples tree:

```yaml
structure:
  example_dirs: [examples, demos]
  example_owners:
    examples/network-basic: modules/network
    examples/storage-basic: modules/storage
```

These are exact paths to directories containing Terraform files. Owners must be
independent components, not other examples. Unknown paths or cycles through
example ownership are errors. Remove `demos`/`examples` from the directory list
if your repository uses those names for standalone components.

A module's example may classify as deployment and receive deployment checks.
It remains owned by that module. It does not turn the repository into a mixed
codebase. Independent components, including nested reusable modules, are counted
in the repository summary. Multiple modules -> module; multiple deployments ->
deployment; both -> mixed. Unresolved components keep classification incomplete;
if only one kind is known alongside unresolved components, the summary is unknown.
A known mixture remains mixed even if other components are unresolved. An
example-only repository has unknown component classification.

## Collection policy

- `include`: basename globs for Terraform files, using Go path.Match syntax
  (`*`, `?`, character classes). Not recursive `**`. Traversal itself is recursive.
- `exclude_dirs`: directory basenames to skip anywhere below the repository root.
- `exclude_paths`: exact root-relative files/directories; directory descendants
  are skipped. No glob syntax.
- `blocks`: which supported top-level HCL blocks to collect. Rules cannot select
  a block type disabled here. Full selected files remain model context even when
  a block type is omitted from the extracted inventory.

Only `.tf` and `.tf.json` can be selected. Symlink files/directories are not
followed. This version does not evaluate `.tfvars`, plans, state, `.tftest.hcl`,
Terraform Stacks, remote module downloads, provider schemas or runtime expressions.
HCL syntax parsing remains Go code; changing include/exclude or block selection
is policy configuration, not redefining Terraform's grammar.

Top-level block selectors: resource, data, ephemeral, action, variable, output,
module, provider, terraform, locals, check, import, moved, removed.
Nested selectors: terraform.backend, terraform.cloud,
terraform.required_providers, variable.validation, resource.lifecycle,
data.lifecycle, output.precondition, check.assert. Other nested source remains
available to Jev but is not normalized into selectable facts yet.

## Classification and profile selection

The configuration labels intended usage, not different Terraform languages:
`module` means a reusable component; `deployment` means a plan/apply entry point.
Technically Terraform calls both root/child modules. One can contain resources
and module calls in either case.

Precedence per analysis unit:

1. `--kind` override for the repository's `.` unit only, if it exists.
2. Exact `classification.overrides` path.
3. Configured structural signals (presence of selected blocks).
4. Jev question for an unresolved unit, in live mode only.
5. Unknown when unavailable, ambiguous, or below the configured thresholds.

Signals are an explicit policy choice: the defaults treat a backend/cloud block
as deployment intent. They are not proof that the author intended correct usage.
Override a known reusable module to detect a misplaced backend there, or remove
signals to let Jev interpret every unresolved component. Conflicting matching
signals produce unknown without asking Jev to silently resolve policy conflicts.

Example overrides:

```yaml
classification:
  overrides:
    modules/network: module
    environments/dev: deployment
    environments/prod: deployment
  # Retain signals, thresholds and question from the default file.
```

Jev classification is a choice with exactly module/deployment/unknown criteria.
Both chosen-option probability and confidence must meet the configured thresholds.
Default 0.8 thresholds are provisional, not a Terraform accuracy guarantee.
When classification is unknown, kind-specific checks are unchecked rather than
running both profiles. Checks explicitly including `unknown` may run.

## Custom checks

Each check has a unique `id`, exactly one `engine`, `applies_to`, `severity`
(info/warning/error), a display `message`, and optional `enabled` (default true).
Severity is policy metadata; it does not change the predicate. Successful CLI
execution returns 0 even when checks fail; this version is a reporting tool.

Go checks specify a block selector, optional attribute, and an assertion:

| Assertion | Meaning |
|---|---|
| exists | At least one selected block, or at least one with the attribute |
| absent | No selected blocks, or none containing the attribute |
| all_have_attribute | Every selected block contains the attribute |
| all_nonempty | Every selected block has a statically evaluable nonempty string attribute |

The `all_*` assertions return not_applicable for no selected blocks. Expression
values unavailable without runtime context produce unchecked for all_nonempty
unless a definite failure is also found. Presence does not evaluate an expression.
For example, an exists rule on required_version establishes presence, not that
the version range is sensible. Go checks do not use Jev question fields.

```yaml
- id: validation-required-for-inputs
  engine: go
  applies_to: [module]
  severity: warning
  block: variable.validation
  assert: exists
  message: This policy requires at least one input validation block.
```

This example intentionally means “at least one validation block,” not “every input
is sufficiently validated.” Use a different predicate implementation for a new
exact semantic operation rather than pretending a broad assertion proves it.

Jev checks specify `question`, `min_confidence`, and `min_probability`. Their
choice criteria must be exactly pass/fail/unknown. The default file contains a
disabled example. Enabling it or adding another choice check requires no Go edit.
They cannot also specify a Go predicate. Only enabled, applicable Jev checks are
batched after classification. Results below either threshold are unchecked.
Descriptions and remediation prose come from configuration, not generated text.

Arbitrary shell code, template execution, networking actions or custom Go plugins
cannot be embedded in a ruleset. Adding a new assertion primitive requires code;
changing profile membership, selectors, thresholds, prompts, criteria descriptions,
severity, scan filters and ownership does not.

## Output and duplicate-work contract

Schema version 2 identifies the repository, collected units, classification per
unit, ownership, applicable checks, actual Jev calls and a repository summary.
Source and ruleset digests identify inputs. Jev responses retain probabilities,
confidence and returned model. Engine/provenance is recorded per result.

Go parses each file once. A conclusive override/signal suppresses the Jev
classification call. Each enabled semantic check is asked once per applicable
unit per run; the checks use source once in their shared state, not source plus a
duplicate AST. Classification and checks can require two sequential calls because
the latter depend on the former. No persistent cross-run response cache exists yet.

Prepare mode writes a JSON plan of currently resolvable requests. For unknown
units it prepares classification only; kind-specific semantic questions can be
planned only after the classification returns. It does not pretend all profiles
can be evaluated independently in one request.
