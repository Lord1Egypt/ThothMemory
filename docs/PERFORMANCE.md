# Measured performance

Measured 2026-10-12 on Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz; Linux/WSL2 amd64,
Docker Desktop Engine 29.8.1, 12 allocated logical CPUs, about 12.7 GiB Docker memory.
Go 1.26.9 static application and PostgreSQL 18.6. Other containers were running.
These are local development measurements, not a production throughput guarantee.

Real authenticated Streamable HTTP requests (Python stdlib client); warm database,
200 new observation records added per run plus their events and prior benchmark
fixtures. Search returned at most 20 ranked matches. Context uses fixed result caps.

| Operation | Requests | Median ms | p95 ms | Maximum ms |
|---|---:|---:|---:|---:|
| MCP initialization | 20 | 1.429 | 2.663 | 44.895 |
| Context retrieval | 100 | 4.947 | 7.164 | 17.756 |
| Event write | 100 | 6.225 | 8.659 | 13.485 |
| FTS search | 100 | 7.307 | 11.236 | 15.555 |

Eight concurrent writers committed 100 events in 387.325 ms.
Task conflict correctness is tested separately: exactly one stale-revision race wins.
Application restart to readiness took 864.973 ms, including the
Docker restart command; image pull/build time is excluded.

Idle sample after a 10-second settling period (single docker stats sample):

| Container | CPU | Memory |
|---|---:|---:|
| Application | 0.00% | 9.359MiB / 128MiB |
| PostgreSQL | 0.00% | 61.32MiB / 384MiB |

CPU percentages are instantaneous Docker samples, not long-duration profiling.
Connection pool max 8, idle minimum 0; no embedding or model process. PostgreSQL
shared memory/page cache accounting differs from app RSS. Container startup during
initial setup also depends on database initialization, image downloads and hardware.

Reproduce: `python3 scripts/benchmark.py` against a healthy local deployment. It
creates only explicitly named benchmark fixtures and restarts the application.
The fixture grows on repeated runs; record the dataset when comparing results.
