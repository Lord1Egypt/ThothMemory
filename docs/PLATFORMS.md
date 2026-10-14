# v0.2.2 product family

One source revision distributes Full 0.2.2, Mini 0.1.2 and UltraLite 0.1.0.
The primary GitHub release is v0.2.2. No historical tag is reused.

| Distribution | Platform | Verification / limitations |
|---|---|---|
| UltraLite executable | Windows amd64 | Native CI: init, permissions, SQLite and MCP stdio |
| UltraLite executable | Linux amd64 / arm64 | Native CI: init, SQLite and MCP stdio |
| UltraLite executable | macOS arm64 | Cross-built only; experimental, runtime untested |
| UltraLite executable | Android arm64 | Experimental; prior Samsung native run; final binary retest required |
| Full executable/image | Linux amd64 | Native server/MCP and PostgreSQL integration |
| Full executable/image | Linux arm64 | ELF verified; server/MCP under QEMU |
| Full executable | Windows amd64 | Native client/CLI CI; PostgreSQL server normally runs in Docker |
| Full executable | Windows arm64 | Cross-built only; runtime untested |
| Mini glibc image | Linux amd64 | Native MariaDB/MCP/lifecycle tests |
| Mini glibc image | Linux arm64 | ELF verified; host QEMU acceptance; final phone digest gate separate |

Mini Android preview: prior Samsung SM-A165F with released ThothDock 0.2.1 passed
11 MCP and 14 lifecycle gates. No multi-hour background/Doze or battery guarantee.
The final published digest is **not device-tested until explicitly recorded in
ANDROID_GATE.md**. Mini adds three processes; consider Android's process budget.
PRoot isolation is functional, not kernel-grade. Alpine ARM64 is not the validated
Android distribution. Normal UltraLite operation needs no network or model key;
the chosen AI client may have its own connectivity/account requirements.

SHA256SUMS covers every distributed binary, source archive, installer and support
document. IMAGE_DIGEST.txt identifies Full, MINI_IMAGE_DIGEST.txt identifies Mini.
Both contain immutable registry references; they are metadata, not local image IDs.
