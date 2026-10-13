# ThothMemory Mini (MariaDB)

One container: the Go service supervises a private `mariadbd` (11.4 LTS, pinned by
digest in the glibc variant, by package version in the Alpine variant). MariaDB has
**TCP networking disabled**; the service reaches it through an abstract Unix socket
named from a random installation ID (a filesystem socket is invisible to `docker exec`
under ThothDock's PRoot, measured). The service listens on `127.0.0.1:7319` only, serves
MCP and the dashboard, and nothing else runs.

## Images

| File | Base | Unpacked / gzip | Use |
|---|---|---|---|
| `Dockerfile.mini` | Alpine 3.22 + `mariadb 11.4.12-r0` | 60 MB / 24 MB | Default; amd64 verified; arm64 verified only up to ELF type |
| `Dockerfile.mini-glibc` | Ubuntu 24.04 + official `mariadb:11.4.12` build | 129 MB / 63 MB | arm64 under QEMU completed bootstrap (19 s) and passed all 11 acceptance gates |

Alpine's musl `mariadbd` for arm64 did not finish `mariadb-install-db` under QEMU in
more than 15 minutes, while the glibc build did. QEMU is not an Android phone, so
whichever you choose, the Android gate in [ANDROID_GATE.md](ANDROID_GATE.md) decides.
No Mini image is published; the owner builds and pushes one (`docker buildx build
--platform linux/arm64 -f Dockerfile.mini-glibc …`) and pins it by digest. ThothDock
cannot pull zstd layers: confirm your registry serves gzip.

## Install (ThothDock or any Docker-compatible CLI; no Compose)

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
doctor, backup, restore, uninstall); **not** on a phone.

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
