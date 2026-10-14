# ThothMemory v0.2.2 — stabilization release

Components: **Full 0.2.2** (PostgreSQL 18), **Mini 0.1.2** (MariaDB 11.4), **UltraLite 0.1.0** (SQLite, unchanged).
v0.2.0, v0.2.1, Mini 0.1.0 and 0.1.1 stay as published; nothing is replaced.

## What changed

- **Mini:** the supervisor now records each server stop in `/data/shutdown.log` (`clean` only when mariadbd exits
  with status 0 after SIGTERM). The recovery gate reads that durable record instead of container logs, whose final
  line can be lost, and checks 20 consecutive restarts. A CI failure of the old log-based check was never
  reproduced; whether it was a lost log line or a real early exit is **not established**, and the new record
  would now distinguish the two.
- **Mini restore:** cleanup of a failed restore is bounded (30 s), checked, never claims "dropped" unless the
  drop succeeded, keeps the original error, and refuses to touch any database that is not a restore copy.
  Unit tests cover both branches and the safety guard.
- **Verification tooling:** `scripts/ultralite-acceptance.py` (native UltraLite acceptance) and
  `scripts/dashboard-check.mjs` (Full dashboard, light/dark, desktop/mobile).
- **Docs:** version references, the Mini guidance and the security note on the database socket.

## Which file should I download?

UltraLite (no Docker): `thothmemory-ultralite-windows-amd64.exe`, `thothmemory-ultralite-linux-amd64`,
`thothmemory-ultralite-linux-arm64`. macOS and Android UltraLite assets are experimental. Full clients are
`thothmemory-{linux,windows}-*`; Full's installer is in the source package. Mini uses `thothmini.sh` plus
`MINI_IMAGE_DIGEST.txt`. Setup steps: README.md and docs/GETTING_STARTED.md.

## Limits

MCP registration does not by itself save every conversation. Codex hook capture is unverified; OpenCode has no
real-model verification. Android Mini is a preview: a phone test of this exact digest may be pending (see
docs/PUBLIC_RELEASE_VERIFICATION.md). Verify downloads with `sha256sum -c SHA256SUMS`.
