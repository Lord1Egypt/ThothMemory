# Release readiness: next-generation candidate

**Nothing has been merged, tagged, published or deployed.** Production (v0.1.1) is
unchanged: its two containers still show the same stop time and its named volume and
secret files carry their original timestamps. Everything below was tested on disposable
databases, containers and directories. A release needs the owner's decision.

Source: branch `nextgen/full-and-mini`, stacked on PR #2 head `ecb8415`. Code commit
reproduced below; later commits are documentation only.

## Verdict by area

| Area | State | Basis |
|---|---|---|
| Full (PostgreSQL) correctness and security | **Verified** on host | PR #2 suite unchanged and green; shared contract on PostgreSQL; clone of the production volume migrated 001→005 with history fingerprints identical; 006→007 applied to a live compose stack |
| Full dashboard (projects register, explorers) | **Verified** in Chromium (desktop, phone width, dark) on Mini and parity-tested API on all editions | screenshots in `docs/screenshots`; not re-screenshotted against Full |
| Mini (MariaDB) on Linux host | **Verified** | contract suite, 11 MCP gates, 14 lifecycle gates, runtime-privilege and migration tests, amd64 |
| Mini on ThothDock v0.2.1 + Garden PRoot | **Verified on a Linux host only** | install/clients/hooks/backup/restore/uninstall scripts, restart/SIGKILL/recreate |
| Mini on Android hardware | **BLOCKED, no device** | [ANDROID_GATE.md](ANDROID_GATE.md) runbook |
| Mini arm64 | glibc image passed 11 gates under QEMU; Alpine arm64 failed to bootstrap there | choose per device gate |
| UltraLite (SQLite, headless) | **Verified** Linux amd64 (all tests, race), Windows x64 (native init/doctor/ACLs), Linux arm64 (QEMU init/doctor) | macOS and Android binaries built, not run |
| Automatic memory | **Partly verified** | see below |
| Portability | **Verified** on all three pairs and through real Compose/Docker entry points | `tests/portable_test.go` |
| Backup/restore | **Verified** per edition on disposable data | Full by PR #2 suite; Mini 14 gates; UltraLite tests incl. backup during writes and restore refusal |
| Performance | **Measured on host** | [BENCHMARKS.md](BENCHMARKS.md); no Android numbers |
| Docs | Complete for the requested set | README section, quickstart, automatic memory, handoff, protocol |

## Automatic memory: what is and is not proven

Proven with **real** clients and models: Codex recorded an idea and an honest
"deferred, unverified" handoff from nothing but an `AGENTS.md` reference; a separate real
Claude Code session retrieved it, named the author and added a note under its own
credential. Real Claude Code fired `SessionStart`, `UserPromptSubmit`, `Stop` and
`SessionEnd` hooks into memory. Genuine client connections: Claude (`✔ Connected`),
OpenCode (`✓ connected`), Codex (registered, stdio).

Proven with **deterministic fixtures and real binaries** (`TestAutomaticMemoryEndToEnd`):
all 20 requested scenarios, including a locked database, queue replay without duplication,
secret refusal, scoped credentials and disabling.

**Not proven**: real Codex firing hooks (none reached memory even with trust bypassed;
cause not isolated), a real `PreCompact` event, a real-model OpenCode session, any
Windows command hook (unsupported), and long-running behavior.

## Open risks and limits

1. **MariaDB on a 39-bit Android kernel** was a real risk (default 8 TiB reservation). It is
   capped, but only a device run can confirm MariaDB starts there.
2. **Search differs by edition** (stemming and operators); documented, ranking not equal.
3. **Codex hooks** may not work in your version; fall back to instructions plus MCP writes.
4. **Secret screening is pattern-based**; it refuses obvious credentials, not all secrets.
5. **UltraLite trust boundary is the OS account**; another process as the same user can read
   the database file.
6. Pre-existing from PR #2: published 0.1.1 arm64 image is x86-64 (fixed in the candidate,
   awaiting an approved patch release); OpenCode V2 runtime unsupported.
7. CI changes (MariaDB, Mini images, UltraLite matrix, Windows race exclusion) were written
   and YAML-validated but **have not run on GitHub**.
8. No Mini image is published; no `latest` promotion or release artifact exists.
9. The dashboard has no live-edit features by design; Full/Mini dashboard code was not
   run against a populated production-sized dataset.

## Release candidate artifacts (reproducible)

UltraLite built twice from the same tree with `CGO_ENABLED=0 -trimpath -ldflags='-s -w -buildid='`:
identical SHA-256 both times ([sums](audits/evidence/rc-ultralite-SHA256SUMS.txt)).

| Artifact | SHA-256 / image ID |
|---|---|
| thothmemory-ultralite-linux-amd64 | `b77149d1d215e94cde29c7d27f845a420599aa8a717356632e67514365eb0758` |
| thothmemory-ultralite-linux-arm64 | `d01f93ac0015086719c9ad14ba43c97cefbb9848b80ad40927029084d4ae02e2` |
| thothmemory-ultralite-windows-amd64.exe | `66ae344e3a54611e95fc38654b781d0e1c7efdfb691195df274f37f1b582eb40` |
| thothmemory-ultralite-darwin-arm64 (untested) | `ce8c6af4bd562d867715821a22ae90aedf829a492121b11df2e310da86158e85` |
| thothmemory-ultralite-android-arm64 (untested) | `33d6a9eb55ebe6ea376fa961633bcb6a77b15201b6a737912ab23f17e0ebe16f` |
| Mini (Alpine) local image ID | `sha256:2f4e3967bf1ff35bd66e4ae90dd5f973fb8138b7d6d86a1cdaf066290cc50e74` |
| Mini (glibc) local image ID | `sha256:f97f80342892a4e8ba69ee7373d135fded334d59e4d5d9dc0792e016eace3dff` |
| Full local image ID | `sha256:670b5ed51fa0a6115329d6585ba6b5a5d225fa1af2b2aca0f8c823d477ff445b` |

Image IDs are local build identities, not registry digests (nothing was pushed to a public
registry). Windows and macOS binaries are unsigned.

## Needed from the owner

1. Review the branch; decide whether to open a PR (not opened) and how it relates to PR #2.
2. Run the Android gate on the Samsung device (or choose UltraLite-in-PocketClaw instead).
3. Run `scripts/real-client-handoff.sh` yourself if you want to see the Codex→Claude flow.
4. Approve (or not): merging, a patch release for the arm64 fix, publishing Mini images,
   upgrading production (back up first; migrations 005–007 are additive and were applied
   only to disposable clones).
