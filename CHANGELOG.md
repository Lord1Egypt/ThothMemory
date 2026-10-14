# Changelog

## 0.2.1 (Mini 0.1.1, Full 0.2.1) — corrective release candidate, not published

Fixes the Mini backup failure found by the independent check of v0.2.0
(`mariadb-dump: TLS/SSL error: certificate is not yet valid`). Mini 0.1.0 and v0.2.0 stay
as published; nothing is replaced or retagged.

- **Mini**: native `mariadb-dump`/`mariadb` no longer negotiate TLS over the private Unix
  socket (`skip-ssl`, socket transport only; the TCP fallback keeps TLS and verification).
  A failed restore now drops its half-loaded database.
- **Tests**: unit tests pin the option file for both transports; the Mini lifecycle now
  checks a failed backup, corrupted and malformed backups, per-table row checksums, foreign
  keys, migrations, real MCP reads on the restored copy, and a backup with the client clock
  5 s behind the server (fails on 0.1.0 with the original error). Required in Mini CI on
  amd64 and emulated arm64, and in the Docker publication gate for both architectures.
- Full 0.2.1 and UltraLite are rebuilt unchanged in behavior; versions only.

## Unreleased — next generation candidate (branch nextgen/full-and-mini, stacked on PR #2)

Three editions on one shared core. Nothing here is released, merged or deployed.

- **Shared core**: edition-independent domain and mutation algorithm (`internal/memory`),
  MCP tool contracts without HTTP (`internal/mcptools`), shared SQL for MariaDB and SQLite
  (`internal/sqlstore`). PostgreSQL SQL is unchanged; one contract suite runs on all three.
- **Mini (MariaDB 11.4)**: one container supervising a private, network-disabled
  `mariadbd`; Alpine and glibc images; CLI-only ThothDock installer; verified on ThothDock
  0.2.1 + Garden PRoot on a Linux host. Caps MariaDB's 8 TiB address-space reservation.
- **UltraLite (SQLite, pure Go, headless)**: one executable, per-user data directory, MCP
  stdio, native CLI (init, doctor, projects, brief, search, tasks, timeline, backup,
  restore, export, import, attest, permissions, hooks, capture); no dashboard, no HTTP
  listener, no daemon. `pkg/memorycore` for in-process embedding.
- **Automatic memory**: `idea` and `note` records; duplicate suppression; secret refusal on
  every write; lifecycle capture engine (session start/resume, opt-in prompts, compaction,
  idle, end) with a bounded idempotent retry queue; hooks for UserPromptSubmit and
  SessionEnd; official `THOTHMEMORY_AGENT_PROTOCOL.md` installed through a non-destructive
  managed block in `AGENTS.md` and `CLAUDE.md`.
- **Dashboard** redesigned: projects register, resume briefing with stale-revision check,
  task/session/decision/evidence/timeline explorers, trust seals, mobile and dark mode.
- **Portability**: checksummed project export and fast-forward import across Full, Mini and
  UltraLite; migrations 006 (imports) and 007 (ideas and notes) for PostgreSQL.
- **CI**: MariaDB service, Mini image architecture checks, emulated arm64 gates, UltraLite
  native matrix with a headless-boundary check.
- Fixes found on the way: lowercase UUID/revision lookups; hook registration used the
  canonical form instead of the remote URL; QEMU/musl MariaDB bootstrap (glibc variant).

## Security hardening included in 0.2.0

- Preserve scoped/revoked credential policy across setup; add local permissions CLI.
- Protect Windows secrets, configuration backups and database dumps using owner ACLs.
- Honor isolated/global client homes consistently for instructions and hooks.
- Add migration 005 for NFC/simple search and literal technical/Arabic substrings.
- Evaluate search query representations once in PostgreSQL generic prepared plans.
- Include durable checkpoints and incomplete sessions in briefings; preserve UTF-8.
- Resolve existing image/port choices correctly in Windows bootstrap.
- Report failed OpenCode lifecycle metadata writes as sanitized unsynced warnings.
- Guard unverified OpenCode V2 registration and serialize draft-only publication.
- Add project boundary, cancellation, realistic handoff, migration rollback and native
  Windows regressions; report measured audit results under docs/audits.

No release or production upgrade is performed by this audit branch.

## 0.1.2: ARM64 container fix only

- **Fix (high):** the published 0.1.1 `linux/arm64` container held the x86-64 server binary
  (an explicit `ARG TARGETARCH=amd64` default overrode BuildKit's automatic value). The
  Dockerfile now takes the automatic target values, and the platform smoke test inspects the
  ELF `e_machine` of the executable inside each image (62 for amd64, 183 for arm64) before
  running it, so an x86 binary that merely runs on an x86 host can no longer pass.
- Smoke test waits on the final PostgreSQL server's TCP readiness instead of the temporary
  initialization server's socket.
- No schema, migration, API, dashboard or behavior change. Version strings and default image
  tag move to 0.1.2. The published 0.1.1 tag and image digests are not modified.

## 0.1.1

Initial public release. The unpublished 0.1.0 tag was retained after a pre-release
recovery check found that backup ignored database activation through .env.
Backup and restore now resolve the active database through Compose itself.

Initial release: local PostgreSQL memory; authenticated MCP; append-only events,
optimistic task revisions, idempotent operations, scoped credentials, evidence and
owner attestation; agent sessions, checkpoints and handoffs; FTS across records,
tasks, events and evidence; embedded dashboard; Linux/WSL and Windows bootstrap;
Codex, Claude Code and OpenCode registration; shared instructions and optional
lifecycle adapters; safe backup/new-database restore; CI and multi-platform images.

Evidence attestation records owner review and does not independently execute tests.
See docs/VALIDATION.md for measured results and platform/client integration limits.
