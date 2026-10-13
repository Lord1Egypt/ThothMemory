# Android / ThothDock gate

**Status: BLOCKED. No physical Android device was reachable in this work** (`adb devices`
was empty; WSL2 exposes no `/dev/bus/usb`). Nothing below is a PASS on a phone. What *was*
run is a Linux-host proxy: ThothDock v0.2.1 built from its tag with Garden's PRoot, and the
Mini image run under it.

## What was verified on the host proxy (not Android)

| Check | Result |
|---|---|
| `docker pull` of the Mini image into ThothDock 0.2.1 (API 1.41, TLS registry, gzip layers) | PASS, 1.9 s |
| First start, restart (`docker restart`), SIGKILL of the container, recreation on the same named volume, clean stop | PASS: data identical each time; no orphaned `mariadbd`; SIGKILL recovery 0.3 s |
| Owner commands through `docker exec` (abstract socket) | PASS after switching from a filesystem socket, which PRoot hides from other tracees |
| `scripts/mini-acceptance.py` (11 MCP gates), `scripts/mini-lifecycle.sh` (14 gates) | PASS |
| MariaDB virtual address space | 139 MiB instead of the default **8 TiB** (see [Mini](MINI.md)); needed because the documented test phone has a 39-bit user address space |
| Idle process tree under ThothDock | 4 processes (engine, PRoot, service, mariadbd), 93.5 MiB PSS, 10 ms CPU per minute |

## What cannot be claimed without the phone

MariaDB itself on Android (SELinux, `fsync` behavior, the 39-bit address space actually
mapping, background process kills), the real process budget next to the engine (about 32
device-wide), memory pressure, thermal and battery effects, and the Android arm64 image
digest. Native arm64 was exercised only under QEMU (glibc image).

## Runbook for the owner (about 30 minutes, one device)

Preconditions: ThothDock 0.2.1 or the current ThothTerm Trixie QA build, free storage
≥ 500 MB, a registry the phone can reach serving gzip layers, an arm64 Mini image pinned by
digest. Use a throwaway volume; do not touch existing containers.

```sh
# in the ThothTerm Debian guest
ps -e | wc -l                                  # note the baseline process count
git clone https://github.com/Lord1Egypt/ThothMemory.git && cd ThothMemory && git checkout <approved commit>
scripts/thothmini.sh install --image REGISTRY/thoth-memory-mini@sha256:DIGEST --port 27319
ps -e | wc -l                                  # expect +3 (service, mariadbd, PRoot); stop if the engine dies
scripts/thothmini.sh clients
python3 scripts/mini-acceptance.py --endpoint http://127.0.0.1:27319/mcp --token-file ~/.thothmemory-mini/agent_token --reader-token-file ~/.thothmemory-mini/read_token --bench 50 --json /sdcard/mini-acceptance.json
scripts/mini-lifecycle.sh thothmini-gate thothmini-gate-data REGISTRY/thoth-memory-mini@sha256:DIGEST 27320
grep -E 'VmSize|VmRSS|Threads' /proc/$(pgrep -f 'mariadbd --defaults-file' | head -1)/status
```

Record: PASS/FAIL per gate with exit codes, the process count before and after, `VmSize`,
`VmRSS`, 60 s idle CPU, and `scripts/thothmini.sh logs` if anything fails. If MariaDB
fails to start (look for `mmap` or `Out of memory` in the log), stop and report; the fallback
is **UltraLite embedded in PocketClaw** ([contract](POCKETCLAW_INTEGRATION.md)), which needs
no extra process at all. Do not raise process limits or disable Android protections.
