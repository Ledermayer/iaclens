# Release Tasks

Owner: Ledermayer.

- [x] Merge the three-fix release-readiness PR only after its required checks pass.
- [x] After Ledermayer merges it, publish a new immutable `v0.1.0-rc.3` from the
  merged commit; do not move existing tags or replace RC1/RC2 assets.
- [x] Verify RC3 downloads, checksums, notices, offline examples, and AVM smoke test.
- [x] Complete public installation/security guidance and the consumer report contract.
- [x] Merge the public documentation PR after its required checks pass.
- [x] Merge the archive and reporting-policy PR after its required checks pass.
- [x] Make the repository public and tag stable 0.1.0 from the merged main commit.
- [ ] Enable private vulnerability reporting now that the repository is public.
  GitHub does not offer that setting before publication.

RC1–RC3 remain immutable prereleases. Publishing 0.1.0 does not replace their assets.