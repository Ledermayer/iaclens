# Changelog

Release tags and assets are immutable. This file summarizes user-facing behavior;
the release page identifies each exact commit. Dates below are publication dates.

## 0.1.0 - 2026-09-30

First non-prerelease release. This remains pre-1.0 software, not a promise of a
mature stable API or security certification.

- Document verified binary installation for all six platforms and safe offline use.
- Define schema-v2 report fields, compatibility, coverage and output/error semantics.
- Add security-reporting/data-handling guidance, issue forms and maintainer support expectations.
- Include public docs and the report schema in release archives, with contract tests in CI.
- Retain the RC3 parser, configurable classification/check model and privacy limits.

## 0.1.0-rc.3 - 2026-09-30

- Upgrade `golang.org/x/text` to v0.39.0 for GO-2026-5970 and add a blocking vulnerability check to full CI/release validation.
- Prevent generated-report changes mixed with documentation from bypassing example validation.
- Include dependency and Go redistribution notices and exact-version source links in every platform archive.

RC3 supersedes RC1/RC2. Older assets remain unchanged; use RC3 or a later release
instead of assuming older downloads contain the dependency fix or notices.

## 0.1.0-rc.2 - 2026-09-28

- Introduce configurable repository/component classification and profile-specific checks.
- Publish schema 2 with explicit example ownership, policy/source digests and actual model evidence.
- Add six synthetic repository archetypes and required offline/advisory live validation.

## 0.1.0-rc.1 - 2026-09-27

- Initial prerelease with Terraform parsing and live AVM Resource Group integration.
- Schema-1 domain/role assessments are superseded by schema 2; reports require an explicit migration rather than being read as the newer format.