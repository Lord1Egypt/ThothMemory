# Release readiness (final assessment, 2026-10-13)

**v0.1.2 is merged, tagged, published and independently verified**; see
[publication evidence](RELEASE_0.1.2_VERIFIED.md). No production deployment occurred.
PR #2 and PR #8 remain unmerged. Production v0.1.1 is untouched: its two
containers show their original stop time (`2026-10-13T13:37:28Z`), and a **verified cold backup**
of the volume exists (`~/ThothMemory-production-backup-20261013/`, SHA-256 `14bb3cce…8612bc`,
restored into a disposable PostgreSQL where every table's row count and content hash matched
the production fingerprint; 12 projects).

| Branch / PR | Purpose | Head | CI |
|---|---|---|---|
| `patch/0.1.2-arm64-fix`, merged PR #9 → `main` | ARM64 container fix only | merge `c7d2f8dd69a8284058f8e2d69619140ddc2d32e1` | **green**: [post-merge CI](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38456555296), [publication](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38457634410); pre-merge GitGuardian pass |
| `nextgen/full-and-mini`, Draft PR #8 → `audit/preproduction-0.1.2` | Full + Mini + UltraLite + automatic memory | tested engineering revision `c9b852adbcf9820eb8d6a5ecff0c1edefb84846a` | **six CI jobs green; GitGuardian separately failed**: [run 38452588196](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38452588196) |
| PR #2 `audit/preproduction-0.1.2` | audit hardening | `ecb8415` | green, unmerged |

GitGuardian reports one synthetic password fixture introduced in commit `1aeacc34`,
`pkg/memorycore/memorycore_test.go:66` (incident **38171534**). The check-run API
identifies it as Generic Password, not the AWS example previously suspected.
The current fixture is built at runtime; the history still holds the literal. It needs
"mark as false positive" in GitGuardian (owner), or a history rewrite of this unmerged branch,
which I did not do.

## 1. ARM64 patch release (v0.1.2): published and verified

Scope 15 files including release notes, no migration, no behavior change ([notes](https://github.com/Lord1Egypt/ThothMemory/blob/ef4867f2d3fc642dd91cee41c77177fcbda2ba94/docs/RELEASE_0.1.2.md)). Reproduced defect:
published arm64 image holds the amd64 binary (`e_machine` 62, identical SHA-256). Candidate: amd64
executable `d7b9c3fa…`, e_machine 62; arm64 `573e4c1c…`, e_machine **183**. Tests, govulncheck, real
server smoke on both platforms pass (arm64 under QEMU locally; CI repeats it).
Owner approvals were obtained; post-merge CI, Release and Docker publication succeeded.
Independent public asset/checksum/ELF/server tests passed. Public index and latest:
`sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245`.

## 2. Full (PostgreSQL), version proposal 0.2.0

Revision: PR #8 tip. Platforms: linux amd64 (arm64 after the 0.1.2 fix). Evidence: full suite incl.
race on PostgreSQL 18; migrations 001→007 applied to a clone of the production volume with history
fingerprints identical through 005 and to a live Compose stack for 006/007; contract and API shape
parity; dashboard screenshots (desktop, phone, dark); real MCP; backup/restore (PR #2 suite).
Dashboard rechecked against a disposable cold-backup restore: all 12 historical projects,
84 explorer/resume routes, revision comparison, desktop and dark mobile layout, no JavaScript
errors or horizontal mobile overflow. Screenshots stay private because they contain history.
Risks: search semantics differ from Mini/UltraLite; production upgrade
needs its own validated backup/rollback (backup now exists), explicit approval. Owner actions: merge
order (#9, #2, #8), release approval.

## 3. Mini (MariaDB), version proposal 0.1.0

Images: glibc `sha256:1a87a62e…cef9a` (recommended for arm64) and Alpine `sha256:cf6218b3…23340`
(local build IDs, not registry digests). Verified: contract, 11 MCP gates, 14 lifecycle gates on
Linux; **on the owner's Samsung SM-A165F with the released ThothDock 0.2.1 in ThothTerm 0.4.1:
install by digest, 11/11 MCP gates, 14/14 lifecycle gates including SIGKILL recovery, loopback-only
listener, abstract-socket database, MariaDB 1.6 GiB virtual (default 8 TiB)**
([ANDROID_GATE](ANDROID_GATE.md)). Verdict: **PASS WITH DOCUMENTED LIMITATIONS**. Not shown:
soak/background-kill behavior, battery, SELinux audit logs, the Alpine arm64 image on the phone.
**Production-ready only after** a digest-pinned image is published to a registry the phone
trusts, and the owner accepts these limits. Do not claim more.

## 4. UltraLite (SQLite, headless), version proposal 0.1.0

| Artifact | SHA-256 |
|---|---|
| linux-amd64 | `b77149d1d215e94cde29c7d27f845a420599aa8a717356632e67514365eb0758` |
| linux-arm64 | `d01f93ac0015086719c9ad14ba43c97cefbb9848b80ad40927029084d4ae02e2` |
| windows-amd64.exe | `66ae344e3a54611e95fc38654b781d0e1c7efdfb691195df274f37f1b582eb40` |
| darwin-arm64 (built, never run) | `ce8c6af4bd562d867715821a22ae90aedf829a492121b11df2e310da86158e85` |
| android-arm64 (**run on the phone**) | `33d6a9eb55ebe6ea376fa961633bcb6a77b15201b6a737912ab23f17e0ebe16f` |

Reproducible (two builds identical). Verified: contract suite, multi-process races, SIGKILL recovery,
backup during writes, restore refusal, export/import, native CLI + stdio test, Linux and Windows
CI jobs (all green), Windows x64 native with owner-only ACLs, Linux arm64 native CI, Android arm64
on the phone (init, doctor, 10k-event load: context 7.5 ms, search 51 ms, write 8.9 ms). The build
excludes the HTTP gateway and web assets (checked in CI). Unsigned binaries; macOS untested.
**Ready for owner review.**

## 5. Automatic memory (all editions)

Real Claude Code fired SessionStart, UserPromptSubmit, Stop, SessionEnd and PreCompact into memory.
Real Codex and real Claude Code exchanged an idea/handoff. Codex hooks: **not verified**; Codex loads
project hooks only after folder trust and hook review in its own UI (documented enable steps;
fallback verified). OpenCode: connection verified, real-model session not run. Windows command
hooks unsupported.

## 6. Remaining owner decisions

1. Mark the GitGuardian finding a false positive (or approve a history rewrite).
2. PR #9 is complete. Approve later merges separately: #2, then #8 as appropriate; renumber PR #2's
   `0.1.2-rc.1` after #9.
3. v0.1.2 publication is complete. Approve later publication separately: a Mini image to a trusted registry; UltraLite
   binaries (consider signing).
4. Approve a production upgrade only after rehearsing it on the verified backup.
5. Optional: trust a project in Codex and approve its hooks to confirm Codex hook capture.
