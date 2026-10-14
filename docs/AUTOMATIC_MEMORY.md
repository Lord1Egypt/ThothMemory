# Automatic memory

The goal: you should not have to tell an agent to remember. Two things make that
happen, and neither is a promise written in a prompt.

1. **Lifecycle hooks** the client runs by itself (session start, your prompt,
   compaction, idle, end). They record facts and bring context in.
2. **Agent writes** over MCP. Hooks cannot know what a bug's cause was or why a
   decision was made; only the agent can say. The
   [protocol](../THOTHMEMORY_AGENT_PROTOCOL.md) tells it exactly when and with which tool.

## What is captured, and what proves it

| Event | Source | What is stored | Evidence in this repository |
|---|---|---|---|
| Session start | `SessionStart` hook / OpenCode plugin | Project registered from canonical `origin` (once), one durable session started or resumed per agent, untrusted context printed for the agent | Real Claude Code run; `TestAutomaticMemoryEndToEnd` |
| Your request | `UserPromptSubmit` hook, **opt-in** | Note "User request" (≤2,000 characters), linked to the session; duplicates and one-word prompts skipped; credential-looking prompts skipped | Real Claude Code run; end-to-end test |
| Before compaction | `PreCompact` hook | Checkpoint on the same session: metadata and a reminder, never a made-up summary | **Real Claude Code** (`claude -p --continue "/compact"`) and the end-to-end test |
| Idle (end of a turn) | `Stop` hook / OpenCode `session.idle` | One lifecycle event, at most every 10 minutes or when HEAD changed | Real Claude Code run; end-to-end test |
| Error | OpenCode `session.error` | Lifecycle event | Plugin fixture only |
| Session end | `SessionEnd` hook | Checkpoint "session ended"; the session stays *unfinished* unless the agent finished it | Real Claude Code run |
| Ideas, notes, decisions, issues, tasks, tests, checkpoints, handoffs | The agent via `memory_record_*` and friends | The agent's concise records | Real Codex run and real Claude Code run |

Per client:

- **Claude Code** exposes every event above; all five fired from the real client.
- **Codex** (0.162.1) has the same hook model (`hooks` is a stable feature, `hooks.json`, per-hook
  `trusted_hash`), but **project hooks only load once you trust the folder and review the hooks
  in Codex itself**. Rechecked installed CLI 0.162.1: `hooks` is stable and enabled.
  [Official hook documentation](https://learn.chatgpt.com/docs/hooks) confirms that
  unreviewed definitions are skipped until approved through `/hooks`; enabling the
  feature alone does not approve commands. In our runs Codex emitted no hook activity at all, and its `-c projects.<path>`
  override is ignored by design, so we could not verify real firing without bypassing its trust
  control, which we will not do. To enable: run `codex` interactively in the project, accept the
  folder trust, open `/hooks`, review and approve the ThothMemory commands, then check
  `TM capture status` and `TM projects` after a short session. Until you have seen it work, rely
  on the fallback that always works: **instructions + the agent's own MCP writes** (verified with
  real Codex).
- **OpenCode** has a plugin with context, idle, error and compaction callbacks but no prompt
  event we could rely on. Its connection is verified (`opencode mcp list`); a real-model session
  was not exercised.

Where a client gives no event, the protocol makes the agent persist that information explicitly.

## Capture modes

```sh
TM capture status
TM capture set metadata       # default after installing hooks: facts only
TM capture set prompts --yes  # also store your own requests as notes
TM capture set off            # hooks record and print nothing; memory is kept
```

The mode is a small `capture.json` beside your data (UltraLite: `<data>/capture/`; Full and
Mini: `.capture/` next to the client config), shared by all clients of that install.
Installing hooks is your consent to `metadata`; `prompts` needs `--yes`. Full
transcripts are **not** stored: the engine never reads a client's transcript file.

## Privacy and secrets

- Writes containing private keys, AWS/GitHub/OpenAI/Anthropic/Slack tokens, JWTs, or
  `password=`/`api_key=`-style assignments are refused by the server on every write
  path and **nothing is stored** (history is append-only, so refusal is the only safe
  option). The agent is told to describe the secret without its value. This is a
  pattern screen, not a guarantee: do not paste secrets into agents.
- Prompts that look like credentials are skipped by the hook before any write.
- Memory is untrusted data to the agent: it is shown as such and never executed.
- Everything is local and owner-only (`0700`/`0600`, Windows protected ACLs).

## When memory is unreachable

Hooks never block your agent. If the database is busy or a service is down, a hook
write is appended to a bounded, owner-only queue (`pending.jsonl`: at most 200 entries
and 512 KiB) together with its operation ID. The next hook, or `TM capture retry`,
replays it **in order and stops at the first transient failure**. Replays reuse the
original operation ID, so a write that actually succeeded earlier returns its original
result instead of creating a second record (tested: a delivered write replayed again
produced exactly one record). A write the memory refuses permanently is moved to
`rejected.jsonl` with the reason, never silently dropped and never retried forever.
Agent MCP writes are not queued; the protocol tells agents to say so when a write fails
and to retry with the same operation ID.

## Keeping memory useful

- Ideas are labelled `idea` (PROPOSED), notes `note`; a decision that `supersedes` an idea
  adopts it and removes it from the open list while keeping it searchable.
- The same title and words in a project return the existing record (`duplicate: true`)
  and add no history. Superseded decisions disappear from the briefing but stay in
  history and search, flagged `superseded`.
- The briefing is bounded: 12 tasks, 8 decisions/issues, 8 ideas/notes, 8 evidence, 4
  sessions and checkpoints, 1 handoff; long text is cut at 1,200 characters with a pointer
  to retrieve the full record. Search is paginated (≤100 per page).
- Hooks do not record tool output, unchanged status, or repeated summaries.

## Limits, stated plainly

- A killed process (SIGKILL, power loss) fires no hook. Recovery relies on checkpoints
  already written and on the *unfinished session* the next start reports.
- Hooks cannot make a model write a good handoff. A hook-written marker is labelled
  metadata and never claims completion, success or verification.
- Claude Code in `-p` (non-interactive) mode may be unable to run `git` commands without
  approval; project detection by hooks does not depend on the model.
- Retention is not automated: history is append-only; only the per-project event budget
  (100,000 by default) bounds growth. Export and archive projects you no longer need.
