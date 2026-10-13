# Real-client handoff, 2026-10-13 (UltraLite stdio, disposable data directory)

REAL here means the installed client binaries and their configured model accounts
ran the session. No fixture or simulated actor is involved in this file.

| Step | Client | What ran | Result |
|---|---|---|---|
| Connect | Claude Code 2.1.296 | `claude mcp list` after `connect claude --scope user` (isolated `CLAUDE_CONFIG_DIR`) | `thothmemory … ✔ Connected` |
| Connect | Claude Code | Same, project scope (`.mcp.json`) | `⏸ Pending approval (run claude to approve)`: Claude requires a human to approve project servers; this is its trust model, not a failure |
| Connect | OpenCode 1.17.13 | `opencode mcp list` (project `opencode.json`) | `✓ thothmemory connected` |
| Connect | Codex 0.162.1 | `codex mcp list/get` after `connect codex --scope user` (isolated `CODEX_HOME`) | Registered, `transport: stdio`; project-level `.codex/config.toml` is ignored by Codex until the folder is trusted |
| Session 1 | Codex 0.162.1, real model | `codex exec` in a scratch Git repo whose `AGENTS.md` only referenced the protocol; MCP server passed with `-c mcp_servers.…` (no Codex config edited); prompt: *"Maybe we can make a SQLite version for PocketClaw."* | Calls: `list_projects`, `get_health`, `create_project`, `get_project_context`, `start_session` (first attempt failed, retried), **`record_idea`**, `finish_session`. Stored idea: *"User suggested exploring a SQLite-based version of PocketClaw. This remains a proposed idea; no implementation task or architectural decision has been authorized."* Handoff began `deferred, unverified:` |
| Session 2 | Claude Code 2.1.296, real model, no shared chat | `claude -p --strict-mcp-config --mcp-config <temp file>` (user config untouched), asked what ideas exist, who recorded them and what the last handoff said, then to record a note | Correctly reported the idea (id prefix `e25007dc`), that agent `codex` recorded it, and the handoff's "deferred, unverified … tests not run". Recorded note `1c4dd433` as agent `claude-code` under principal `claude`, plus its own session and handoff |
| Limits seen | | | Claude could not run `git remote get-url origin` without an approval prompt in `-p` mode and said so; it used the only project in memory. OpenCode was **not** run with a real model (no provider exercised); its plumbing is verified through `opencode mcp list` and the deterministic fixture. Codex hooks and Claude hooks were **not** fired by the real clients in this run (hook command strings were executed through the shell by `TestAutomaticMemoryEndToEnd`) |

The scratch repository, data directory and tokens were disposable and live under the
session scratchpad. Tokens were never printed. Reproduce with
`scripts/real-client-handoff.sh`.

## Which lifecycle events the real clients fired (same day, same disposable setup)

| Client | Method | Events observed in memory |
|---|---|---|
| Claude Code 2.1.296 | `hooks claude --yes`, `capture set prompts --yes`, then `claude -p "Reply with the single word: ok"` in a scratch repo | `SessionStart` (project registered from `origin`, session started), `UserPromptSubmit` (the prompt stored as note "User request"), `Stop` (idle metadata), `SessionEnd` (checkpoint "session ended"). `PreCompact` did not fire because no compaction occurred. |
| Codex 0.162.1 | `hooks codex --yes`, `codex exec --enable hooks --dangerously-bypass-hook-trust` in scratch repos under `/tmp` and under the home directory | **No hook event reached memory.** Codex ignored the `projects.<path>.trust_level` override ("is ignored") and requires hook review in its own UI; the cause was not isolated. Real Codex hook firing is therefore **not verified**. The generated `.codex/hooks.json` commands do work when run through the shell (see `TestAutomaticMemoryEndToEnd`), and Codex's MCP-side behavior is verified above. |
| OpenCode 1.17.13 | `opencode mcp list` only | Plugin lifecycle callbacks are exercised by `tests/opencode-lifecycle.mjs` (fixture), not by a real model session here. |
