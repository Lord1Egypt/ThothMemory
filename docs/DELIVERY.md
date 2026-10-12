# Delivery artifacts

Source: https://github.com/Lord1Egypt/ThothMemory
Image: https://hub.docker.com/r/lord1egypt/thoth-memory
Release: https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.1.1
License: MIT; dependency notices in third_party and the image's /licenses.

The release pipeline validates source and deployment, creates draft native artifacts,
builds an OCI candidate with SBOM/provenance, tests both architectures and a clean
published-image deployment, then promotes the version/latest tags and publishes the
release. A failed gate leaves the release unpublished. See the live CI, Release and
Docker publication workflows for authoritative execution status.

## Architecture and implementation

Go 1.26.9, official MCP Go SDK 1.8.0, pgx 5.11.0, PostgreSQL 18.6; one non-root
application container plus PostgreSQL and a transient migration service. Embedded
read-only dashboard. Streamable HTTP MCP and an authenticated stdio bridge.
23 validated tools. Normalized project/session/task/evidence/history tables; optimistic
revisions, transactional idempotency, append-only events, decision supersession,
project authorization, owner attestation and revision-specific verification gates.

Source layout: cmd/thothmemory, internal/{cli,gateway,store,web}, migrations, tests,
scripts, docs, examples and .github/workflows. See ARCHITECTURE.md and DATABASE_SCHEMA.md.
See VALIDATION.md for executed tests and PERFORMANCE.md for measured resource data.

## Deployment and integration

Windows: start-thothmemory.cmd. Linux/WSL2: ./setup.sh.
After secure initial configuration: docker compose up -d.
Dashboard: http://127.0.0.1:7319. MCP: http://127.0.0.1:7319/mcp.
Connect clients with bin/thothmemory connect codex|claude|opencode|all, then install
shared instructions. User scope shows file targets and asks consent before editing.
Client trust approvals remain in effect. Native Windows uses bin\thothmemory.exe.

Backup/restore are local administrative commands; restore creates a recovery backup
and new database. Ordinary restarts preserve the named volume. Memory is untrusted
data. HTTP access never grants owner attestation or database administration.

## Known limits

Single-owner local trust domain; no public hosting. No embeddings/local LLM process.
Agent inference and reported tests are not independently verified. Hooks cannot
capture every abrupt termination. Native command hooks target Linux/WSL; Windows
uses shared instructions. OpenCode V2 schema registration is supported and tested
for preservation; the installed legacy runtime has genuine connectivity and writes,
while a complete V2 runtime/provider matrix remains unexecuted. See VALIDATION.md.
