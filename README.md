# iaclens

A Go CLI proof of concept for extracting Terraform source facts and classifying a
module with TypeSafe Jev. One invocation scans a local repository, makes one
optional System One API request, and writes YAML.

## Build

Requires Go 1.27 or later (the current module's toolchain requirement).

```sh
go build -o work/iaclens ./cmd/iaclens
go test ./...
```

## Run on an Azure Verified Module

```sh
git clone --depth 1 https://github.com/Azure/terraform-azurerm-avm-res-resources-resourcegroup.git work/resourcegroup

# Local extraction, no API request
./work/iaclens --source work/resourcegroup --out work/resourcegroup.yaml

# Inspect the complete request without sending it
./work/iaclens --source work/resourcegroup --mode prepare \
  --request-out work/request.json --out work/resourcegroup.yaml

# Export LLM_GATEWAY_API_KEY in your shell, then run a live classification
./work/iaclens --source work/resourcegroup --mode jev --provider llmgateway \
  --out work/resourcegroup.yaml

# Alternatively export TYPESAFE_API_KEY for direct TypeSafe access
./work/iaclens --source work/resourcegroup --mode jev --provider typesafe \
  --out work/resourcegroup.yaml
```

The model defaults to `jev-1.13.0`. Override with `--model`. Credentials are read
from environment variables and are never written into generated requests or YAML.
Live mode sends the discovered Terraform source and extracted inventory to the
selected provider. Prepare mode writes that same source locally into its request.

## Output and boundaries

The `iaclens` YAML object contains a source digest, module directories, symbols
with file/line provenance and verbatim attribute expressions, and `analysis_jev`.
Each Terraform directory remains separate, including examples and child modules.
Resources, data, ephemeral resources, actions, module calls, providers, variables,
outputs, locals, checks, and Terraform settings are recognized. Required provider
expressions are included. Both `.tf` and `.tf.json` are supported. Hidden
directories, vendor directories, and symlink files are skipped.

HCL is parsed, not evaluated: runtime values, remote child-module contents and
expanded dynamic blocks are not resolved. Nested block contents remain in the
source sent to Jev but are not fully normalized in the YAML symbol inventory.
This first POC does not implement the predecessor's complete metadata schema,
resource indexes, Git/DevOps collection, or conference factory.

Jev answers two bounded questions: primary domain and architectural role of the
root module. The CLI records probabilities, confidence, model, token usage, and
question version. Successful responses have `review_required` status; no
confidence threshold has been calibrated on Terraform yet. An offline result is
explicitly `not_run`, and a prepared request is `prepared`.

Output is atomically replaced. On subsequent runs, only the exact
`# BEGIN IACLENS` / `# END IACLENS` region is replaced. Existing manual enrichment
and other tool-owned regions outside it are preserved. Invalid markers or HCL
cause an error; API failures do not overwrite the existing output. This POC uses
its own namespace so it can coexist with existing module stubs without claiming
full compatibility with their schema.

The POC rejects API request bodies over 100,000 bytes; this is a conservative
local guard, not a tokenizer or a guarantee of fitting the model's token limits.
Oversized modules need a future batching strategy. HTTP requests have a 90-second
timeout; failed calls return an error without automatic retries.

## Verified live run

On 2026-09-27 the compiled CLI successfully ran through LLM Gateway against
`Azure/terraform-azurerm-avm-res-resources-resourcegroup` at commit
`de43ed490b29e950baea4e826dca861413f93c6e`:

- 18 Terraform files across 5 module directories.
- Returned model: `typesafe/jev-1.13.0`.
- Domain: `management`, confidence 0.99.
- Role: `resource`, confidence 0.87 (chosen-option probability 0.89).
- Usage: 16,370 input tokens and 147 output tokens.

This verifies integration for one module; it does not establish classification
accuracy across the AVM catalog. Local run artifacts and the binary live under
ignored `work/`.

API references: [TypeSafe](https://docs.typesafe.ai/api),
[LLM Gateway System One](https://docs.llmgateway.io/features/system-one).

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
