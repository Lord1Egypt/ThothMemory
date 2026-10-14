# Reliable handoff between agents

Agents share one project memory, found through the repository's canonical `origin`
(never the folder name). Handing over means leaving enough behind that a stranger can
resume without the chat.

## What the outgoing agent leaves

`memory_checkpoint_session` as it goes and `memory_finish_session` at the end. A good
handoff states, using the protocol's outcome words *completed*, *partially completed*,
*failed*, *blocked*, *deferred*, *unverified*: the objective, what is done, what is in
progress, the git revision, uncommitted or unverified changes, attempts that failed and
why, decisions with reasons, evidence IDs, and one next action. Ideas and notes recorded
along the way stay separate and searchable.

## What the incoming agent does

1. Registers/finds the project from `origin` and loads `memory_get_project_context` with
   the current HEAD, then `memory_get_latest_handoff`.
2. Treats everything as *claims*: `stale: true` records were made at another commit,
   reported tests are not verified, and an agent's "done" is checked against the code.
3. Resumes the unfinished session if a start hook printed one, otherwise starts its own.
4. Tells you what it found before acting.

## Interrupted agents

If an agent died, its session stays *unfinished* and its last checkpoint is in the
briefing. The next agent sees it, continues from the checkpoint, and records what it
found. Nobody has to guess whether the earlier work completed.

## Evidence of this working

| Handoff | How it was tested | Where |
|---|---|---|
| Codex → Claude Code, real models, no shared chat | Codex recorded a PROPOSED idea and a "deferred, unverified" handoff; Claude retrieved both, named the author, and added a note | [evidence](audits/evidence/real-client-handoff.md) |
| Claude → Codex → OpenCode, MCP stdio identities, no model | Briefing completeness, blocked task, failed attempt, checkpoint, partial handoff, search for the other agent's note, unauthorized project, SIGKILL | `TestAutomaticMemoryEndToEnd` |
| Four-session engineering scenario on every edition | Shared contract suite | `internal/contract` |

Not yet shown: a real-model OpenCode session in the chain, and a handoff across
editions mid-session (use [export/import](PORTABILITY.md); it is a transfer, not sync).
