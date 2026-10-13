# Your first ThothMemory project

[English overview](../README.md) · [العربية](README_AR.md)

Start with **UltraLite** unless you need a browser dashboard. It needs one
download, no Docker or Go compiler. Install Claude Code, Codex CLI or OpenCode
separately; ThothMemory registers installed clients, not their model accounts.

## 1. Download and install

The unified [v0.2.0 release](https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.2.0)
is currently pending the launch gates; v0.1.2 has no UltraLite assets.
When available, choose the file matching your OS/CPU and download SHA256SUMS.
Windows Intel/AMD uses `thothmemory-ultralite-windows-amd64.exe`; Linux Intel/AMD
uses `thothmemory-ultralite-linux-amd64`; Linux ARM64 uses `...-linux-arm64`.

Windows, PowerShell opened in Downloads:

```powershell
Get-FileHash .\thothmemory-ultralite-windows-amd64.exe -Algorithm SHA256
$bin = "$env:LOCALAPPDATA\Programs\ThothMemory"
New-Item -ItemType Directory -Force $bin | Out-Null
Copy-Item .\thothmemory-ultralite-windows-amd64.exe "$bin\thothmemory-ultralite.exe"
& "$bin\thothmemory-ultralite.exe" init
& "$bin\thothmemory-ultralite.exe" doctor
```

Compare the hash with the matching SHA256SUMS entry before execution. Linux:

```sh
sha256sum thothmemory-ultralite-linux-amd64
mkdir -p ~/.local/bin
install -m 0755 thothmemory-ultralite-linux-amd64 ~/.local/bin/thothmemory-ultralite
~/.local/bin/thothmemory-ultralite init
~/.local/bin/thothmemory-ultralite doctor
```

Substitute `linux-arm64` in the install command on ARM. Expected doctor output
includes owner-only permissions, database integrity, MCP stdio handshake and
child-process exit. It does not contact your model provider.

## 2. Connect in a project

Open a terminal in a Git project that has a known `origin` remote. On Windows
use `Set-Location C:\your\project`; keep `$bin` from above in the same PowerShell
session. Use `& "$bin\thothmemory-ultralite.exe"` for the commands below.

```sh
~/.local/bin/thothmemory-ultralite connect all
~/.local/bin/thothmemory-ultralite instructions
```

The first command previews configuration changes and asks consent. The second
adds a protocol reference to AGENTS.md and CLAUDE.md while preserving other text.
Choose `connect claude`, `connect codex` or `connect opencode` for just one client.
Default scope is this folder. `connect all --scope user` registers for every
project, but project instructions should still be installed or referenced.
Do not move the executable after registration; rerun connect if its path changes.

Restart the assistant and accept its normal trust prompts. `claude mcp list`,
`codex mcp list` or `opencode mcp list` should include thothmemory. OpenCode's
tested compatibility is legacy 1.17.13; do not assume V2 compatibility.

## 3. Save one idea and read it back

Tell the first agent: “Maybe we can make a SQLite version for PocketClaw. Record
this as a proposed idea for this project, then checkpoint the session.” Ask it
to report the project and record IDs, and any failed writes.

Close it, start another connected agent in the same project and ask: “Load the
project context and latest handoff. What ideas were recorded for PocketClaw?”
Real Codex→Claude retrieval was demonstrated; OpenCode's third real-model step
has not been demonstrated. Verify storage directly:

```sh
~/.local/bin/thothmemory-ultralite projects
~/.local/bin/thothmemory-ultralite search PROJECT_ID PocketClaw
```

Use the returned project UUID in place of PROJECT_ID. If no result appears,
check that both clients launch the same executable/data directory and that the
agent made a real write. Registration by itself is not a write.

## 4. Optional automatic capture

Linux/WSL Claude Code:

```sh
~/.local/bin/thothmemory-ultralite hooks claude
~/.local/bin/thothmemory-ultralite capture status
~/.local/bin/thothmemory-ultralite capture set metadata
```

This captures lifecycle metadata and loads context. The model still needs the
protocol to write meaningful ideas, decisions and handoffs. Optional bounded
request capture: `capture set prompts --yes`; stop capture: `capture set off`.
No full transcript collection. Codex semantic writes work; hook capture remains
unverified and requires project/hook trust review. Native Windows command hooks
are unsupported. [Full details and retry queue](AUTOMATIC_MEMORY.md).

## 5. Keep a backup

```sh
~/.local/bin/thothmemory-ultralite backup
```

Copy the reported `.db` file and its `.sha256` sidecar to independent storage.
To restore, close connected clients first, then:

```sh
~/.local/bin/thothmemory-ultralite restore /path/to/backup.db --confirm
~/.local/bin/thothmemory-ultralite doctor
```

Default data: Linux `~/.local/share/thothmemory` (or XDG_DATA_HOME), Windows
`%LOCALAPPDATA%\ThothMemory`. Replace only the executable to update, after backup.
Keep `.secrets` private. Do not put an active SQLite database on a network share.

## Full with Docker

Choose Full for the complete browser dashboard. Install Docker Desktop with
Linux containers on Windows or Docker Engine and Compose on Linux. Download
the v0.2.0 source package, extract it into a new installation folder, then:

```sh
./setup.sh
./bin/thothmemory doctor
./bin/thothmemory connect all --dir /path/to/project
./bin/thothmemory instructions --dir /path/to/project
./bin/thothmemory backup /path/to/independent-backup.dump
```

Windows: `.\start-thothmemory.cmd`; CLI `.\bin\thothmemory.exe`. Setup starts
PostgreSQL and the MCP/dashboard service. Open http://127.0.0.1:7319 and use
`.secrets/read_token` to unlock. Tasks, sessions, decisions, evidence and timeline
are browseable. Git revision is last observed, not live-polled current state.
Use `docker compose stop` to stop and `docker compose start` to resume. Memory
persists in the `memory` named volume within the Compose project. Never delete
it as routine maintenance. [Backup/restore](BACKUP_RESTORE.md).

## Mini with ThothDock

See [the Mini walkthrough](MINI.md). It uses one MariaDB container and no Compose.
The final published digest/device test is separate from the earlier Samsung pass.

## If something fails

Run `doctor`; preserve errors rather than claiming a save succeeded. Hook writes
can be inspected with `capture pending` and retried with `capture retry`. Agent
MCP writes need explicit retries with the same operation ID. The next session
can load unfinished sessions and their last checkpoint. [Troubleshooting](TROUBLESHOOTING.md).
