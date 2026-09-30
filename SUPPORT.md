# Support

**Maintainer:** [Ledermayer](https://github.com/Ledermayer).

IaCLens is an early-stage, community-maintained Terraform analysis CLI. Support
and maintenance are best-effort; there is no SLA or commercial support commitment.
It is not an official Microsoft/Azure Verified Modules certification tool, and
does not claim employer or vendor endorsement.

- Read the [installation guide](docs/installation.md), [rules reference](rules/README.md), [report contract](docs/report-contract.md), and [examples](examples/README.md) first.
- Report ordinary bugs or request features through [GitHub Issues](https://github.com/Ledermayer/iaclens/issues/new/choose).
- Ask usage questions in an issue with the minimal context needed to reproduce the problem.
- Follow [Security](SECURITY.md) for vulnerability reports; do not disclose them in ordinary issues.
- Follow [Contributing](CONTRIBUTING.md) for code, policy, documentation or test contributions.

Useful bug reports include `iaclens --version`, OS/architecture, a redacted command,
expected/actual behavior, and a small synthetic Git-repository reproduction.
Remove API keys, private URLs, customer identifiers, source expressions/literals,
local user paths and model request contents. A failed check may be expected policy
behavior, not a CLI defect; include the effective policy identity and check IDs.

Please report against the latest maintained release when possible. Older RC
reports use different contracts; RC1 uses schema 1 and is not interchangeable
with schema 2. Private repositories/releases require access until the maintainer
explicitly makes the project public.