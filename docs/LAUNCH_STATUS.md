# Product launch checkpoint

**v0.2.0 is not published.** The existing public v0.1.2 remains unchanged and
contains Full clients/images only, not UltraLite. Planned v0.2.0 download links
must not be presented as working downloads before publication verification.

The owner authorized integration and publication conditional on successful CI
and security gates, excluding every production operation.

- PR #2 integrated main's ARM64 patch with eight conflicts resolved, preserving
  security changes. Integrated head `a211b973bddd686e1f87253fa6abd498395d8570`
  passed CI and GitGuardian; merged as `c6a78e5412bc7ddc569382bcd7299a5b80511216`.
  No intermediate v0.1.3 release is needed; audit hardening belongs in v0.2.0.
- PR #8 retargeted to main. Shared-core implementations were retained while
  merging the audit/main histories; no history rewrite or duplicate cherry-picks.
- Versions prepared: Full 0.2.0, Mini/UltraLite 0.1.0. Unified release automation
  stages nine native binaries, source archive, Mini installer, checksum manifest,
  release notes and platform support. Docker publication stages separate Full
  and glibc Mini multi-platform candidates, verifies them, adds both digest files,
  verifies download completeness, then publishes and promotes Full latest only.
- Draft resumption compares assets and refuses differing bytes; published
  releases/version tags are immutable. The Release guard requires a positive
  GitGuardian result on the reviewed merged PR head, not merely green CI.
- README now starts with UltraLite and explains complete Windows/Linux paths,
  MCP registration, instructions, capture limitations, storage and recovery.
  GETTING_STARTED.md, README_AR.md, PLATFORMS.md and UPGRADE.md added.
- Mini installer now distinguishes ordinary Docker's loopback port publication
  from ThothDock's shared-network direct loopback binding. Test-only name/volume
  overrides allow isolated installer verification.

## Blocking owner action

GitGuardian incident **38171534** remains an affected PR #8 merge/publication
gate until its check actually succeeds. Reported occurrence is a synthetic
password refusal fixture in earlier commit 1aeacc34, not a real service login.
Current source constructs the fixture at runtime and keeps refusal/sanitization
assertions. No scanner was disabled and no historical commits rewritten.

https://dashboard.gitguardian.com/workspace/668579/incidents/38171534?occurrence=303908630

Owner: inspect and mark false positive with the test-fixture reason if satisfied;
recheck GitGuardian's actual PR conclusion afterward. Do not waive the check.

## Remaining executable gates

Fresh final branch CI, Mini image security scan and native beginner quickstart
tests are required after packaging changes. Native model behavior is separate
from the deterministic installer/MCP tests. Linux/Windows instructions must be
verified against the release artifacts again after publication. A final Full
populated-database upgrade/dashboard rehearsal remains required at the integrated
revision. Existing physical Samsung evidence is retained; the final digest retest
is blocked if ADB remains empty. Android is a clearly labelled preview with no
endurance or battery claim. macOS and Windows ARM64 remain build-only as noted.

After the gate clears: inspect PR #8's exact checks, ready/merge its tested head
using a merge commit, require main CI, tag v0.2.0 on that merge, follow Release
and Docker publication, verify all public assets/digests and actual installations,
then remove the pending-publication notices and record the final result.
If a required check fails, diagnose/fix and rerun only the affected work.

Production containers, PostgreSQL volume, credentials/configuration and cold
backup remain protected; no production start, migration or upgrade is authorized.
