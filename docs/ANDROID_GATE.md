# Android / ThothDock gate: PASS WITH DOCUMENTED LIMITATIONS

Run 2026-10-13 on the owner's Samsung Galaxy A16 (SM-A165F), Android 16, aarch64,
`CONFIG_ARM64_VA_BITS_39=y` (verified from the kernel config), SELinux Enforcing, 65 GB free,
on charger at 32 to 34 °C. Runtime: ThothTerm Debian 0.4.1 with its bundled ThothDock 0.2.1
(commit `9beafd1`, docker CLI 29.8.1), the **released app, unmodified**. The owner's two
live containers (`tonapi-*`) and the engine ran throughout and were still running afterwards.

## Decision

**PASS WITH DOCUMENTED LIMITATIONS** for ThothMemory Mini (MariaDB 11.4.12, glibc arm64 image).
Everything below was executed on the phone; nothing is a host or QEMU result. Raw logs:
`docs/audits/evidence/android/`.

## What ran, and where

| Layer | How | What it proves |
|---|---|---|
| **A. Real app, real engine** | Commands typed into ThothTerm's terminal (via `adb input`; scripts and results moved over `adb reverse`). Image pulled by digest through the engine, installed with `scripts/thothmini.sh` | Mini works through ThothDock exactly as a user would run it |
| B. Same kernel, app's own PRoot as the `shell` user | `libproot.so` from the APK, Mini rootfs, TCP-loopback database mode (the shell SELinux domain cannot bind Unix sockets, so no engine could run there) | Early MariaDB-on-39-bit proof; a different SELinux domain, **not** the app path |
| C. UltraLite native | Android arm64 executable run from `adb shell` | UltraLite on the phone's CPU, filesystem and flash |

## Results, layer A (real ThothDock inside the app)

| Gate | Result |
|---|---|
| Pull by digest (HTTPS, gzip layers, platform arm64) and install | PASS, **51.4 s** total incl. network pull and first MariaDB initialization |
| 11 MCP acceptance gates (handshake, auth 401, CRUD, rollback, 8 concurrent writers → 1 wins, idempotency, evidence gates, Arabic search, read-only credential, handoff) | **PASS 11/11** |
| Database transport | abstract Unix socket, TCP disabled (`status` reports it) |
| Listeners | service on `127.0.0.1:27400` only (checked from adb); no database port |
| 14 lifecycle gates on a fresh fixture: restart (4.4 s), clean MariaDB shutdown, **SIGKILL** (no orphan after 100 ms; recovery 7.1 s, data identical), recreate on same volume (7.8 s), backup + SHA-256, wrong-checksum refusal, restore into a new database, activate and re-activate, clean stop exit 0, no orphaned process | **PASS 14/14** |
| MariaDB virtual size | **1,609 MiB** (default would be 8 TiB; kernel limit is 512 GiB) |
| Cleanup | Only my container, volume and image removed; user containers unchanged; 0 MariaDB processes left |

