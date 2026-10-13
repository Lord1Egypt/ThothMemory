# Reproducing the audit

Use a disposable checkout, generated synthetic credentials, a unique
`COMPOSE_PROJECT_NAME=thoth-audit-...` and an unused `THOTH_PORT` other than 7319.
Never copy live credentials/data to these fixtures. The helpers refuse default-port
MCP access. They write synthetic records and restart their own application.
Python 3 and Docker Compose are required; no Python dependencies or model APIs.

After setup and doctor in that isolated checkout:

```sh
python3 scripts/audit/search.py
python3 scripts/audit/performance.py seed
./bin/thothmemory backup .validation/audit-seed.dump
python3 scripts/audit/performance.py baseline > .validation/baseline.json
python3 scripts/audit/recovery.py
```

`search.py` creates/queries synthetic multilingual notes.
`recovery.py` backs up, compares every core table, rejects corruption, activates a
new DB, makes a real MCP sentinel write, backs up/restores that activated DB, and
confirms the original stayed unchanged. It deliberately leaves the isolated `.env`
pointing at the validated restored database. `THOTH_AUDIT_CLI` can select another
local test binary. Its archive names are unique on every run.

For a controlled version comparison, restore **the same seed archive** into a fresh
PostgreSQL named volume for each version, use identical settings/hardware and copy
`.validation/audit-performance-state.json` containing synthetic project IDs.
Run versions sequentially with no builds or other audit workloads in flight, then
stop that audit Compose project. Do not compare a fresh DB against an old cache-filled
DB. `performance.py` resolves the selected DB through Compose, reports 60 samples
per operation/project size, 120 writes with 8 workers, 12 idle samples and restart
readiness. Unique operation labels are needed if reusing a database.

Resource capture uses a short-lived PostgreSQL-image helper in the audit container's
PID namespace to inspect `/proc`; it never mounts the Docker socket. The deployed
product does not need that helper, Python or a polling worker.

Core acceptance is independently automated with `./scripts/test-integration.sh`
and CI `./scripts/acceptance.sh` (also disposable). Native Windows regressions are
`go test ./internal/cli` and `tests/windows-image-selection.ps1`. Genuine client
steps and untested cases are distinguished in `docs/audits/AGENT-COMPATIBILITY-MATRIX.md`.
