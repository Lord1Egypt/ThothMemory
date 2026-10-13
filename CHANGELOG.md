# Changelog

## Unreleased — 0.2.0 security hardening


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
