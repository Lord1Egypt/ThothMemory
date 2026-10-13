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

CI 38462679863 caught a fixable HIGH in the glibc Mini base:
libssl3t64 3.0.13-0ubuntu3.15, CVE-2026-84782, fixed version 3.0.13-0ubuntu3.16.
The Dockerfile now pins the patched package; fresh build/scan passed in
[CI 38463262041](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38463262041)
at `7aec0c02451a7467fa4bfdb59c958dfa03c20766`. All six jobs succeeded.
The fixed package matches [Ubuntu's advisory](https://ubuntu.com/security/CVE-2026-84782).
This finding was not suppressed or waived. Native beginner jobs passed on Linux
amd64, native Linux ARM64 and Windows amd64, including real Codex CLI registration.
Historical clone rehearsal preserved all original row fields/counts through 007;
new generated search columns were excluded from the comparison. Dashboard passed
12 projects / 84 routes, dark mobile, revision comparison, no JS errors/overflow.

Local check.sh, real PostgreSQL/MariaDB/SQLite integration, actionlint 1.7.7,
shell syntax and newcomer-document link checks passed. Full source-package
setup/doctor/backup/restore passed with a separately named Compose fixture.
Mini installer pull/clients/doctor/backup/stop/start/uninstall passed with a
loopback-only disposable registry. These are candidate tests, not public artifact
or physical-device tests. Disposable fixtures were removed; production unchanged.

The local package includes nine native binaries, source archive, Mini installer
and support documents; all SHA256SUMS entries verified. Its Linux UltraLite
binary passed real MCP stdio write/read, backup/restore and installed Codex CLI
registration. Publication rebuilds from the exact tag revision. Draft resumption
also handles an interrupted draft with zero assets, without overwriting assets.

Full upgrade rehearsal preserved original fields/counts: 12 projects, 18 tasks,
20 sessions, 448 records, 937 events, 12 evidence and 937 operations through 007.
New generated search columns were excluded from the historical field comparison.

Final branch CI after the last staging/checkpoint change remains required before
merge. Real-model behavior is separate from deterministic installer/MCP tests.
Public artifact installation verification remains gated on publication.
Existing physical Samsung evidence is retained; the final digest retest
is blocked if ADB remains empty. Android is a clearly labelled preview with no
endurance or battery claim. macOS and Windows ARM64 remain build-only as noted.

After the gate clears: finalize the pending-publication notices/download labels
in README and beginner guides for the release snapshot, run checks on that head,
then inspect PR #8's exact checks, ready/merge its tested head
using a merge commit, require main CI, tag v0.2.0 on that merge, follow Release
and Docker publication, verify all public assets/digests and actual installations,
then record the final result. The release workflow itself has not yet executed;
local packaging and workflow lint do not prove a successful public launch.
If a required check fails, diagnose/fix and rerun only the affected work.

Production containers, PostgreSQL volume, credentials/configuration and cold
backup remain protected; no production start, migration or upgrade is authorized.
