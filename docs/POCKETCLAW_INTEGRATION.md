# PocketClaw integration contract (prepared, not integrated)

PocketClaw is **not** modified or built against UltraLite yet; that is a separate phase
needing explicit approval. This document fixes the boundary so that phase is small.

## Shape

PocketClaw's Go core imports one public package and calls it in-process. No subprocess,
no localhost HTTP server, no Docker, no daemon, no background goroutine of ours.

```go
import "github.com/Lord1Egypt/ThothMemory/pkg/memorycore"

memorycore.Init(ctx, dataDir)                 // once: directory, credentials, database
m, _ := memorycore.Open(ctx, dataDir)         // at core start; holds the instance lock
defer m.Close()                               // at core stop; releases the lock
s, _ := m.Session(ctx, tokenFile)             // an authenticated, scoped principal
tools := s.Tools()                            // names, descriptions, JSON Schemas for the model
out, err := s.Call(ctx, "memory_record_idea", argsJSON)   // {"data": …}; sanitized errors
m.Backup(ctx, target); m.Status(ctx)
```

`Call` runs the exact MCP path: argument schema validation, per-credential project
allowlists and read-only scope, secret refusal, evidence gates, idempotency by
`operation_id`, optimistic revisions. An agent runtime exposes `Tools()` to the model as
its function list and forwards the model's tool calls to `Call`. Tool names accept the
`memory_` prefix or not. `TestEmbeddingContract` exercises this, under `-race`.

## Rules the host must respect

- **Data directory**: app-private storage on a local filesystem (never a shared or FUSE
  mount). It holds `memory.db`, `-wal`/`-shm`, `.secrets/` and `capture/`.
- **One process owns it** at a time: `Open` takes a shared advisory lock, so a second
  PocketClaw core and a CLI restore cannot corrupt each other; restore refuses while open.
  Several goroutines may call concurrently (SQLite serializes writers; busy waits are
  bounded to 10 s and honor the call's `context`).
- **Credentials**: `Init` writes random tokens under `.secrets/`. PocketClaw may use one file
  per agent identity or its own store, as long as the token matches the policy in
  `tokens.json`; narrowing a principal to projects or read-only is the same JSON policy.
- **Untrusted content**: results contain agent-written text. Present it to a model as data,
  never as instructions. The server already marks briefings `content_trust`.
- **Lifecycle**: nothing runs between calls. Call `Backup` at your own cadence; a call is
  a consistent online snapshot with an integrity check.
- **Sessions and capture**: hooks and the `lifecycle` command are for external CLIs. An
  embedded runtime does its own equivalent: call `get_project_context` and
  `start_session` at conversation start, `record_*` while working, `checkpoint_session`
  before trimming context, `finish_session` at the end, following the
  [protocol](../THOTHMEMORY_AGENT_PROTOCOL.md).

## Build facts

CGO-free, so `GOOS=android GOARCH=arm64 go build` works: the headless executable built
(13.6 MB, `ELF 64-bit … ARM aarch64`) but **was not run on a device**. `pkg/memorycore` itself
was compiled and tested on Linux amd64 only. Android's `sun_path`, SELinux and `fsync`
behavior, `flock` on the app's data partition, and the Go runtime inside the app
(gomobile/JNI or a native shared library) are unverified. Dependencies added: pure-Go
SQLite (`modernc.org/sqlite`) and its runtime packages; no HTTP stack is linked by this
package.

## Open decisions for the approval phase

1. How the model-facing tool list reaches PocketClaw's existing tool/skill layer.
2. Where the credential file lives under Android's backup rules (exclude `.secrets/` from
   auto-backup unless the owner wants it).
3. Whether the phone also exposes memory to other apps (it should not by default); sharing
   with desktop agents uses [export/import](PORTABILITY.md), not a network port.
4. A device acceptance run of `TestEmbeddingContract`'s scenarios inside the app.
