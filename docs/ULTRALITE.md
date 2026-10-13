# ThothMemory UltraLite

UltraLite is persistent shared memory for coding agents in **one native
executable and one SQLite file**. There is no Docker, no database server, no web
dashboard and no network listener. Each agent launches the executable over MCP
stdio for its session; it exits when the agent disconnects. Owners manage
memory with the same executable's CLI.

Platforms built without CGO: Windows x64, Linux x64, Linux arm64 (static), macOS
arm64 and Android arm64 (a PIE executable). Verified so far: Linux x64 (all
tests), Windows x64 (native init, doctor and ACLs on Windows 11 through WSL
interop), Linux arm64 under QEMU (init and doctor). CI adds native Linux arm64
and Windows runners. macOS and Android executables are built but not executed.

## Install

Linux (no root):

```sh
install -m 0755 thothmemory-ultralite-linux-amd64 ~/.local/bin/thothmemory-ultralite
thothmemory-ultralite init
thothmemory-ultralite connect all          # shows the files it will change, then asks
thothmemory-ultralite doctor
```

Windows PowerShell (no administrator):

```powershell
$bin = "$env:LOCALAPPDATA\Programs\ThothMemory"
New-Item -ItemType Directory -Force $bin | Out-Null
Copy-Item .\thothmemory-ultralite-windows-amd64.exe "$bin\thothmemory-ultralite.exe"
& "$bin\thothmemory-ultralite.exe" init
& "$bin\thothmemory-ultralite.exe" connect all
& "$bin\thothmemory-ultralite.exe" doctor
```

`connect` registers Claude Code, Codex and OpenCode at project scope in the
current directory (`--scope user` for all projects, `--dir PATH` for another
project). It preserves every other setting, refuses to overwrite a different
existing `thothmemory` entry, backs up files it changes and asks for consent;
`--yes` is explicit consent for automation. Registered clients run:

```text
thothmemory-ultralite mcp-stdio --data <data dir> --token-file <data dir>/.secrets/<client>_token
```

Restart the agents afterwards. `instructions` adds the shared memory protocol to
`AGENTS.md` and `CLAUDE.md`.

## Data directory

| OS | Default | Contents |
|---|---|---|
| Linux | `$XDG_DATA_HOME/thothmemory` or `~/.local/share/thothmemory` | `memory.db`, `.secrets/`, `backups/` |
| Windows | `%LOCALAPPDATA%\ThothMemory` | same |
| macOS | `~/Library/Application Support/ThothMemory` | same |

Override with `--data DIR` or `THOTH_HOME`. Keep the directory on a local disk:
SQLite's locking is unreliable on network shares. Only `init` creates a
database; every other command refuses a directory without one, so a typo never
starts an empty memory.

## Commands

| Command | Purpose |
|---|---|
| `init` | Create directory, credentials and database. Safe to repeat. |
| `doctor` | Check owner-only permissions, integrity, foreign keys, and run a real MCP stdio session through a child process; confirms the child exited. |
| `mcp-stdio --token-file F` | What clients launch. Never run by hand. |
| `projects`, `brief P [--head SHA]`, `search P QUERY`, `tasks P [--state S]`, `timeline P`, `call TOOL [JSON]` | Read-only inspection as the local owner, through the same validation as MCP. |
| `status` | Engine, schema, file, WAL and free-page sizes, counts, principals. |
| `backup [FILE]` | Online snapshot (`VACUUM INTO`) while agents keep working; integrity-checked; SHA-256 sidecar. Default `backups/memory-<UTC>.db`. |
| `restore FILE [--confirm]` | Verifies checksum, integrity and schema first. Refuses while any agent session is open. Keeps the replaced file as `memory.db.before-restore-<UTC>`. |
| `export P > FILE`, `import < FILE` | Portable project transfer with Full and Mini ([format](PORTABILITY.md)). |
| `attest P EVIDENCE OPERATION STATEMENT` | Owner review of reported evidence. |
| `permissions PRINCIPAL --projects UUID[,UUID] / --read-only / --write / --revoke` | Narrow or revoke a client. Applies at its next session. |

## How SQLite is used

- Pure Go driver `modernc.org/sqlite` v1.60.1 (SQLite 3.53.4), no CGO.
- STRICT tables with the same CHECK constraints, composite foreign keys and
  append-only triggers as the other editions; foreign keys enforced on every
  connection; `trusted_schema=OFF`.
- WAL journal, `synchronous=FULL` (every commit is fsynced), transactions begin
  `IMMEDIATE` so a writer never deadlocks upgrading a read lock.
- SQLite waits at most 250 ms per attempt for the write lock; a bounded retry
  repeats busy attempts until the request's own deadline (cap 10 s), because
  SQLite's internal wait cannot observe a cancelled request.
- Migrations run in one transaction (SQLite DDL is transactional).
- Search stores NFC-lowercased text and matches substrings with `INSTR`, so
  Arabic, identifiers and commit prefixes work without tokenizer limits. FTS5 was
  evaluated and not used: its tokenizers miss substrings and short Arabic words,
  and the measured scan cost at 10,000 events is in [benchmarks](BENCHMARKS.md).

Verified by tests: six separate OS processes racing one task revision (exactly
one wins, no write lost), SIGKILL during continuous writes (database intact and
every event has its operation row), an online backup taken during writes
(consistent snapshot), restore refusal while in use, and every shared contract
scenario.

## Security model

The trust boundary is the operating-system account. Memory and credentials are
owner-only (Unix 0700/0600; Windows protected owner-only DACLs). Each client
presents its own token from a file; the token selects a principal with its
project allowlist and read/write scope, so stdio does not bypass authorization
and a revoked token is refused at the next session. A process running as the
same OS user can read the database file directly; UltraLite does not defend
against the owner's own processes. Stored memory is untrusted data: it is never
executed, and clients are told it is not instructions. There is no listener to
attack remotely.

## Upgrade

Replace the executable. On first use the new version applies pending migrations
to the existing file atomically; credentials and memory are kept. Take a
`backup` first.

## Not provided

No dashboard, HTTP endpoint, lifecycle hooks (which need a running service), or
English stemming dictionaries. Use `export`/`import` to move projects to Full
when you want the dashboard.
