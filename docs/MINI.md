# ThothMemory Mini (MariaDB)

> **Which Mini?** Use the Mini from the latest release (0.1.2 in v0.2.2; the digest is in that release's
> `MINI_IMAGE_DIGEST.txt`). Mini 0.1.0 (v0.2.0) is superseded: `backup` could fail with
> `TLS/SSL error: certificate is not yet valid` and restore was never verified. Mini 0.1.1 passed the 25-gate
> backup/restore/recovery lifecycle on amd64 and emulated arm64. Results for the newest digest are in
> [PUBLIC_RELEASE_VERIFICATION.md](PUBLIC_RELEASE_VERIFICATION.md). Limits: the physical Samsung test of the
> current digest is pending (earlier results belong to older images); no soak or battery measurement; the
> database socket is protected by the owner password, not by a strong isolation boundary.

## Start here

Mini gives you the browser dashboard and a persistent memory service in one
container. MariaDB runs beside the Go application inside that container, using
a private database socket. UltraLite is simpler if you only need a desktop
SQLite file and no dashboard.

ThothDock is the Docker-compatible engine bundled with ThothTerm on Android.
Start that engine in the app first. Mini uses its plain Docker CLI; Compose is
not required. On ordinary Docker, the installer publishes only host loopback;
on ThothDock, where containers share the device network, the application itself
binds loopback and no forwarder is needed.

From the unified v0.2.2 release, download
`thothmini.sh` and `MINI_IMAGE_DIGEST.txt` from that release. The latter contains
the immutable reference for `lord1egypt/thoth-memory-mini`, with glibc amd64 and
ARM64 children. It is separate from Full and never replaces Full's latest tag.

```sh
sh ./thothmini.sh install --image "$(cat MINI_IMAGE_DIGEST.txt)" --port 7319
sh ./thothmini.sh clients
~/.thothmemory-mini/thothmemory-mini connect all --config ~/.thothmemory-mini/client.json
~/.thothmemory-mini/thothmemory-mini instructions --dir /path/to/project
sh ./thothmini.sh doctor
sh ./thothmini.sh stop
sh ./thothmini.sh start
sh ./thothmini.sh backup /path/to/independent-backups
```

Open http://127.0.0.1:7319 after readiness. Client files and private token files
are under `~/.thothmemory-mini`; database/credentials persist in the named volume
`thothmemory-mini-data`. `sh ./thothmini.sh restore FILE.sql` checks the checksum
and restores into a new database. Follow its printed activation command only
after reviewing the restore. `sh ./thothmini.sh uninstall` removes the container
but **keeps memory**. Never use `--delete-data` as routine maintenance.

Historical Samsung SM-A165F evidence: 11/11 MCP and 14/14 lifecycle gates;
MariaDB VmSize about 1,609 MiB, three added processes. The final public digest's
physical-device retest is **BLOCKED while no phone is connected**. Treat Android
as a preview: short tests only, no multi-hour background/Doze or battery proof,
process-budget pressure, functional rather than kernel-grade PRoot isolation.
See [the exact device evidence](ANDROID_GATE.md). Alpine ARM64 is not the
validated Android variant. [Upgrade and recovery](UPGRADE.md).

One container: the Go service supervises a private `mariadbd` (11.4 LTS, pinned by
digest in the glibc variant, by package version in the Alpine variant). MariaDB has
**TCP networking disabled**; the service reaches it through an abstract Unix socket
named from a random installation ID (a filesystem socket is invisible to `docker exec`
under ThothDock's PRoot, measured). The socket name is not a secret or a strong isolation boundary: ThothDock may share networking and the same UID across containers, so database access is protected by the owner password, not by the socket. The service listens on `127.0.0.1:7319` only, serves
MCP and the dashboard, and nothing else runs.

## Images

| File | Base | Unpacked / gzip | Use |
|---|---|---|---|
| `Dockerfile.mini` | Alpine 3.22 + `mariadb 11.4.12-r0` | 60 MB / 24 MB | Default; amd64 verified; arm64 verified only up to ELF type |
| `Dockerfile.mini-glibc` | Ubuntu 24.04 + official `mariadb:11.4.12` build | 129 MB / 63 MB | arm64 under QEMU completed bootstrap (19 s) and passed all 11 acceptance gates |

Alpine's musl `mariadbd` for arm64 did not finish `mariadb-install-db` under QEMU in
more than 15 minutes, while the glibc build did. QEMU is not an Android phone, so
whichever you choose, the Android gate in [ANDROID_GATE.md](ANDROID_GATE.md) decides.
Publication status is recorded above. The release workflow builds and validates
the glibc distribution and supplies MINI_IMAGE_DIGEST.txt. ThothDock
cannot pull zstd layers: confirm your registry serves gzip.

## Installer reference (source checkout)

```sh
scripts/thothmini.sh install --image REGISTRY/thoth-memory-mini@sha256:DIGEST --port 7319 --restart
scripts/thothmini.sh clients          # native client + per-agent configs in ~/.thothmemory-mini
~/.thothmemory-mini/thothmemory-mini connect all --config ~/.thothmemory-mini/client.json
scripts/thothmini.sh doctor | status | logs | stop | start
scripts/thothmini.sh backup [DIR]     # SQL dump + SHA-256
scripts/thothmini.sh restore FILE     # into a NEW database; then `activate`
scripts/thothmini.sh uninstall        # keeps the volume; --delete-data asks you to type its name
```

It touches only the container `thothmemory-mini` and volume `thothmemory-mini-data`. It uses
no `--read-only`, `--memory`, Compose secrets, health checks or `-p` forwarders, none of
which ThothDock 0.2.1 provides; the service binds loopback itself. Verified end to end
against ThothDock v0.2.1 with Garden PRoot on a Linux host (install, clients, hooks,
doctor, backup, restore, uninstall). The glibc ARM64 image also passed on the Samsung
SM-A165F with released ThothDock 0.2.1: 11 MCP and 14 lifecycle gates. See
[the device evidence and limitations](ANDROID_GATE.md).

## MariaDB settings and why they are safe

`deploy/mini/my.cnf`: `skip-networking`, `performance_schema=OFF`, no binlog, 10
connections, 16 MiB buffer pool, 8 MiB redo log, `innodb_use_native_aio=0` (PRoot and
Android may refuse native AIO), **`innodb_buffer_pool_size_max=64M`** because MariaDB
reserves 8 TiB of address space by default and a 39-bit Android kernel cannot map it
(measured 8.59×10⁹ kB → 139 MiB). Durability is unchanged: `innodb_flush_log_at_trx_commit=1`,
doublewrite on, fsync on every commit. Shrinking the pool below 8 MiB saved under 2 MiB, so
it stays at 16.

## Differences from Full

Shared rules and response shapes are identical (parity-tested). Search uses per-project
substring matching with light English suffix folding instead of PostgreSQL stemming and
query operators; see [editions](EDITIONS.md). Migrations are checksummed and fail closed;
because MariaDB DDL auto-commits, an interrupted migration is detected and refused rather
than re-run: restore the pre-upgrade backup into a new volume. The runtime database login
can insert history and update only tasks, sessions and projects; it cannot insert
attestations or imports, which stay owner operations inside the container.

## Owner commands (inside the container)

`docker exec thothmemory-mini thothmemory-mini status | attest … | backup | restore --sha256 H | activate DB | export P | import | token PRINCIPAL | self-export`.
Credentials are generated on first start in the volume and are never printed to a
terminal (`token` refuses a TTY).
