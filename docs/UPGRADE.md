# Backup before upgrading

The public release never upgrades an existing installation automatically.
Keep the old binary/image digest, configuration and private credentials. Test
the upgrade on a disposable restored copy before changing a valuable database.

## UltraLite

Run `thothmemory-ultralite backup`; copy the database snapshot and its SHA-256
sidecar to independent storage. Close clients, replace the executable, and run
`doctor`. Pending SQLite migrations are transactional. If rollback is needed,
close clients and use the old executable with a **separate restored pre-upgrade
copy**. Do not open a newer schema with an older executable or delete the original.

## Full

From the installation folder run `./bin/thothmemory backup /safe/backup.dump`.
Restore into a disposable installation using the documented owner CLI and
verify counts/history before rehearsing migration. Migrations 005, 006 and 007
are additive and checksummed. Rehearse the dashboard and MCP with representative
historical data. Only after reviewing the backup and downtime should an owner
change THOTH_IMAGE to the verified release digest and run setup.

Rollback uses the old image plus a separate restored pre-upgrade database and
preserved credentials/configuration. It does not reverse SQL migrations in place.
v0.1.1's ARM64 image is broken; use the verified v0.1.2 ARM64 digest as an older
application baseline where its pre-upgrade schema is compatible. Never use
`docker compose down -v` on valuable data. [Detailed commands](BACKUP_RESTORE.md).

## Mini

`sh thothmini.sh backup /safe/backups` creates a SQL dump and checksum. Restore
uses a new database; review `activate` separately. MariaDB DDL is not fully
transactional: an interrupted migration fails closed. Keep the original volume
and use a disposable restored copy to validate recovery rather than blindly
rerunning an incomplete migration. Stop the old container, retain its volume,
and recreate only after reviewing the installer/image and backup. Uninstall
without `--delete-data` keeps the volume. There is no unattended upgrade command.

For all editions: portable project archives are transfer documents, not a full
installation backup or continuous synchronization. Verify their checksums and
import results; they preserve newer revisions and reject divergent conflicts.
