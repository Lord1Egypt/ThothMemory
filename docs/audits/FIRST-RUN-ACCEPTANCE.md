# Owner's first real handoff

This procedure is for the owner, after reviewing and approving a patch release.
The audit did not upgrade the live installation. Its app and DB containers were
already stopped when inspected; resolve availability explicitly before attempting
an authenticated backup. Do not run an installer just to investigate stopped services.

Published 0.1.1 Docker arm64 is defective (x86-64 executable in arm64 manifest).
ARM container users must wait for the authorized corrected patch image; do not treat
the prior arm64 smoke report as proof of compatibility. The tested owner installation
uses amd64; its live state is still protected.

## 1. Protect the existing installation

From its normal installation directory, inspect `docker compose ps -a`. If it is
intentionally stopped, decide whether to start it; this is an owner action. Once the
DB is available, use the existing CLI to create a uniquely named backup:

```sh
./bin/thothmemory backup /protected/location/before-upgrade.dump
```

Windows: `.\bin\thothmemory.exe`. Keep dump + `.sha256`, current `.env`, protected
`.secrets`, original image digest, Compose project/volume identity, and an independent
copy of the old CLI. Use encrypted storage; dump checksums do not authenticate a
maliciously replaced archive. With 0.1.1 on Windows protect the backup directory ACL
before writing: its Go Unix modes alone do not enforce Windows access restrictions.
Never use `docker compose down -v` or delete credentials.

## 2. Upgrade only after approval

The branch identifies itself as `0.1.2-rc.1`; it has no public release image. Do not
point production at an invented 0.1.2 tag. After a validated release is explicitly
approved, retain the existing installation directory/configuration and volumes,
select that exact published digest in THOTH_IMAGE, and run the supplied installer:

```sh
./setup.sh
# Windows: .\start-thothmemory.cmd
./bin/thothmemory doctor
```

Migration 005 adds search representations; the normal installer runs migrations.
Keep pre-upgrade backup and old database; unchanged credentials/project IDs/task
revisions were tested on disposable clones. The installer defaults in this audit
branch still target published 0.1.1 deliberately, pending release authorization.

## 3. Connect one trusted test repository

Use absolute paths to the installation CLI; `--dir` points to a separate Git repository:

```sh
./bin/thothmemory connect all --scope project --dir /path/to/test-repository
./bin/thothmemory instructions --dir /path/to/test-repository
./bin/thothmemory doctor
```

Accept each client's real repository/MCP trust prompts. Existing configurations and
instructions are backed up/preserved; conflicting registrations stop for review.
OpenCode 1.17.13 must use legacy configuration; V2 normal run is not accepted. Hooks
are optional additional assistance on Linux/WSL:

```sh
./bin/thothmemory hooks claude --dir /path/to/test-repository
./bin/thothmemory hooks codex --dir /path/to/test-repository
./bin/thothmemory hooks opencode --dir /path/to/test-repository
```

Enable Codex hooks with `--enable hooks` and approve their commands normally. Native
Windows uses explicit instruction-driven checkpoints. Registration alone does not
make any model write memory. Global scope is optional and separately consented.

## 4. Perform the actual handoff

1. Claude: identify canonical remote and full HEAD, resolve/create the project, start
   a session, load context. Create one small task. Record a real decision or failed
   attempt and a checkpoint with concrete evidence; finish with a brief handoff.
2. Codex: resolve the same project, compare current HEAD, read Claude's handoff and
   failure before working. Update with expected revision, record actual test output
   and checkpoint, then hand off. Do not mark agent-reported output owner verified.
3. OpenCode: read Codex's current task/revision and handoff, resume the small task,
   checkpoint and finish/pause honestly.
4. Claude: retrieve final shared state and distinguish completion, failures and
   remaining work. Compare IDs across clients; do not create another project merely
   because a checkout has a different folder name.

Open `http://127.0.0.1:7319`, unlock locally using the token file, select the project
and inspect timeline/handoff/evidence/search. Keep tokens out of prompts and Git.
After an owner-approved app restart, run doctor and retrieve the same task/handoff.

## 5. Recover a missing checkpoint or roll back

If a client loses connection, do not claim the write succeeded. Retry the same
operation ID/payload; reload on revision conflict. If finalization was skipped, the
next client must see an incomplete session and recent checkpoints, not infer completion.
Record unsynced work explicitly and recover it when service is healthy.

For an upgrade failure, retain logs and original volumes. Restore the trusted
pre-upgrade archive using `restore ARCHIVE --confirm`; this creates a recovery backup
and a new DB without replacing the original. Activate the printed database through
THOTH_DATABASE with the retained 0.1.1 image/CLI/configuration when rolling back.
A pre-upgrade dump is required for that schema rollback; do not attempt to remove
005 columns manually or edit recorded migration checksums. Revalidate doctor and
memory before resuming. The backup-after-activation selection path was tested.

Optional after trial: use `permissions codex --projects PROJECT_UUID` (likewise
claude/opencode), or `--read-only` / `--revoke`, then explicitly recreate only app
as printed by the CLI. Policy takes effect on app startup; current requests retain
the old policy until reload. Do not rerun 0.1.1 init/setup after revoking default
principals: the patch fixes a baseline revocation-recreation defect.
