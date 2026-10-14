# Backup and restore

Run these commands from the installation directory with Docker running.

```sh
./bin/thothmemory backup backups/memory.dump
./bin/thothmemory restore backups/memory.dump --confirm
```

Windows: use `.\bin\thothmemory.exe`. Backup uses pg_dump custom format, transactionally
consistent snapshots, restrictive file permissions and a SHA-256 sidecar. Existing
backup paths are never overwritten. Copy both files to protected storage.

Restore checks the checksum and archive catalog, creates a recovery backup of the
current database, and restores into a NEW timestamped database in one transaction.
It then validates the projects/events tables. The original remains intact. Review
counts, then set `THOTH_DATABASE=<printed new database>` in `.env` and rerun setup.
Migrations reapply runtime grants for the selected database. To roll back, restore
the prior THOTH_DATABASE setting and rerun setup. Keep the recovery archive.

For another PC, install ThothMemory there, copy the trusted dump and checksum, run
restore and activate the printed database. New local credentials are independent of
those on the old PC. Agent registrations must point to the new installation's CLI.
Named volumes survive container recreation and `docker compose down`. The destructive
`docker compose down -v` removes them; never use it without a tested backup.

## Upgrades and interruption

Back up before changing THOTH_IMAGE. Setup retains credentials and volumes. Embedded
numbered migrations are checksum-checked under a PostgreSQL advisory lock and one
transaction. An interrupted migration rolls back; rerunning starts safely. A checksum
mismatch stops startup: restore the original migration files/image, investigate and
add a new migration. Never edit an already released migration to force an upgrade.

An uncertain agent write should be retried with the same operation_id and exact
payload; transaction commit and the stored response make retries safe. Conflicting
revision writes require reloading state. If PostgreSQL is unavailable, readiness
fails; clients report the failure and should retry checkpoints after recovery. App
pool connections recover when PostgreSQL returns. No in-memory queue claims success.

Candidate Windows CLI protects dump files with explicit owner ACLs before writing.
Backups/volumes are not encrypted by ThothMemory; use encrypted host storage for
at-rest protection. A SHA-256 sidecar is corruption detection, not authenticity.
For a schema downgrade use a pre-upgrade dump restored into a new database and the
retained old image. Do not manually delete migration columns/checksums. See the
[owner acceptance and rollback procedure](audits/FIRST-RUN-ACCEPTANCE.md).

## Mini: how backup and restore reach the database

Mini's `backup` runs the container's own `mariadb-dump`, and `restore` runs `mariadb`, with
a private option file (mode 0600, removed afterwards) so the password never appears in a
process argument or log. Both connect over the container's abstract Unix socket; MariaDB is
started with `skip-networking`, so there is no TCP listener.

MariaDB 11.4 clients still negotiate TLS over a Unix socket, against a self-signed
certificate the server generates in memory at every start, valid from that second. A client
whose clock reads earlier than the server's start second (a VM or WSL2 clock step, emulation)
fails with `certificate is not yet valid`. Mini 0.1.1 therefore writes `skip-ssl` into the
option file **only for the socket transport**. This opens no listener and removes no
authentication: every connection still needs the owner password. It is not a claim of strong
isolation. An abstract socket is scoped to a network namespace, not to a UID or a container
boundary; ThothDock and other runtimes may share networking and the same UID between containers,
so another local process in that namespace may be able to reach the socket. The password, kept in
a 0600 file under /data, is the protection, and TLS to a self-signed in-memory certificate never
authenticated the server anyway. In the optional `THOTH_DB_TCP` loopback mode the option file is
unchanged, so TLS negotiation and certificate verification stay on; a unit test pins both
branches and rejects any `skip-ssl`/`ssl-verify-server-cert=0` there.
There is no general insecure fallback.

A failed backup exits non-zero and prints no `sha256=` line, so it never looks like a valid
artifact. Restore refuses a checksum mismatch before touching the server, loads into a NEW
database, tries to drop that database if loading or validation fails (bounded 30 s; the error says whether the drop succeeded), and never activates it
unless you run `activate` explicitly.