Resources (whole engine tree, `measure.py`, same device, minutes apart): before Mini 13
processes / 36 threads / 121.5 MiB PSS (engine + the owner's node and nginx); with Mini 16 /
64 / 184.5 MiB, so Mini added **3 processes**. Mini's own idle over 60 s: service 10 ms CPU,
MariaDB 50 ms, 16 and 9 threads, PSS 15.8 MiB and 36.2 MiB. The remaining CPU in that window
belonged to the owner's own node app (15.5 s), not to Mini. ThothDock's Process Guard printed
"ThothTerm currently has 21 processes" while Mini fixtures ran; the engine was never killed.

Latency from a Python client on the phone (includes interpreter cost; 10,030 events): context
33.6 / 43.6 ms, search 90.5 / 103 ms, write 31.1 / 37.8 ms (median / p95). First fresh start of the
lifecycle fixture, image already local: 7.9 s.

## Layer B (shell + PRoot) and C (UltraLite native)

| | Context | Search | Write | Notes |
|---|---|---|---|---|
| Mini, Go client on device, PRoot as shell, 10,050 events | 20.9 / 30.4 ms | 68.6 / 80.3 ms | 20.8 / 34.0 ms | MariaDB RSS 54 to 75 MB, 24 threads, VmSize 1.8 GB |
| UltraLite native, 10,050 events (fsync every commit) | 7.5 / 10.9 ms | 51.1 / 73.3 ms | 8.9 / 35.4 ms | 14 MB executable run, `doctor` 64 ms |

Layer B also passed the same 11 gates and a lifecycle sequence (SIGTERM tree gone in 1 s,
SIGKILL then InnoDB crash recovery then 10,058 events identical, backup, restore).
UltraLite ran init, doctor (integrity, foreign keys, live stdio MCP session, child exit),
and the load test with owner-only permissions on the phone.

## Found and fixed during the gate

1. **TCP-loopback fallback was broken** (host-reproducible): `root@localhost` never matched
   `127.0.0.1` under `skip-name-resolve`. Fixed (resolve names in TCP mode only) and covered
   in CI. The default abstract-socket mode was unaffected.
2. The lifecycle script aborted under `set -e` on the phone's `/proc` scan and relied on
   `pgrep`; now a pure `/proc` scan and non-fatal loop.
3. A published HTTPS-only engine cannot pull from a local registry: staging needs a trusted
   TLS endpoint (see limitations).

## Limitations, stated plainly

- The image reached the engine through a **temporary public HTTPS tunnel** to a local
  registry (owner-approved, torn down afterwards, confirmed HTTP 530). No registry exists for
  the product yet; the owner must publish a digest-pinned image before users can install.
- Short test only: no multi-hour soak, no Android background-kill observation, no Doze.
- Battery was **not** measured (charging); temperature stayed 32 to 34 °C.
- SELinux: Enforcing; I could not read audit logs as shell, so "no denials" is not claimed.
- Alpine arm64 variant was **not** run on the phone; only the glibc image was.
- Latency from a Python client overstates server time; the Go on-device figures (layer B)
  are closer but add PRoot overhead identical to layer A.
- The phone was used by its owner meanwhile (CPU contention from their app).

## Reproduce

`scripts/thothmini.sh install --image REGISTRY/image@sha256:DIGEST --port 27400`, then
`python3 scripts/mini-acceptance.py --endpoint http://127.0.0.1:27400/mcp --token-file ~/.thothmemory-mini/agent_token --reader-token-file ~/.thothmemory-mini/read_token`
and `scripts/mini-lifecycle.sh NAME VOLUME IMAGE PORT` inside ThothTerm.

## Addendum 2026-10-14: published Mini 0.1.2 digest on the physical phone — PASS

Image: `lord1egypt/thoth-memory-mini@sha256:32e9e4c94adc36e8f781202ab55db1862bcdb5ac0a2ec6d10d156947955a7e57`
(Mini 0.1.2, release v0.2.2), pulled by digest through the engine over normal HTTPS (no tunnel). Device: Samsung
SM-A165F, Android 16, ThothTerm Debian guest aarch64, ThothDock server 0.2.1 (API 1.41), docker CLI 29.8.1,
charging at 48-52 %, 30-31 °C. Commands were typed into ThothTerm's terminal over ADB; scripts came from the host over
`adb reverse` and results went back the same way. The owner's `tonapi-*` containers and images were not touched
and were in the same state afterwards. Raw logs: `docs/audits/evidence/android/mini-0.1.2/`.

| Gate | Result |
|---|---|
| Image architecture and MariaDB | `arm64`/`aarch64`, 11.4.12; pull 13.4 s |
| Installer (`thothmini.sh install`) to ready, first start | 13.3 s |
| MCP acceptance | **11/11** (handshake, 401, CRUD, rollback, 8 concurrent writers, idempotency, evidence gates, Arabic search, read-only credential, handoff); 25 tools |
| Lifecycle (`mini-lifecycle.sh`, 3 extra restarts) | **23/23**: restart and 3 more clean restarts (durable `shutdown.log`), SIGKILL (no orphan after 100 ms; recovery 8.8 s, data identical), recreate on same volume, backup + SHA-256, failed/corrupt/malformed backups refused, restore into a new database with all 13 tables and 26 foreign keys identical, activation with identical seeded payload hash, re-activation, clean stop, no orphan |
| MariaDB virtual size | 1,897 MiB (kernel limit 512 GiB) |
| Network exposure | One new listener during the test: `127.0.0.1:27431` (service). No database port. |
| Cleanup | Only `qa-mini`, `qa-lc`, their volumes and the pulled image were removed; 0 MariaDB processes left |

Resources (ThothTerm app UID, measured from ADB): baseline 7 processes / 101 to 102 threads / 106.7 to 108.4 MiB PSS;
with Mini running idle 10 / 139 / 129.0 MiB PSS (**+3 processes, +37 threads, about +21 MiB**); during the lifecycle run
(two containers' worth of activity) peak 20 processes, 155 threads, 115.8 MiB; after cleanup back to 7 / 101 / 106.7 MiB.

Not measured or not run on the device: multi-hour endurance, battery drain (the phone was charging), Doze or
background-kill behavior, MCP reads on the restored copy and the clock-skew gate (both need a host-published port or
`docker build`, which ThothDock does not provide; they are covered on amd64 and emulated arm64 CI), SELinux audit logs.
Alpine arm64 was not tested on the device. Android stays a **preview** with these limits.
