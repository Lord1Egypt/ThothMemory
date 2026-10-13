# Performance regression audit

## Method and hardware

Intel Core i7-9750H, 12 logical CPUs; WSL2/Docker Desktop, approximately 12.7 GiB
available to Docker. Go 1.26.9, PostgreSQL 18.6, pool max 8/min 0. No model workload
or embedding process is part of the product. Four unrelated Docker workloads were
left running; this is a shared development host, not a dedicated benchmark machine.

Final comparison uses independent **fresh PostgreSQL volumes**, the exact same
pre-candidate seed archive and identical Compose settings. Versions ran sequentially
without audit builds in flight. Dataset: projects with 0, 100 and 2,500 engineering
observations, each producing an event; additional search fixture notes. Final counts
are identical: 2,608 records and 2,908 events. Each operation/size has 60 authenticated
HTTP MCP calls, then 120 writes with 8 concurrent workers. Percentiles use sorted
samples, p95 at index 56 for n=60. Connections include the measuring SQL connection.

Evidence: [baseline JSON](evidence/performance-baseline.json),
[candidate JSON](evidence/performance-candidate.json).
Reproduction helpers and precautions: [scripts/audit](../../scripts/audit/README.md).

## Latency (milliseconds, median / p95)

| Records in project | Operation | Published 0.1.1 | Candidate |
|---:|---|---:|---:|
| 0 | context | 5.357 / 7.293 | 3.922 / 5.977 |
| 0 | search | 3.020 / 5.567 | 2.724 / 4.177 |
| 0 | write | 4.927 / 7.263 | 5.552 / 7.636 |
| 100 | context | 3.741 / 4.891 | 4.168 / 5.196 |
| 100 | search | 7.402 / 9.018 | 6.891 / 9.017 |
| 100 | write | 5.519 / 7.579 | 5.446 / 6.695 |
| 2,500 | context | 5.055 / 6.577 | 7.569 / 10.665 |
| 2,500 | search | 41.816 / 81.531 | 17.450 / 22.495 |
| 2,500 | write | 5.536 / 7.129 | 5.748 / 6.890 |

Candidate largest observed search outlier: 32.947 ms; baseline 99.650 ms. Context
cost increases about 2.5 ms at moderate size with the additional checkpoint query;
its absolute p95 remains 10.665 ms. Writes are close in the final paired workload.
120 concurrent writes: baseline 410.438 ms total, candidate 443.510 ms (+8.1%).
App restart to readiness: 1,123.773 ms versus 1,087.663 ms. This measures app restart,
not clean image download or an empty database's full initialization.

## Resources and storage

| Measurement | Baseline | Candidate |
|---|---:|---:|
| App RSS | 21,036 KiB | 20,988 KiB |
| App Docker memory sample range | 9.773–10.96 MiB | 9.621–11.02 MiB |
| PostgreSQL Docker memory sample range | 91.04–94.84 MiB | 98.06–103.70 MiB |
| PostgreSQL parent RSS only | 30,204 KiB | 30,452 KiB |
| App process count | 1 | 1 |
| App threads at sample | 16 | 15 |
| PostgreSQL process count | 17 | 17 |
| DB connections including measurement | 9 | 9 |
| App / PG parent file descriptors | 15 / 10 | 15 / 10 |
| Database bytes | 23,000,767 | 27,276,991 |
| App CPU sampled average / max | 0.389% / 4.67% | 0.318% / 3.78% |
| PG CPU sampled average / max | 1.124% / 4.66% | 3.197% / 11.91% |

Twelve Docker stats samples span roughly 36 seconds per version. They include normal
health checks and PostgreSQL maintenance after a restore/workload. These are finite
idle observations, not proof of zero ongoing CPU or absence of long-term growth.
The higher PG CPU burst remains a measurement limitation requiring a real-use soak.
Docker memory excludes some reclaimable cache; RSS includes mapped pages. Summing PG
worker RSS double-counts shared mappings, so parent RSS is explicitly not total DB
memory. The generated text/vector/index representations cost about 4.28 MB (+18.6%)
at this dataset size. Event/record counts are unchanged by migration; new writes
remain append-only and bounded by the project's event budget, not automatic retention.

## Investigation and rejected measurements

An initial clone comparison on one reused PG instance measured 15.07/17.38 ms search
medians but mixed database caches, so it is not the final resource comparison.
Fresh-volume testing exposed a pre-fix candidate slowdown (100.639 ms search median).
`EXPLAIN` with `plan_cache_mode=force_generic_plan` reproduced repeated text-query
parsing: about 78.5 ms database execution versus 15.3 ms after materializing the
normalized English/simple query once. [Final generic plan](evidence/search-plan-final.txt).
A separate redundant substring rank boost was also removed earlier. The final paired
numbers above include both corrections; no global prepared-cache or JIT setting was
changed and no persistent cache/service was introduced.

Other runs varied on the shared host (including a concurrent installer build), so
no universal latency/CPU guarantee is implied. Historical 9.36 MiB app / 61.32 MiB
PG figures used different workload/cache conditions and are not comparable RSS caps.
There are no new persistent processes, polling workers, queues, models or embeddings.
Large-scale substring scans, migration locking and long-duration disk growth are
remaining capacity questions beyond this initial daily-use trial.
