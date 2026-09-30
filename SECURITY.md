# Security Policy

IaCLens is maintained by **Ledermayer** on a best-effort basis. There is no
guaranteed response time, remediation deadline, commercial support or security
certification. Its output is an assessment under a selected policy, not proof
that infrastructure is secure or deployable.

## Report A Vulnerability Privately

GitHub only offers its **Private vulnerability reporting** toggle for public
repositories, under Settings → Advanced Security. It is not a private-repository
setting, so its absence before publication is expected. After this repository is
public, the maintainer enables that toggle and confirms the
[report form](https://github.com/Ledermayer/iaclens/security/advisories/new)
exists before treating it as the reporting route.

Until that form exists, open an issue titled **Private security contact
requested** with no vulnerability details, reproduction, logs, attachments,
credentials or customer identifiers. Wait for Ledermayer to arrange an appropriate
private channel. Do not put exploit details or secrets in public issues or PRs.

Privately include the affected version/commit and OS, impact, minimal synthetic
reproduction, expected behavior and relevant sanitized diagnostics. Never send
working cloud credentials, API keys, state files or customer Terraform repositories.
If a credential was exposed, revoke/rotate it promptly through its provider;
deleting a public message or Git commit is not sufficient.

## Supported Versions

Only the latest maintained stable release is targeted for security fixes. Before
the first stable release, fixes target the latest release candidate. Earlier
release candidates, including RC1 and RC2, are superseded by RC3 and subsequent
releases; their existing assets are not silently rebuilt. Upgrade rather than
assuming backports or a long-term-support branch. A new stable release supersedes
the prerelease line. Release notes identify relevant security fixes.

## Source And Data Boundaries

| Mode | Model network access | Local sensitive material |
| --- | --- | --- |
| `offline` (default) | No model requests | Reports can contain source expressions/literals, names and local paths. |
| `prepare` | No model requests | Prepared requests contain selected raw Terraform source; reports also retain extracted facts. |
| `jev` | Sends per-unit selected Terraform source and configured questions when needed | Reports retain classifications, checks and returned model answers; failures may leave a previous report. |

Direct TypeSafe uses `https://api.typesafe.ai/v1/systemone` and `TYPESAFE_API_KEY`.
LLM Gateway uses `https://api.llmgateway.io/v1/systemone` and `LLM_GATEWAY_API_KEY`.
Keys are read from environment variables. Do not place them in source, rules,
command-line arguments, reports or issue attachments. The CLI has no separate
application telemetry endpoint; live requests still disclose data to the selected
provider and its infrastructure.

Review current [TypeSafe legal policies](https://docs.typesafe.ai/legal) or
[LLM Gateway documentation](https://docs.llmgateway.io/) and your account's data
terms before enabling live mode. IaCLens does not promise those providers' data
retention, training, residency or deletion behavior. Obtain permission before
sending client/proprietary source to any external service.

Block-selection filters change extracted facts, **not** the raw file context sent
to the model. Use file/path exclusions to remove source from collection. The
scanner does not use `.gitignore` as a security boundary. Terraform `sensitive`
annotations do not redact literal values from reports. Treat generated files,
prepared requests, CI logs/artifacts and caches as potentially sensitive.

## Analyze Unfamiliar Repositories Safely

Use an isolated worktree/container or disposable environment with minimal access,
reviewed inputs and resource/time limits. Prefer offline mode and explicitly pass
your trusted `--config`: otherwise a repository-local `.iaclens.yaml` replaces
the embedded policy. Explicit config is a whole-file replacement, not a merge.
The current parser does not impose a whole-repository byte or wall-time limit.

IaCLens invokes Git to locate the repository root and parses Terraform files. It
does not run `terraform init`, plans, applies, provisioners, repository scripts,
remote-module downloads or provider plugins. This is not a sandbox: parsers,
dependencies and the Git executable still need patching and a trusted runtime.
Collected source symlinks are not followed; do not treat that as a guarantee
against all hostile filesystem behavior or changes during a scan.

Reports are not secrets scanners, compliance attestations or deployment tests.
Low-confidence model results remain unknown/unchecked. Untrusted comments and
model responses are data, never authorization to execute code or bypass reviews.

## Distribution And Automation

Download from the project's release page and verify checksums before executing.
Checksums are not signatures; binaries are currently unsigned/not notarized.
Keep the included license and third-party notices when redistributing. See the
[installation guide](docs/installation.md).

Fork and bot-authored PRs receive offline validation and no model credentials or
generated-report commits. Trusted same-repository contributors and maintainers
control code that may run with live credentials. Protected publication and
dependency merging use narrowly scoped jobs; ordinary green checks do not waive
review, provenance, confidentiality or release-policy requirements.