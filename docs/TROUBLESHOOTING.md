# Troubleshooting

- **Docker unavailable:** start Docker Desktop (Linux containers) or Docker Engine.
  WSL2 requires integration enabled for the distribution; Docker Compose is required.
- **"no data volume is selected" / "no isolated project name":** this folder was not set up
  by this release. In a new folder run `./setup.sh`; to continue an older installation follow
  [Upgrade an existing installation](UPGRADE.md) (`adopt-volume`).
- **"REFUSING: data volume ... already contains data that this installation did not create":**
  the safety guard stopped before anything was started or migrated. Choose a new empty
  volume (delete the `THOTH_VOLUME` and project lines from `.env` only in a **new** folder)
  or adopt the volume deliberately. Nothing was changed.
- **I used `-p thothmemory` and my old stopped `app` container was replaced:** this is a known limit
  (see [Upgrade](UPGRADE.md)). Data is not affected; recreate it with `docker compose up -d` after a successful
  `adopt-volume` upgrade.
- **Port conflict:** setup reports the Compose bind failure. Choose a free THOTH_PORT
  in .env and match the endpoint in client*.json. No unrelated process is terminated.
- **Permission denied reading secrets:** .secrets must remain owner-only while files
  mounted into containers need read access for UID 65532. Rerun setup for normal modes.
  On Windows verify Docker Desktop can access the installation directory.
- **401 / failed handshake:** run bin/thothmemory doctor; check client token_file,
  endpoint, tokens.json and container health. Restart after changing credentials.
- **Codex shows no server:** trust the project to permit .codex/config.toml, or use
  connect codex --scope user after reviewing the displayed target file.
- **Claude pending approval:** launch Claude interactively in the project and approve
  its .mcp.json server. Registration does not bypass this client security control.
- **OpenCode V2:** the connector detects mcp.servers; legacy CLI mcp list in 1.17.13
  uses the V1 diagnostic path. Use the active V2 runtime diagnostic for V2 configs;
  see validated behavior and limits in VALIDATION.md. Do not mix both schemas.
- **Missing origin/HEAD:** lifecycle integration cannot infer project identity. Use
  memory tools to select/create an explicit local UUID project. No folder-name guess.
- **Task conflict:** retrieve fresh task revision, review another writer's changes,
  then retry deliberately with a NEW operation_id. Never overwrite blindly.
- **Claim blocked:** previous principal should release it; interrupted claims expire
  after 30 minutes. Claim with the current expected_revision once the lease expires.
- **Verification rejected:** matching task/revision/pass evidence and owner attestation
  are required. Older revision results do not apply to a new commit.
- **Event budget reached:** back up and review disk usage; ask owner to raise the budget.
- **Migration failure:** inspect docker compose logs migrate; checksum failures require
  investigation. The app will not start against a failed migration.
- **Restore failure:** original memory is retained. Inspect the new restore database
  and the recovery backup before retrying; do not remove the volume.

Useful diagnostics: `docker compose ps`, `docker compose logs --tail 100`,
`./bin/thothmemory doctor`. Review logs before sharing them; exclude private memories.

## Patch candidate first-run checks

OpenCode 1.17.13 normal runtime rejects V2 permissions/providers keys. Do not infer
compatibility from `debug v2` or backend doctor; use an independently supported
legacy configuration without overwriting unrelated settings. Experimental V2
registration is guarded with `--allow-unverified-v2` and is not validated runtime support.

After narrowing/revoking tokens, explicitly recreate only app to reload its policy
and bind mounts. Repeated patched setup preserves policy. If a checkpoint is missing,
look for incomplete sessions/recent checkpoints and retry the exact operation ID.
Read [first-use acceptance](audits/FIRST-RUN-ACCEPTANCE.md) before a live upgrade.
