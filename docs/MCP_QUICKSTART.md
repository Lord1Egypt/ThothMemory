# MCP quickstart

**Install first:** download the matching UltraLite executable from the unified
v0.2.2 release when published, then follow [Windows/Linux setup](../README.md).
v0.1.2 contains Full clients only; UltraLite is not an alias for those files.
Do not assume the downloaded executable is already on PATH.

Client behavior was checked with Claude Code 2.1.296, Codex CLI 0.162.1 and
OpenCode 1.17.13. Use a Git checkout that has an `origin` remote: that remote is the
project identity.

## UltraLite (no Docker, no server)

```sh
thothmemory-ultralite init
cd ~/code/your-project
thothmemory-ultralite connect all          # Claude Code, Codex, OpenCode (whichever are installed)
thothmemory-ultralite instructions         # THOTHMEMORY_AGENT_PROTOCOL.md + a reference block
thothmemory-ultralite hooks claude         # optional automatic capture; also: hooks codex, hooks opencode
thothmemory-ultralite doctor
```

Windows PowerShell: use `& "$env:LOCALAPPDATA\Programs\ThothMemory\thothmemory-ultralite.exe"` in place of
`thothmemory-ultralite` (after copying it there, see the README). Command hooks are not
installed on native Windows; the OpenCode plugin and `instructions` are.

Registered clients run `thothmemory-ultralite mcp-stdio --data DIR --token-file DIR/.secrets/<client>_token`;
each session starts its own short-lived process that exits with the client.

## Full (PostgreSQL)

```sh
./setup.sh                                     # or .\start-thothmemory.cmd on Windows
./bin/thothmemory connect all --dir ~/code/your-project
./bin/thothmemory instructions --dir ~/code/your-project
./bin/thothmemory hooks claude --dir ~/code/your-project
./bin/thothmemory doctor
```

## Mini (ThothDock, MariaDB)

```sh
sh ./thothmini.sh install --image "$(cat MINI_IMAGE_DIGEST.txt)" --port 7319
scripts/thothmini.sh clients
M=~/.thothmemory-mini/thothmemory-mini
$M connect all --config ~/.thothmemory-mini/client.json --dir ~/code/your-project
$M instructions --dir ~/code/your-project
$M hooks claude --dir ~/code/your-project --config ~/.thothmemory-mini/client-claude.json
```

## Any other MCP client

Register a stdio server whose command and arguments are:

| Edition | Command | Arguments |
|---|---|---|
| UltraLite | `thothmemory-ultralite` | `mcp-stdio --data DIR --token-file DIR/.secrets/agent_token` |
| Full / Mini | `thothmemory` / `thothmemory-mini` | `bridge --config client.json` |

Use an absolute path to the executable. Tools are named `memory_*`; discovery returns
25 for a read-write credential and only read tools for a read-only one. For HTTP-capable
clients (Full, Mini) the endpoint is `http://127.0.0.1:7319/mcp` with
`Authorization: Bearer <token from a private file>`. UltraLite has no HTTP endpoint.

## Verify

```sh
TM doctor                    # TM = thothmemory-ultralite | ./bin/thothmemory | thothmemory-mini
claude mcp list              # ✔ Connected
opencode mcp list            # ✓ thothmemory connected
codex mcp list               # thothmemory, stdio
```

Registration prompts: Claude Code asks you to approve project servers (`claude`, once);
Codex needs the folder trusted for project config, or use `--scope user`.

## Config file locations

| Client | Project | User |
|---|---|---|
| Claude Code | `.mcp.json`, hooks in `.claude/settings.json` | `~/.claude.json`, `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR`) |
| Codex CLI | `.codex/config.toml`, hooks in `.codex/hooks.json` | `~/.codex/config.toml`, `~/.codex/hooks.json` (or `$CODEX_HOME`) |
| OpenCode | `opencode.json` or `.jsonc`, plugin `.opencode/plugins/thothmemory.js` | `~/.config/opencode/` (or `$XDG_CONFIG_HOME/opencode`) |

## Turn it off

`TM capture set off` stops hooks from recording or printing; stored memory stays. To
remove the server: `claude mcp remove thothmemory`, `codex mcp remove thothmemory`, or delete
the `thothmemory` entry in `opencode.json`.
