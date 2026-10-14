# Verified engineering checkpoint — 2026-10-13

## Latest checkpoint — 2026-10-14 public release

v0.2.0 is public at be30dfe67c9094f197d8810add7b77fb3abcb5d8. GitGuardian,
PR/main CI and release workflows passed. Independent Mini backup subsequently
FAILED with certificate-not-yet-valid TLS error; launch is not fully verified.
See [PUBLIC_RELEASE_VERIFICATION.md](PUBLIC_RELEASE_VERIFICATION.md) for exact
digests, downloads, successful checks and the stopped corrective-release gate.
This supersedes historical pending/security-blocked notes below. Production
remains stopped and unchanged; do not mutate published semantic-version tags.

## Launch resumed — 2026-10-14

Owner reviewed GitGuardian incident 38171534. Supported GitHub check-run
rerequest succeeded; fresh GitGuardian check 115653901254 at f94a63b reported
SUCCESS with 29 commits scanned and no secrets. No bypass or scan disablement.
Release-facing documentation finalized; await checks on the final PR head,
merge PR #8, require main CI, then tag/publish/independently verify v0.2.0 using
the existing conditional owner authorization. Older blocked notes are historical.

## Current product launch checkpoint: external security gate blocked

The owner now conditionally authorizes PR #2/#8 merges and unified v0.2.0
publication after required CI/security success. Production remains excluded.
PR #2 integration head a211b973bddd686e1f87253fa6abd498395d8570 passed CI and
GitGuardian and merged as c6a78e5412bc7ddc569382bcd7299a5b80511216.
Its main CI 38462178511 passed. No intermediate v0.1.3 release was published.

PR #8 targets main and retains the shared core while incorporating that history
without rewriting or cherry-picking duplicate commits. Product packaging,
beginner-first README, English/Arabic tutorials and safe upgrade documentation
are implemented. Unified release scripts stage all Full/UltraLite assets and
separate Full/Mini registry candidates before publishing the primary release.
Versions prepared: Full 0.2.0, Mini/UltraLite 0.1.0. Historical v0.1.2 unchanged.

Engineering revision 7aec0c02451a7467fa4bfdb59c958dfa03c20766 passed all six
jobs in https://github.com/Lord1Egypt/ThothMemory/actions/runs/38463262041 .
Includes native newcomer tests on Windows amd64/Linux amd64/Linux ARM64 with
actual Codex CLI registration and real MCP stdio idea persistence/backup/restore.
Mini's newly added scan initially found CVE-2026-84782 in libssl3t64 3.15;
the pinned 3.16 fix passed the fresh scan. No vulnerability suppression.

Local integration/race/static tests, nine-binary packaging checksums, workflow
lint, Full/Mini installer/recovery scenarios and 12-project historical dashboard
passed. Original fields/counts survived migrations 005–007 (new generated search
fields excluded). Dashboard: 84 routes, no JS errors/overflow, dark mobile.
Candidate evidence/assets live in .validation/launch-* and bin/launch-dist/.
The final release must rebuild from its tag; no v0.2.0 public asset is claimed.

GitGuardian remains FAILURE on historical synthetic fixture incident 38171534.
Owner action was requested through the UI and remains unanswered; elapsed time
does not clear this gate. Do not merge PR #8 or tag/publish v0.2.0 until the
actual check succeeds. Direct incident URL and complete resume steps are in
[LAUNCH_STATUS.md](LAUNCH_STATUS.md). No further publication permission is
needed within the owner's conditional scope once mandatory checks pass.

ADB is empty; final Mini digest/device retest remains BLOCKED. Historical
Samsung pass is preserved, and Android publication is allowed only as the
documented preview. No battery/endurance claim. Original production containers
retain their stop timestamps; no credentials/configuration/data were changed.

Next action: owner reviews/marks the synthetic fixture false positive in
GitGuardian; inspect the updated PR check, finalize release-facing availability
wording, obtain green checks on that head, then merge/tag/publish/verify the
complete family. Do not describe the launch as complete while blocked.

