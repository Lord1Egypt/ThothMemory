# Moving memory between editions

`export` writes one project to a backend-neutral JSON document; `import` applies it to
any edition. It is a transfer you run, **not synchronization**: there is no live link and
no second source of truth.

```sh
# UltraLite
thothmemory-ultralite export PROJECT_ID > project.thothmemory.json
thothmemory-ultralite import < project.thothmemory.json
# Full (owner connection through the migrate service; -T keeps stdout clean)
docker compose run --rm -T migrate export PROJECT_ID > project.thothmemory.json
docker compose run --rm -T migrate import < project.thothmemory.json
# Mini
docker exec thothmemory-mini thothmemory-mini export PROJECT_ID > project.thothmemory.json
docker exec -i thothmemory-mini thothmemory-mini import < project.thothmemory.json
```

## Format (version 1)

A header (`format`, `format_version`, `export_id`, `exported_at`, source product, edition,
version and schema, `project_id`), per-table row `counts`, a `payload_sha256` over the
canonical rows, and the payload: `projects`, `tasks`, `task_dependencies`, `sessions`,
`evidence`, `attestations`, `records` (decisions, issues, observations, handoffs, ideas,
notes), `events`, `operations`. Timestamps are UTC microseconds; identifiers are preserved.
Search text and per-installation budget counters are rebuilt, not exported. Credentials
and principals' tokens are never exported.

## Import rules

1. The document must parse, name a known format version, match its counts and checksum,
   hold exactly one project, stay under 512 MiB and 2,000,000 rows, and every in-document
   reference must resolve. Anything else is rejected before the database is touched.
2. One transaction. New rows are inserted with their original IDs, times and authors.
3. Importing the same document again is a no-op (`already_imported: true`).
4. Existing history rows (events, records, evidence, attestations) must be identical;
   a difference refuses the import.
5. Tasks and sessions are mutable. If the target's row appears in the incoming history,
   the target is **fast-forwarded**. If the incoming row is an older state found in the
   target's own history, the target is kept (`kept_newer_target`). Otherwise the two
   edited the same entity independently and the import is refused with nothing written.
6. Operation records travel too, so a retry of an earlier write still returns its
   original result after the move.
7. A provenance row records each import (append-only).

## Tested

All three pairs, both directions, with Arabic text, supersession chains, attestations and
dependencies: Full↔Mini, Full↔UltraLite, Mini↔UltraLite (`tests/portable_test.go`).
Checked: identical reads afterwards (context, tasks, handoff, activity, evidence, task
detail), repeat import, continuing work on the target and fast-forwarding back, an older
export not overwriting newer state, a divergent edit refused with no events written,
and tampered or inconsistent documents rejected. UltraLite→UltraLite through the real
CLI is in `TestUltraLiteNativeCLIAndStdio`.

Through the real entry points (2026-10-13): a project exported from the PostgreSQL
Compose stack (`migrate export`), imported into a Mini container with `docker exec`, and
exported again produced the **same `payload_sha256` and row counts**, i.e. the canonical
data survived the round trip unchanged.

Not tested: documents near the size limits, exports from the v0.1.1 production database
(only a disposable clone was migrated, not exported), and cross-edition search ranking
(it differs; see [editions](EDITIONS.md)).
