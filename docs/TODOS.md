# Release Tasks

Owner: Ledermayer.

- [x] Merge the three-fix release-readiness PR only after its required checks pass.
- [x] After Ledermayer merges it, publish a new immutable `v0.1.0-rc.3` from the
  merged commit; do not move existing tags or replace RC1/RC2 assets.
- [x] Verify RC3 downloads, checksums, notices, offline examples, and AVM smoke test.
- [x] Complete public installation/security guidance and the consumer report contract.
- [ ] Merge this public-release-readiness PR after its required checks pass.
- [ ] Obtain explicit approval for repository visibility changes and stable 0.1.0.

RC3 remains a private-repository prerelease. Neither merging this PR nor the
already-published RC3 authorizes making the repository public.