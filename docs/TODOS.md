# Release Tasks

Owner: Ledermayer.

- [ ] Merge the three-fix release-readiness PR only after its required checks pass.
- [ ] After Ledermayer merges it, publish a new immutable `v0.1.0-rc.3` from the
  merged commit; do not move existing tags or replace RC1/RC2 assets.
- [ ] Verify RC3 downloads, checksums, notices, offline examples, and AVM smoke test.
- [ ] Complete public installation/security guidance and the consumer report contract.
- [ ] Obtain explicit approval for repository visibility changes and stable 0.1.0.

RC3 remains a private-repository prerelease. Neither merging this PR nor creating
RC3 authorizes making the repository public.