## Current release status: v0.1.2 completed

Owner approved PR #9 merge and conditional v0.1.2 publication. PR #9 merged as
`c7d2f8dd69a8284058f8e2d69619140ddc2d32e1`; fresh main CI succeeded before
annotated v0.1.2 was pushed. Release and Docker publication workflows succeeded.
Public downloads/checksums, both image architectures, immutable child digests,
provenance/SBOM, and independent public server smoke tests passed. latest equals
0.1.2; v0.1.1 and historical Git tags unchanged. Production remains stopped and
untouched. No PR #2/#8 merge or other release was performed.

Public release: https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.1.2

Full image: `lord1egypt/thoth-memory@sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245`

Complete hashes, workflow URLs, platform limitations, recovery instructions and
executable verification commands: [RELEASE_0.1.2_VERIFIED.md](RELEASE_0.1.2_VERIFIED.md).
The approval record below is retained as historical context and superseded by
this completed result. Next engineering step is PR #2 integration and fresh CI;
its merge/publication require a new owner approval. PR #8/GitGuardian and final
Mini device gate remain outstanding.

## Superseded pre-publication checkpoint

At that checkpoint release action was **awaiting explicit owner approval**, as required by
section 3 of the owner's final integration mission. No approval was inferred.
Rechecked PR #9 at `ef4867f2d3fc642dd91cee41c77177fcbda2ba94`: Draft/open,
MERGEABLE/CLEAN; test, Windows and GitGuardian all SUCCESS on its actual head.
Main remains `88a4a3bf419dce13994b56063f5c925160ee63f2`. Its 3 patch commits
change 15 files with no migrations, web assets or database dependency changes.
Retained image archive checksum and both architecture/MCP smoke logs verified.
No unchanged binary was rebuilt and no successful suite was rerun.

Origin tags remain v0.1.0 `4839a2e1d1049ce5daf9df123b23b598488b46fe` and
v0.1.1 `854cb758489dfa4fd0dd2ba3b6d129773f8d39a2` (annotated tag objects).
v0.1.2 is absent; GitHub currently exposes only the v0.1.1 release. Docker Hub
secret names are configured; their validity is not claimed until publication.

The two separate approval actions are:

1. Ready PR #9 and merge its verified head into main using a merge commit;
   record the resulting SHA and require successful CI on that merge.
2. After that success, annotate/push v0.1.2 on the approved merge SHA. This
   publishes a public candidate image, tests its immutable digest, publishes
   version 0.1.2 and GitHub assets, and promotes Full's latest tag. Follow through
   with independent release/checksum/index/child-ELF/startup verification.

Executable resume after action 1 is explicitly approved:

```sh
cd /home/lordegypt/DataBase/ThothMemory-arm64patch
gh pr ready 9
gh pr merge 9 --merge --match-head-commit ef4867f2d3fc642dd91cee41c77177fcbda2ba94
gh api repos/Lord1Egypt/ThothMemory/pulls/9 --jq .merge_commit_sha
```

Fetch the merge and record its exact SHA, inspect its main-branch CI, and only
after action 2 approval use `git tag -a v0.1.2 <APPROVED_MERGE_SHA> -m
"ThothMemory 0.1.2: ARM64 container fix"` and `git push origin refs/tags/v0.1.2`.
The Release→Docker workflow chain owns publication; do not dispatch it a second
time while it runs. See RELEASE_PLAN.md for verification and corrected rollback.

| Product | Version/tag | Prepared / CI verified | Approved / published / public verified / deployed |
|---|---|---|---|
| ARM64 fix | v0.1.2 | yes / yes at PR #9 head | no / no / no / no |
| Audit hardening | v0.1.3 | existing branch / pre-integration only | no / no / no / no |
| Full | v0.2.0 | candidate / pre-integration only | no / no / no / no |
| Mini | mini-v0.1.0 | glibc ARM64 candidate / host+QEMU verified | no / no / no / no |
| UltraLite | ultralite-v0.1.0 | five native artifacts / existing native CI | no / no / no / no |

