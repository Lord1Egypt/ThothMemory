# Executed public-image demonstration

Executed 2026-10-12 using a fresh clone of the public v0.1.1 tag, Docker Desktop and
isolated Compose project/ports. No Go toolchain was used by the installer. Test
identity data was simulated; MCP, PostgreSQL, credentials and containers were real.

## Linux / WSL2

```sh
git clone --depth 1 --branch v0.1.1 https://github.com/Lord1Egypt/ThothMemory.git "$DEMO_DIR"
cd "$DEMO_DIR"
export COMPOSE_PROJECT_NAME=thothmemory-published-validation
export THOTH_PORT=7321
./setup.sh
./setup.sh                         # repeat; credential hash unchanged
./scripts/acceptance.sh             # demo, restarts, backup/restore and privileges
./bin/thothmemory connect all --dir "$DISPOSABLE_PROJECT" --yes
./bin/thothmemory instructions --dir "$DISPOSABLE_PROJECT" --yes
docker compose stop
```

First clean install pulled the public candidate by digest, generated secrets and
initialized a new named volume in 33 seconds (cached base layers). After promotion,
the entire sequence passed using `lord1egypt/thoth-memory:0.1.1`; repeat credentials
were unchanged. Observed output excerpts:

```text
MCP handshake: OK
Authentication: OK
Tool discovery: 23 tools
Database: ready
Installer repeat preserved credentials: PASS
claude-code: session, task retrieval/write, handoff OK
codex: session, task retrieval/write, handoff OK
opencode: session, task retrieval/write, handoff OK
claude-code: session, task retrieval/write, handoff OK
projects=1
events=26
Deployment, handoff, restart persistence, backup/restore integrity and least privilege: PASS
FRESH PUBLIC CLONE / PUBLISHED IMAGE / ONE-CLICK / THREE-AGENT / PERSISTENCE / BACKUP / RESTORE: PASS
```

The fresh database had 13 events after the first four-session demo; 26 after repeating
against the promoted image. Restart retained matching counts. Restore created a new
database and recovery archive. The activated-backup sentinel proved that `.env`
database selection affects subsequent backups. All three installed CLI versions
were detected, registrations created in a disposable project, and shared instructions
written. Normal client settings were untouched.

## Native Windows

Public tagged source was copied to an empty Windows temporary directory. PowerShell
5.1 and Docker Desktop used a separate Compose project and port 7323. Ran:

```powershell
.\setup.ps1
.\setup.ps1                         # credential hash unchanged
.\bin\thothmemory.exe demo
.\bin\thothmemory.exe backup windows-validation.dump
.\bin\thothmemory.exe restore windows-validation.dump --confirm
docker compose stop
```

The installer pulled the promoted image, extracted the native Windows CLI, initialized
PostgreSQL and migrations, and passed authenticated MCP discovery. The demo wrote and
retrieved memory across all four simulated sessions. Restore reported `projects=1`,
`events=13`, retained the original database and created a recovery backup. Final output:

```text
WINDOWS PUBLISHED IMAGE NATIVE SETUP / REPEAT / MCP / BACKUP / RESTORE: PASS
```

The first Windows test invocation encountered a WSL interop socket timeout before
PowerShell started; retry completed successfully. This was not an installer failure.

## Publication and architecture verification

The [successful publication run](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38257187961)
pulled the candidate by digest for native amd64 and emulated arm64. Each ran migrations,
started the actual server, discovered 23 tools, and performed the four-session demo.
Only then did clean deployment acceptance run and version/latest promotion occur.
Public release download returned HTTP 200; all four native binary SHA-256 checks
passed. Anonymous Docker pull by the final index digest passed. Per-platform SPDX
SBOM and SLSA provenance were retrieved. Digests are in [DELIVERY.md](DELIVERY.md).

The local primary installation was backed up and upgraded to the released image;
existing memory and credentials were retained. Dashboard remains at port 7319.
Actual installed-client validation and limits are documented in [VALIDATION.md](VALIDATION.md).
