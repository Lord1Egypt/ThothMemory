# v0.2.0 public verification — 2026-10-14

## Published, with a failed Mini recovery gate

Release: https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.2.0

Source/PR #8 merge: `be30dfe67c9094f197d8810add7b77fb3abcb5d8`.
Annotated tag object: `3f1cdb82918c5bd62ba10c6dad6a7cc55ea8691d`.
Full component 0.2.0; Mini and UltraLite components 0.1.0.

GitGuardian was officially rerequested using GitHub's check-run rerequest API
after the owner's incident review. Fresh check 115653901254 succeeded, scanning
29 commits without secrets. Final reviewed head 5ca6a8391d90e0887301a0096f9878d6dbf54b06
also passed GitGuardian and all six CI jobs. No bypass or scanner disablement.

- PR CI: https://github.com/Lord1Egypt/ThothMemory/actions/runs/38503392283
- Main CI: https://github.com/Lord1Egypt/ThothMemory/actions/runs/38503866569
- Release: https://github.com/Lord1Egypt/ThothMemory/actions/runs/38504405851
- Docker publication: https://github.com/Lord1Egypt/ThothMemory/actions/runs/38504979873

All four runs succeeded. Publication included all 16 expected assets. Independent
anonymous downloads matched the prepared artifacts; every SHA256SUMS entry
passed. Linux amd64 UltraLite's documented user-local installation, MCP idea
write/read, backup and restore passed. Public Windows amd64 UltraLite executed
natively: initialization, private ACLs, SQLite integrity, MCP stdio handshake,
backup and restored doctor passed. Local PowerShell script-file policy was not
disabled; direct documented commands were used. CI additionally covers native
Windows/Linux amd64/Linux ARM64 newcomer persistence flows. macOS is build-only;
Android UltraLite remains experimental.

Full's public digest independently passed amd64 and emulated arm64 executable
architecture/startup/PostgreSQL/MCP smoke tests. Full/Mini provenance and SBOM
metadata reference the released source revision.

## Immutable registry digests

Full `lord1egypt/thoth-memory:0.2.0` and `latest`:
`sha256:784eb92b1c5c7f4145e879f325033a33546aeaeb55dd154b450f2581fb2d9526`

- amd64: `sha256:35a0fb365183e37dc930758e219a62e38bcc9106e6802d769d7a6a7af93975ab`
- arm64: `sha256:3b68be44205adcd5741e57ff315fa871c60cb3861d7e6869952bd2e3793cb57a`

Mini `lord1egypt/thoth-memory-mini:0.1.0`:
`sha256:a73a2e3c344ba3c5537e751d520b15ab334475a89d7aeebd3782e29c1083ef9d`

- amd64: `sha256:8cc6dbb6cf98883b38302a427e5488a3f0823eb0316b5eea7e989c470c094142`
- arm64: `sha256:a3613b54b70935f03eb75f8eac878d41434d3c4a1eee9ff2e7a3e3336ba8dd0f`

Manifest bytes and platform children were checked against registry digests;
Mini layers use gzip. Historical Full 0.1.1 and 0.1.2 index digests remain
`sha256:0edd6990289b12099a5c4d44525e9e5bf03fc52934ba09dfb43f293ed4bf9e63`
and `sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245`.

## STOP: independent Mini backup failure

The publication runner passed its seeded lifecycle tests, but the subsequent
independent host check against the public Mini digest exited 1. Both architectures
passed 11/11 MCP gates. Restart, SIGKILL recovery and volume recreation preserved
the seeded portable payload hash (13 events, four sessions, four records, one task).
MariaDB VmSize was 1960 MiB on this host, not an Android measurement.

At 03:21:34 UTC, backup failed:

```text
mariadb-dump: Got error: 2026: "TLS/SSL error: certificate is not yet valid" when trying to connect
ERROR operation failed error="mariadb-dump failed"
```

No valid backup checksum resulted; subsequent restore could not run. The client
invocation is in `internal/mini/backup.go`; certificate validity/time root cause
is not established. Do not call this a passing recovery gate or recommend Mini
as production-ready. No TLS/security bypass, blind publication retry, version-tag
mutation or replacement asset was attempted. Disposable test fixtures were cleaned.

Next action requires owner direction after this failed public-release gate:
diagnose certificate validity using an isolated fixture, repair and test recovery,
then prepare an immutable corrective release rather than replacing 0.1.0/0.2.0.
Do not republish the existing semantic-version tags. The unified launch is not
fully verified while this blocker remains.

Final Mini physical-device digest retest remains BLOCKED when no ADB device is
available. Historical Samsung 11 MCP/14 lifecycle passes remain valid historical
evidence, not validation of this digest or long-duration/battery endurance.

## Safety and evidence

Production containers remained stopped with their original stop timestamps;
no production database, secrets, configuration, volume or backup was changed.
The previously verified cold backup remains the recovery point; deployment and
migration still require separate authorization. Claude hooks and Codex semantic
handoff retain their prior verified scope; Codex lifecycle capture is not claimed.

