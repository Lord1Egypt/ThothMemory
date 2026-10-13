# Verified engineering checkpoint — 2026-10-13

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
