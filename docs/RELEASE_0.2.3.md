# ThothMemory v0.2.3 — Compose installation safety (candidate, not published)

Components: **Full 0.2.3**, **Mini 0.1.3**, **UltraLite 0.1.0**. v0.2.0, v0.2.1, v0.2.2 and every earlier
image digest stay as published. Mini 0.1.3 and the rebuilt UltraLite behave exactly like 0.1.2 and 0.1.0; Mini is
re-versioned only because the release workflow tags every component of a unified release.

## What was wrong

The Full `compose.yaml` hard-coded the project name `thothmemory` and used the default volume, so a fresh
`docker compose up` on a machine that already ran an older Full installation resolved to that installation's volume
`thothmemory_memory` and could initialize or migrate it. Nothing in the published releases did this to the owner's
data, but the risk was real.

## What changed

- Fresh installs get their own project name and data volume (`thothmemory-<id>` / `thothmemory-<id>-data`),
  written to `.env` and created by `thothmemory init-config` (so by `./setup.sh`). Repeat runs are idempotent.
- The volume is `external`: Compose never creates, recreates ("data will be lost") or deletes it, not even with `down -v`.
- A `guard` service runs before the database and migration. It refuses any volume that holds data this installation
  did not create, or that is marked for another installation or project, with a clear message. This also protects direct
  `docker compose up`, `-p`, `COMPOSE_PROJECT_NAME` and `THOTH_VOLUME` overrides. A bare `docker compose up` in a
  folder without `.env` fails before creating anything.
- Continuing an older installation is deliberate: `./bin/thothmemory adopt-volume VOLUME --confirm VOLUME`
  (docs/UPGRADE.md). It runs no migration. Older folders are never given a volume automatically.
- A Full build now refuses to migrate a database written by a newer release.
- Docs separate fresh install, upgrade and restore.

## Verified

`scripts/compose-isolation-dind.sh` (CI job `compose-isolation`) builds a real legacy installation from the published
v0.2.1 image and compose file, with the production names, seeds it through MCP, stops it, and shows that bare, fresh,
repeated and misconfigured invocations leave its volume byte-identical and start nothing from it, then upgrades it
v0.2.1 → v0.2.2 → this build with identical data, backup, restore into an isolated database and a refused downgrade.

## Limits

Forcing `-p thothmemory` can make Compose replace the old *stopped* `app` container with a never-started one; the
data volume and database container are untouched and the guard refuses to start anything. Already-released builds
(including 0.1.2) cannot refuse a newer schema. Mini and UltraLite are unchanged; Android Mini remains a preview.