Mini's proposed image namespace is `lord1egypt/thoth-memory-mini:0.1.0`;
Full keeps `lord1egypt/thoth-memory`. UltraLite assets use existing explicit
`thothmemory-ultralite-OS-ARCH` names plus SHA256SUMS. The edition tags do not
match the current Full release trigger. Edition publication automation must be
implemented and validated after integration, before approving those tag pushes;
the names here reserve distinct identities and do not claim working triggers.

PR #8 engineering CI at c9b852a is SUCCESS, all six jobs, run 38452588196.
GitGuardian remains FAILURE on synthetic test fixture incident 38171534:
https://dashboard.gitguardian.com/workspace/668579/incidents/38171534?occurrence=303908630
Owner: inspect the test-only occurrence, mark false positive with reason
"synthetic secret-rejection fixture; no credential authenticates against a service",
then recheck the PR status. Current source constructs the fixture at runtime.
Existing secret-history audit found no real credentials; no new evidence contradicts it.
Do not bypass or globally disable this check.

ADB still has no device. Final Mini digest retest remains BLOCKED; preserve the
historical physical-device pass and all documented endurance/process limitations.
Production containers retain their original stop timestamps; backup and original
volume preserved. Further integration follows the owner's order: PR #2 after #9
merges, then PR #8 after #2 merges. Each merge/release has its own approval.

## Prior engineering verification

This follow-up began at `d391f0b247dd83f595f079945fa11af41a68914c`, not the older
`fd23809` checkpoint. PR #8 already existed as a Draft targeting PR #2's branch.
PR #2 remains open at `ecb841547d5365524cb718d6443f171820692d92`; independent
Draft PR #9 remains at `ef4867f2d3fc642dd91cee41c77177fcbda2ba94`, targeting main.
No merge, tag, publication, production start, production migration or credential
change was performed. PocketClaw and unrelated client configuration were untouched.

## Corrections completed in this follow-up

- GitGuardian's API identifies incident 38171534 as a synthetic Generic Password
  in `pkg/memorycore/memorycore_test.go`, introduced at `1aeacc34`. Corrected the
  previous speculative AWS attribution. Constructed the fixture at runtime;
  secret refusal and sanitized error assertions still pass. Historical detection
  requires owner review in GitGuardian; no history rewrite was attempted.
- Mini acceptance previously called a missing reader-token test PASS. It now
  records SKIP and exits 2 for incomplete coverage. ARM64 CI now supplies the
  reader token. Tested against a real disposable Full instance: with both tokens
  **11/11, 0 skipped, exit 0**; without reader **10/11, 1 skipped, exit 2**.
  Older ARM64 CI logs saying "PASS ... skipped" do not establish that gate.
- Corrected stale Mini/UltraLite Android statements and the broken patch notes
  link. Kept historical device evidence distinct from fresh host results.

## Fresh verification

- `scripts/check.sh`: formatting, module integrity, vet and race tests PASS.
- `scripts/test-integration.sh`: real PostgreSQL 18, MariaDB 11.4.12 and SQLite
  PASS. Includes all six portability directions, concurrency, migrations,
  automatic-memory scenarios, authorization, crash recovery and online backup.
  The SQLite helper-process entry is intentionally skipped in the parent run;
  its subprocess scenarios passed. Initial attempts hit the sandbox's read-only
  default Go cache; successful runs used `GOCACHE=/tmp/thoth-final-go-cache`.
- Full dashboard: restored the existing cold archive into the disposable volume
  `thoth-final-dashboard-copy`, migrated the copy through schema 007, and served
  it only on loopback. Browser exercised **12 projects / 84 routes**, revision
  comparison, desktop, dark mobile, no JavaScript errors and no horizontal page
  overflow. Private screenshots and driver are under `.validation/dashboard/`;
  do not publish screenshots containing historical memory.
- Patch images: `scripts/smoke-platforms.sh` freshly passed ELF architecture,
  migration, application startup, MCP handshake/discovery, writes and handoff
  on amd64 native and arm64 **QEMU**, with disposable PostgreSQL.