Local detailed logs, downloaded assets, registry JSON, provenance and SBOMs:
`bin/release-v0.2.0/` (ignored, intentionally not published as raw machine logs).
Resume with this document, not superseded pre-publication notes. Safe checks:

```bash
gh release view v0.2.0 --repo Lord1Egypt/ThothMemory
gh run view 38504979873 --repo Lord1Egypt/ThothMemory
```

## Root cause of the Mini backup failure (diagnosed 2026-10-14)

Established with the exact published digest in disposable containers:

- MariaDB server and clients are 11.4.12. The server generates a self-signed certificate in
  memory at every start (`Ssl_server_not_before` equals the start second, 10-year validity).
- Native `mariadb`/`mariadb-dump` do negotiate TLS (`TLS_AES_256_GCM_SHA384`, "cert is OK")
  over the abstract Unix socket; no global option file overrides it. The Go driver connects
  without TLS, so the app itself never saw the error.
- With the client's clock 5 s behind (`libfaketime`) the original error is reproduced
  deterministically and `backup` exits 1; with the real clock it succeeds.
- Not reproduced naturally: 6 recreate-then-backup runs on amd64, 6 restart runs, and one
  emulated arm64 run all passed. The trigger on 2026-10-14 03:21 UTC (a client clock that
  read earlier than the server's start second) was not individually captured; host and
  container clocks agreed when checked. The defect is the unnecessary TLS dependency on a
  clock comparison over a private socket, fixed in Mini 0.1.1 (see docs/BACKUP_RESTORE.md).

## Corrective candidate evidence (local, 2026-10-14; CI results are in the PR)

Candidate source branch `hotfix/mini-backup-tls`. Images were built locally from
`Dockerfile.mini-glibc` (amd64 native; arm64 under QEMU, **emulated**) and `Dockerfile.mini`
(Alpine amd64). No candidate has been pushed to a registry, so no corrected digest exists yet.

`THOTH_LIFECYCLE_MCP=1 THOTH_SKEW_GATE=1 scripts/mini-lifecycle.sh …` result: glibc amd64
25/25, glibc arm64 emulated 25/25, Alpine amd64 25/25. Covered: seeded MCP data (project, task,
sessions, events, evidence, note, idea, handoff), restart, SIGKILL recovery, recreation, backup
with SHA-256, failed backup leaves no checksum or restorable artifact, wrong-checksum, one-bit
corrupt and malformed backups refused with no leftover database, backup with the client clock 5 s
behind, restore into a NEW database, equal row checksums in all 13 tables, identical
`schema_migrations`, all 26 foreign keys present with no orphan rows, activation, identical
export hash and identical real MCP read results on the restored copy, re-activation of the
original, clean stop, no orphan process. The same script against the published 0.1.0 digest fails
the skew gate with the original TLS error.

Android: **BLOCKED.** The Samsung SM-A165F was reachable over wireless ADB, but no corrected
digest is published to pull, staging an unpublished image would need a public tunnel that has
not been approved for this release, and the phone was in active use by its owner (another app
in the foreground). The device was not modified. Earlier Samsung evidence applies to Mini 0.1.0
only and is preserved unchanged.

## v0.2.1 published and independently verified (2026-10-14)

Release https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.2.1 (merge commit `f1bac9a`, PR #10).
Release run 38536634909 and Docker publication run 38537717127 succeeded; 16 assets; all 13 SHA256SUMS
entries verified after an anonymous download.

- Mini 0.1.1 `lord1egypt/thoth-memory-mini:0.1.1`: `sha256:6f457f5d3ad060a045fa81c652e6fad30263973c79119f8a50a06752bbd29902`
  (amd64 `sha256:c045877b…38d8`, arm64 `sha256:1883acec…5bca`).
- Full 0.2.1 and `latest`: `sha256:73dc7b329b3da64817497bec23fb23ab946a2762bb6b7524ce9a63f7ae54b86d`.
- Unchanged: Mini 0.1.0 `sha256:a73a2e3c…ef9d`, Full 0.2.0 `sha256:784eb92b…9526`.
- Pulled by digest, `scripts/smoke-mini-platforms.sh`: 11/11 MCP gates and the 25-gate backup, verified restore
  and recovery lifecycle passed on linux/amd64 and on linux/arm64 (**emulated**, QEMU).
- Post-merge CI first failed once on the existing "no clean MariaDB shutdown logged" check and passed on rerun;
  not reproduced locally (0 of 25 restarts). Treat as an unexplained intermittent CI failure.
- Android: **pending.** The phone was reachable but in active use; the engine can only be driven through the app UI.
  The published digest has not been tested on the device.
- Full and UltraLite 0.2.1 were rebuilt by the release workflow; only Mini was independently re-tested here.
