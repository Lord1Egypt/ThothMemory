# Benchmarks

Measured on one machine, 2026-10-13: Intel i7-9750H (12 threads), 12.7 GiB, WSL2
kernel 6.18, Docker 29.8. One synthetic project per run, identical MCP workload
(`scripts/bench-editions.py`), loopback, single client, sequential. Raw results are
under `docs/audits/evidence/bench-*.json`. These are **host** numbers.
**No physical Android device was reachable in this session** (`adb devices` was
empty), so no Android figure is claimed; see [ANDROID_GATE.md](ANDROID_GATE.md).

## Latency (median / p95 ms, 50 samples per size)

| Events stored | Edition | Context | Search | Write |
|---|---|---|---|---|
| 250 | Full (PostgreSQL) | 4.03 / 5.01 | 4.72 / 6.41 | 5.22 / 6.35 |
| | Mini (MariaDB, Docker) | 5.16 / 7.31 | 5.49 / 8.39 | 6.00 / 9.56 |
| | Mini under ThothDock 0.2.1 PRoot (host) | 6.11 / 8.12 | 8.99 / 11.11 | 7.50 / 10.33 |
| | UltraLite (SQLite, stdio) | 1.09 / 1.82 | 2.42 / 3.77 | 3.54 / 5.10 |
| 10,050 | Full | 4.07 / 4.84 | 23.70 / 30.15 | 5.78 / 7.36 |
| | Mini (Docker) | 4.96 / 6.02 | 44.32 / 51.79 | 5.61 / 7.31 |
| | Mini (PRoot) | 6.41 / 7.47 | 49.58 / 54.49 | 7.36 / 8.40 |
| | UltraLite | 1.20 / 1.95 | 19.08 / 21.45 | 4.01 / 5.75 |

Search grows with history in every edition (a per-project scan of text fields);
at 10,000 events it is still under 55 ms p95. Context retrieval is flat. Write
latency is dominated by the fsync of each commit; durability was not relaxed.
Sustained fill rate: Full 172–189/s, Mini 145–165/s (PRoot 114–138/s), UltraLite 286–340/s.

## Footprint

| | Full | Mini (Docker) | Mini (PRoot) | UltraLite |
|---|---|---|---|---|
| Processes at idle | 12 (1 app + 11 PostgreSQL) | 2 | 4 incl. ThothDock engine and PRoot | **1**, only while a client is connected |
| Threads | 26 | 25 | 40 | 11 |
| RSS (PSS) | 18.3 + 170.9 MiB (18.3 + 62.1) | 15.5 + 60.1 MiB (75.1 total PSS) | 95.3 MiB (93.5) | **14.4 MiB (14.4)** |
| CPU over 60 s idle | 90 ms | 30 ms | 10 ms | **0 ms** |
| Context switches / 60 s | 775 | 202 | 194 | **2** |
| Listening sockets | 1 (app) | 1 (service) | 1 | **0** |
| Executable / image (unpacked; gzip) | 28.7 MB + 304.8 MB PostgreSQL image | 60.3 MB Alpine (23.8 MB); 128.7 MB glibc (62.6 MB) | same | **12.5 MB (5.9 MB gzip)** |
| Empty database | PostgreSQL cluster ≈ 36 MB at idle | — | — | **0.2 MB** |
| After 10,050 events | 27.0 MB database, 101.7 MB cluster | 154.3 MB data directory (InnoDB system files included) | | **11.7 MB** |

PRoot RSS is for Mini plus the ThothDock engine on this Linux host; Android's
process cap (about 32 device-wide) makes process count the binding constraint:
Mini adds 3 processes beside the engine, UltraLite adds none while idle.

Startup: UltraLite `init` 30 ms and 12 MB; a full stdio session (spawn, MCP
initialize, tools/list) 9–10 ms; process exit 4 ms after client disconnect, with no
process or listener left. Mini first start 3 s (Docker) to 2.7 s (PRoT host) with a
65 MB initial datadir; restart 0.9–1.4 s; SIGKILL recovery 0.3 s. Full compose
first start 11.3 s. Emulated arm64 Mini first start 19 s (glibc image).

## Findings worth knowing

- Mini is **not** lighter than Full in memory per request path, but lighter in
  processes (2 versus 12) and total PSS (75 MiB versus 80 MiB); its advantage is
  operational (one container, no separate database image), not raw speed.
  MariaDB's resident set (60 MiB) is mostly code pages; shrinking the buffer pool
  below 8 MiB saved under 2 MiB.
- Alpine's musl `mariadbd` for arm64 never finished bootstrap under QEMU in 15+
  minutes while the official glibc build did in 19 s; the amd64 Alpine build is
  fine. Real ARM hardware may behave differently; this is untested.
- UltraLite meets the "very lite" goal by construction: no database process, no
  listener, nothing running between sessions.
- Not measured: battery, Android RSS, concurrent-agent throughput beyond the
  six-process contention test, backup/restore timing at scale.
