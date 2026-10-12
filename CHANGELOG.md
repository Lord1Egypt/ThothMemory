# Changelog

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
