# Agent compatibility — audited 2026-10-12

All tests used isolated homes and a synthetic Git repository. The owner's normal
profiles were not edited. Client-side stdio bridge uses authenticated loopback
Streamable HTTP to the shared official Go SDK server; no provider-specific DB API.

| Capability | Claude Code 2.1.296 | Codex CLI 0.162.1 | OpenCode 1.17.13 legacy | OpenCode V2 |
|---|---|---|---|---|
| Project and global registration | PASS: isolated registration + SDK doctor | PASS: isolated registration + SDK doctor | PASS: existing settings preserved | Schema merge tested; guarded experimental option only |
| Actual client connection | PASS: `claude mcp list` reports Connected | PASS: `codex exec` discovers server and calls memory_get_health | PASS: actual `opencode run` | FAIL: normal run rejects permissions/providers |
| Authentication / discovery | SDK handshake, 23 tools; scoped/read-only/401 tests | Actual authenticated health call plus SDK tests | Actual authenticated tool discovery/calls | Backend doctor cannot establish runtime support |
| Actual CLI write and retrieval | Not exercised with a Claude provider | Health call only in genuine exec; writes tested with real SDK actor | PASS: create_project, create_task, get_task through deterministic local provider fixture | Not supported by tested normal runtime |
| Instructions / context | Generated AGENTS/CLAUDE guidance, direct lifecycle adapter tests | Genuine SessionStart/Stop hooks executed | Actual legacy plugin loaded; direct system/compaction callback tests | Plugin loading not verified |
| Semantic checkpoint/handoff | Simulated Claude actor over real MCP/PG | Simulated Codex actor over real MCP/PG | Simulated OpenCode actor over real MCP/PG | Not claimed |
| Hooks / failure recovery | Existing Stop hook preserved; generated SessionStart/PreCompact/Stop/SessionEnd. Actual callback firing in an interactive Claude session not verified | Enable hooks explicitly, trust repository and review commands. Isolated genuine exec used test-only hook trust bypass | Idle metadata observed from actual run; error/compaction directly invoked through fixture | No normal runtime acceptance |

The genuine OpenCode run used `tests/fixture_model.py` with local deterministic
OpenAI-compatible responses. This proves client tool plumbing, not model reasoning or
compatibility with every provider. The genuine Codex run used an existing authenticated
account through a protected temporary auth copy that was removed after testing.
No commercial-provider Claude coding session was run during this audit.

## Executed engineering scenario

`TestAuditRealisticFourSessionEngineeringHandoff` in [the executed suite](evidence/integration-tests.txt)
uses distinct scoped tokens for Claude → Codex → OpenCode → Claude:

1. Claude reads context, starts a session, creates a task, decision, observation,
   failed attempt, failed evidence, checkpoint and final handoff.
2. Codex retrieves the failed attempt and handoff, chooses a different simulated
   remedy, compares task revision, records a new passing reported result, updates
   task and writes a handoff. No unsupported owner verification is asserted.
3. OpenCode retrieves the current task/revision and records checkpoint/handoff.
4. Claude retrieves final task state, earlier failure and remaining work.

The separate recovery test disconnects without finish: checkpoint stays readable,
finished_at stays null and context reports incomplete. Other tests cover duplicate
retry, stale task writes, claim conflict/expiry, canceled in-flight DB transaction,
missing final hook, stale Git revision and server/DB restart persistence.

## Operator requirements

- Approve the repository and MCP configuration in each client's normal trust UI.
  Registration is not a substitute for trust approval or session instructions.
- Install project instructions; opt into hooks separately. Native Windows command
  hooks remain unsupported; use instructions and explicit checkpoints there.
- Codex command hooks require `--enable hooks`. Do not use audit trust bypass for
  ordinary projects. Start/context hooks provide data; they cannot force a model to
  record engineering progress or write an accurate semantic final handoff.
- For OpenCode 1.17.13 use the tested legacy configuration. Do not remove unrelated
  V2 settings automatically. `--allow-unverified-v2` is only for independently
  validated runtimes and is not an acceptance PASS.
- Custom CODEX_HOME, CLAUDE_CONFIG_DIR and XDG_CONFIG_HOME are honored by registration,
  instruction and lifecycle installation. User scope shows targets and asks consent.

Official references: [Claude MCP](https://code.claude.com/docs/en/mcp),
[Claude hooks](https://code.claude.com/docs/en/hooks),
[Codex MCP](https://developers.openai.com/codex/mcp/),
[Codex hooks](https://developers.openai.com/codex/hooks/),
[OpenCode rules](https://opencode.ai/docs/rules/),
[OpenCode plugins](https://opencode.ai/docs/plugins/).

Failed OpenCode lifecycle calls now emit a sanitized `client.app.log` warning with
stderr fallback. A missing executable callback was reproduced silently failing on
baseline; [the regression](evidence/plugin-failure-after.txt) verifies visible unsynced
reporting without credential/config/error details. [Actual client excerpts](evidence/client-results.txt)
and [native Windows acceptance](evidence/windows-acceptance.txt) and [native CLI suite](evidence/windows-unit-tests.txt) preserve precise scope.

## Next-generation update, 2026-10-13 (installed: Claude Code 2.1.296, Codex 0.162.1, OpenCode 1.17.13)

| Capability | Claude Code | Codex CLI | OpenCode (legacy) |
|---|---|---|---|
| MCP over stdio to UltraLite | REAL: `claude mcp list` ✔ Connected (user scope; project scope waits for human approval) | Registered, stdio; project config needs folder trust | REAL: `opencode mcp list` ✓ connected |
| Real-model memory write from `AGENTS.md` reference only | REAL: added a note under its own credential | REAL: recorded a PROPOSED idea and a "deferred, unverified" handoff | not run |
| Real cross-agent retrieval | REAL: retrieved Codex's idea and handoff with no shared chat | n/a (author) | not run |
| `SessionStart`, `UserPromptSubmit`, `Stop`, `SessionEnd` into memory | REAL, all four | **Not verified**: hooks need folder trust and hook review in Codex's UI; no events observed | plugin fixture only |
| `PreCompact` into memory | REAL (`/compact`) | not verified | plugin fixture only |
| Windows command hooks | unsupported (use instructions and MCP writes) | unsupported | plugin works |
Evidence: [real-client-handoff.md](evidence/real-client-handoff.md).
