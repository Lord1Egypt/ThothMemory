# Validation record — initial release

Executed locally on 2026-10-12. Results below distinguish real protocol/database
behavior, simulated agent identities, installed clients and provider behavior.
Release workflow results and published digests are linked from DELIVERY.md.

| Gate | Observed result |
|---|---|
| Clean migration and repeated migration | Passed against disposable PostgreSQL 18.6 |
| Existing-data upgrade | Passed from migration 001 through current version; sentinel retained |
| Migration checksum corruption | Rejected as required |
| Task concurrency / stale revision | Exactly one conflicting update committed; stale write rejected |
| Stable retry / changed payload | Original response replayed; changed payload rejected |
| Failed transaction / cancelled request | No operation persisted |
| Cross-project FK / credential scope | Invalid associations and unauthorized project access rejected |
| Claims / release / event budget | Principal ownership enforced; budget failure rolled back |
| Evidence verification | Unattested and wrong-revision evidence could not verify a task |
| Immutable event history | UPDATE rejected |
| MCP initialization / discovery / schemas | Real official SDK client; 23 tools; invalid input rejected |
| MCP authentication / read-only | Missing token rejected; reader could read and could not write |
| HTTP Host / Origin | Attacker origins and hostnames rejected |
| Three-agent handoff | Claude → Codex → OpenCode → Claude; independent real MCP connections |
| Reconnection | New SDK connection retrieved durable state |
| Linux/WSL setup | Clean generated secrets, Compose startup, migrations and doctor passed |
| Windows native setup | PowerShell 5.1 / Docker Desktop; clean setup, repeat setup, unchanged credential, real MCP demo, backup and restore passed |
| Browser dashboard | Real Chromium: unlock, project overview, search, mobile viewport; no JS errors |
| Backup/restore | SHA-256/archive checks, recovery backup, new database restore and matching event counts |
| Activated restore backup | .env database selection honored; restored-database sentinel present in subsequent archive |
| App/PostgreSQL restart | Recorded memory survived; client reconnected |
| Container architectures | Native amd64 and emulated arm64 passed actual migrations, server startup, handshake, writes and handoff |
| Runtime privileges | Runtime cannot insert attestations or create schema objects |
| Go vulnerability scan | Updated Go 1.26.9 and dependencies: no vulnerabilities found |

The race detector is enabled for integration and CI. Unit-only `go test ./...`
explicitly skips database tests without THOTH_TEST_DATABASE_URL; it is not the
integration acceptance gate. `scripts/test-integration.sh` provisions a real DB.

## Installed clients

- **Codex CLI 0.162.1:** registered in an isolated trusted project. A genuine Codex
  execution called memory_get_health successfully and received database=true.
- **Claude Code 2.1.296:** genuine `claude mcp list` reported Connected using the
  supported local configuration path. Project .mcp.json registrations require
  interactive approval; registration correctly preserves that client control.
- **OpenCode 1.17.13:** genuine `opencode mcp list` reported Connected. A genuine
  `opencode run` with a deterministic local model fixture discovered tools, created
  a project/task and retrieved that exact task via authenticated MCP. No paid model
  or external inference provider was used for that test. Model choice is independent
  of the memory interface.
- **OpenCode lifecycle:** the generated .js plugin loaded in the genuine client run
  and persisted a session.idle metadata event. Direct callback tests against the real
  backend also verified system-context loading, idle/error and compaction callbacks.
  These metadata events are explicitly not claims of completed engineering work.
- **Codex/Claude hooks:** configs use their documented event schemas, preserving
  existing groups. Detailed semantic handoffs remain an agent responsibility.
  Termination without a callback cannot guarantee a final write.

Legacy and V2 OpenCode configuration merge tests preserve unrelated settings and
servers, handle JSONC and reject conflicting existing registrations. The installed
CLI's MCP diagnostics use its legacy path; V2 registration follows the same release's
official V2 source schema (`mcp.servers`, disabled, structured timeout). A complete
interactive V2 runtime/provider matrix has not been executed. Do not interpret
schema validation as proof of every OpenCode runtime variant or future plugin API.

## Reproduce

```sh
./scripts/test-integration.sh
./scripts/check.sh
./setup.sh --build
./scripts/acceptance.sh
python3 scripts/benchmark.py
IMAGE=<candidate-tag-or-digest> ./scripts/smoke-platforms.sh
```

Actual agent CLI tests should use isolated configuration directories and disposable
projects. The deterministic model fixture is tests/fixture_model.py; it is never
part of the application/runtime and does not implement memory behavior.

No adoption, user-count, performance-at-scale or unexecuted operating-system claims
are made. Windows ARM64 client artifacts are cross-compiled; native Windows testing
used amd64. Runtime container architecture results are recorded in DELIVERY.md.