- UltraLite: rebuilt all five platforms; every SHA-256 exactly matches
  `audits/evidence/rc-ultralite-SHA256SUMS.txt`. A second Linux amd64 build was
  byte-identical. Fresh native Linux init/doctor passed owner-only permissions,
  integrity, foreign keys, real read-credential MCP stdio (9 tools, 10 ms) and
  child termination. This follow-up did not rerun native Windows or Android.

## Usable local candidates

UltraLite binaries and SHA256SUMS: `bin/rc/` in the nextgen worktree (ignored,
not published). Linux amd64 SHA-256:
`b77149d1d215e94cde29c7d27f845a420599aa8a717356632e67514365eb0758`.
Full/Mini/UltraLite version proposals remain 0.2.0 / 0.1.0 / 0.1.0.

Patch 0.1.2 images are retained locally. An offline Docker archive and checksum
are in the patch worktree's `bin/rc/thothmemory-0.1.2-candidate-images.tar`:
`84a8afea80d12fa962e225a2b53ccbaf7425d234484f8beaa7f75a21ed7f4c51`.
Image IDs (local identities, not a claim of registry publication):

- amd64 `sha256:39924a525ce0a1254edf9930a2ac21e291c81a39bf308eaebbf6a5c87d682de8`
- arm64 `sha256:fbe8bd0cfdf0cb276e8c32d1544d23374d22dca10753d893652c4b9146f3a8ea`

Registry provenance/SBOM attestations are not yet claimed for these candidates.
The existing tag-triggered release pipeline also promotes latest: approving a
tag push must explicitly cover publication and promotion; do not push a tag as
an innocuous CI trigger. See [release procedure](RELEASE_PLAN.md).

## Preserved evidence and remaining gates

- Actual GitHub CI at starting HEAD: all six jobs succeeded in
  [38448408897](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38448408897).
  GitGuardian was separately FAILURE. Patch CI remains green in
  [38441187422](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38441187422).
  Inspect PR #8's checks on the final correction commit before approving release.
- Samsung logs substantiate the earlier **PASS WITH DOCUMENTED LIMITATIONS**:
  11 MCP / 14 lifecycle gates on released ThothDock 0.2.1, glibc MariaDB,
  1,609 MiB VmSize, 3 added processes. No new device run: both `adb devices -l`
  and `adb mdns services` are empty. Soak/Doze/battery remain unverified.
  Reconnect via Samsung Developer options → Wireless debugging; use its shown
  address with `adb connect IP:PORT`, or `adb pair IP:PAIRING_PORT` first if
  needed (enter the code locally). Then follow [Android commands](ANDROID_GATE.md).
- Codex 0.162.1 reports hooks stable/enabled. Official documentation confirms
  project trust and per-definition `/hooks` review. No trust bypass was used;
  actual Codex hook capture remains unverified. Supported fallback remains
  MCP semantic writes + AGENTS.md protocol + checkpoints. Real Codex→Claude
  handoff is recorded in existing evidence. Claude lifecycle/PreCompact is
  previously reported; OpenCode real-model behavior remains unverified.
- Production containers retain `2026-10-13T13:37:28Z` stop timestamps. Existing
  cold backup was read only; SHA-256 reverified:
  `14bb3ccee45ed76cbd76676b710047787f9655525f289b516558a555d28612bc`.
  Its parent directory is mode 0700. No original volume was mounted or modified.
  The restored copy successfully supplied the historical dashboard above.

Next owner action: review GitGuardian incident 38171534, then separately approve
the proposed patch merge/publication sequence. Full release still requires stack
integration and fresh CI; Mini distribution needs approved trusted-registry
publication and acceptance of device limitations. UltraLite unsigned binaries
are prepared; macOS execution is unverified. Production upgrade remains a
separate decision: retain the stopped original volume, rehearse against a new
backup restore, and roll back by selecting a preserved compatible copy, never
by downgrading a migrated database in place.
