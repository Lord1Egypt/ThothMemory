# ThothMemory

**One Brain. Every Agent. Zero Lost Context.**

Local-first shared engineering memory for Codex CLI, Claude Code and OpenCode.
One lightweight Go process, PostgreSQL and standard authenticated MCP. No LLM keys,
paid services or model-specific database integrations required.

Source and one-click launchers: https://github.com/Lord1Egypt/ThothMemory

## Quick deployment

Clone or download the source repository. Start Docker Desktop (Linux containers) or
Docker Engine with Compose, then run `./setup.sh` (Linux/WSL2) or
`start-thothmemory.cmd` (Windows). Setup pulls this image, generates protected local
credentials, starts PostgreSQL, applies migrations and verifies MCP connectivity.
After setup: `docker compose up -d`.

Dashboard: http://127.0.0.1:7319
MCP: http://127.0.0.1:7319/mcp (bearer authentication)
Connect: `bin/thothmemory connect all` (Windows: bin\thothmemory.exe).

## Features

Persistent project context, agent sessions and handoffs; revision-aware evidence;
append-only events and decision supersession; transactional idempotency and optimistic
task revisions; project-scoped credentials; PostgreSQL full-text search; embedded
read-only dashboard; safe backup/restore to a new database.

## Runtime and safety

Requires PostgreSQL 18; use the provided Compose deployment. Application runs as
non-root, read-only, without a Docker socket. Only loopback ports are published by
default; PostgreSQL is internal. Secrets stay outside images and Git. Data persists
in a named volume. The image supports linux/amd64 and linux/arm64; release gates
execute real MCP smoke tests on native/emulated platforms before latest promotion.

This is a local single-owner service, not a public internet deployment. Reported
agent evidence is unverified until owner review; older revision evidence cannot
verify a new revision. Instructions and checkpoints are required: connecting MCP
alone does not guarantee memory updates or a final handoff after abrupt termination.

Docs: https://github.com/Lord1Egypt/ThothMemory/tree/main/docs
Security: https://github.com/Lord1Egypt/ThothMemory/blob/main/docs/SECURITY.md
Backups: https://github.com/Lord1Egypt/ThothMemory/blob/main/docs/BACKUP_RESTORE.md
License: MIT; third-party notices are included in /licenses.
