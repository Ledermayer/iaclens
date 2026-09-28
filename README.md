# iaclens

A Go CLI for analysing Terraform code within a Git repository. A repository can
contain reusable modules, infrastructure deployments, or both. Supporting examples
remain attached to their owning module/component even when they are deployment code.

Go parses HCL and evaluates exact checks; Jev classifies unresolved components and
runs explicitly enabled semantic checks. Repo-local YAML defines collection policy,
component/example ownership, classification and profile-specific checks.

**Scope:** Terraform code only. Git is used to find the analysis root. Git history,
remote hosting settings, Azure DevOps, pipeline compliance, provider release feeds,
and DevOps governance are outside scope.

## Build and run

Requires Go 1.27 or later and Git. From this repository:

```sh
go build -o work/iaclens ./cmd/iaclens
go test ./...
git clone --depth 1 https://github.com/Azure/terraform-azurerm-avm-res-resources-resourcegroup.git work/resourcegroup

# Repository-root analysis without API calls; known root kind supplied explicitly
./work/iaclens --source work/resourcegroup --kind module \
  --format json --out work/resourcegroup.json

# Automatically classify unresolved components using Jev
# Export LLM_GATEWAY_API_KEY in your shell before this command.
./work/iaclens --source work/resourcegroup --mode jev --provider llmgateway \
  --format json --out work/resourcegroup-live.json

# Inspect proposed requests without calling Jev
./work/iaclens --source work/resourcegroup --mode prepare \
  --request-out work/requests.json --out work/resourcegroup.yaml
```

`--source` may point anywhere inside the target worktree; analysis always resolves
to that repository's root. A repository without root .tf files is supported.
Directories are classified independently; there is no assumption that one
repository equals one module. `--kind` affects only the root Terraform directory;
use per-path overrides for multi-component repositories.

The model defaults to `jev-1.13.0`; override with `--model`. Direct TypeSafe access
uses `--provider typesafe` and `TYPESAFE_API_KEY`. Credentials are read only from
environment variables. Live requests contain selected Terraform source for the
target unit. No model calls are made for conclusive configured classifications,
or for disabled/inapplicable semantic checks.

## Customize policy without recompiling

```sh
./work/iaclens --print-default-config > work/company-rules.yaml
# Edit that YAML, then:
./work/iaclens --source work/resourcegroup --config work/company-rules.yaml \
  --format json --out work/company-report.json
```

Alternatively put `.iaclens.yaml` in the analysed repository root. Explicit
`--config` takes precedence; otherwise the local file overrides embedded defaults.
Files replace the entire ruleset; implicit merges are not performed.

See [rules/default.yaml](rules/default.yaml) and the [ruleset reference](rules/README.md)
for scan filters, ownership, classification, assertions, semantic checks and examples.
The shipped defaults are an initial policy, not a universal Terraform standard.
In particular backend/cloud signals are configurable intent assumptions, and the
provider-declaration warning can be disabled for provider-free code.

## Results and limits

Schema version 2 records repository identity, a source digest, ruleset identity and
digest, extracted units, per-unit classifications/checks and actual Jev responses.
Examples have explicit owners and retain their own checks; they do not inflate
independent component counts or change the repository's kind. The summary includes
module/deployment/unknown component counts and classification completeness.

`--format json` writes a complete JSON report (replacing the output file).
Default YAML output replaces only `# BEGIN IACLENS` / `# END IACLENS`; content
outside those markers is preserved. Files are written via a temporary file and
rename. Errors do not overwrite an existing report. Check failures are reported
in the output but do not currently set a failing CLI exit status.

Unknown intended usage stays unknown; code-specific checks remain unchecked until
classification is resolved. Use overrides for known components. Model thresholds
are configurable and provisional. “No findings” is not a Terraform plan, deployment
validation, or security certification.

The parser retains expressions without evaluating runtime values or downloading
remote modules. It supports .tf and .tf.json; tfvars, Terraform Stacks/test files,
plans and state are not yet analysed. Selected nested facts are normalized; see
the ruleset reference for supported selectors. Provider-specific requirements and
version-range evaluation are not implemented in this first configurable engine.
Jev calls have a 90-second timeout and a 100,000-byte guard, not a token estimator.
A single CLI run can contain several scoped requests, and responses are not cached
across runs yet.

The earlier v0.1.0-rc.1 release demonstrated the live AVM Resource Group integration
through LLM Gateway (18 files, 5 directories). Its schema-v1 domain/role questions
are superseded here by configurable component classification; old releases remain
unchanged.

## CI and semantic-versioned releases

`CI` runs formatting, module verification, tests with the race detector, vet, and
builds on Linux, macOS, and Windows. A separate job verifies all six release
packages without credentials or model calls.

`Release` runs when a `v*` tag is pushed. Version selection is explicit:

- `v0.1.0`: first POC release.
- `v0.1.1`: backward-compatible bug fixes.
- `v0.2.0`: new capabilities or breaking changes while pre-1.0.
- `v1.0.0`: first stable compatibility contract; thereafter breaking changes bump
  the major version, compatible features the minor, and fixes the patch.
- `v0.2.0-rc.1`: prerelease, automatically marked as such on GitHub.

Tags must follow SemVer, prefixed with `v`; build metadata (`+...`) is not used.
There is no automatic version bump based on commits. Commit and push the CLI and
workflow files, wait for CI, then tag the chosen commit on `main`:

```sh
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin v0.1.0
```

The workflow verifies the tag is reachable from `main`, repeats tests, builds
Linux/macOS/Windows archives for amd64/arm64 with CGO disabled, and includes
README, LICENSE, and SHA-256 checksums. `iaclens --version` reports the tag,
commit, and source commit date. A separate publishing job uses `GITHUB_TOKEN`
with `contents: write`; no personal token or LLM credentials are required.

Assets are uploaded to a draft before publication. Use the workflow's **Run
workflow** form with an existing tag to retry an interrupted draft release.
Published versions cannot be overwritten by this workflow: issue a new version.
Do not move a release tag. Prereleases never become the latest stable release.

Releases inherit repository visibility: while this repository is private,
authorized users can download with `gh release download v0.1.0 --repo
Ledermayer/iaclens`. Making the repository public later requires no workflow
change and makes its existing published releases available publicly too. These
workflows never change repository visibility or publish to a package registry.
GitHub Actions must be enabled and repository/organization policy must permit
the publishing job's token permissions. Hosted runner usage is subject to the
account's private-repository Actions allowance.

To inspect the release bundle without publishing:

```sh
scripts/release.sh v0.1.0-rc.1
(cd dist && shasum -a 256 --check checksums.txt)
```

The output directory must not already exist; use `RELEASE_DIR=work/another-build`
for another local run. Binaries are unsigned in this first release pipeline;
macOS/Windows signing and notarization are not configured.

## Example suite and PR evidence

Six self-contained repository archetypes and per-example rulesets live under
[examples/](examples/README.md). The PR Examples workflow runs each offline and
against live Jev, verifies reviewed expectations, and uploads JSON/YAML reports
and run metadata. Local generated results stay inside each example's ignored
results/ folder. See the [result-storage decision](docs/result-storage.md) for
alternatives to making the CLI repository a metadata database.

All changes to main require a PR and the required checks; see CONTRIBUTING.md.
