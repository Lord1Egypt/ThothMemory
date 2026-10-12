#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
./bin/thothmemory init-config
chmod 700 .secrets
chmod 644 .secrets/db_owner_password .secrets/db_app_password .secrets/tokens.json
docker compose up -d --wait --wait-timeout 120
./bin/thothmemory doctor
./bin/thothmemory demo
before=$(docker compose exec -T db psql -U postgres -d thoth -Atc 'SELECT count(*) FROM events')
docker compose restart app db
for ((i=0;i<90;i++)); do if ./bin/thothmemory doctor >/dev/null 2>&1; then break; fi; sleep 1; done
./bin/thothmemory doctor
after=$(docker compose exec -T db psql -U postgres -d thoth -Atc 'SELECT count(*) FROM events')
test "$before" = "$after"
backupdir=$(mktemp -d)
./bin/thothmemory backup "$backupdir/acceptance.dump"
./bin/thothmemory restore "$backupdir/acceptance.dump" --confirm | tee "$backupdir/restore.log"
restored=$(sed -n 's/^Restored into //p' "$backupdir/restore.log")
count=$(docker compose exec -T db psql -U postgres -d "$restored" -Atc 'SELECT count(*) FROM events')
test "$count" = "$after"
# Verify runtime cannot attest evidence or administer the database.
docker compose exec -T db psql -U postgres -d thoth -Atc "SELECT NOT has_table_privilege('thoth_app','attestations','INSERT') AND NOT has_schema_privilege('thoth_app','public','CREATE')" | rg '^t$'
echo 'Deployment, handoff, restart persistence, backup/restore integrity and least privilege: PASS'
