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
# Activate the restored database using .env, exactly as documented. A subsequent
# backup must contain its sentinel rather than silently backing up the old database.
docker compose exec -T db psql -U postgres -d "$restored" -c 'CREATE TABLE backup_selection_sentinel(id integer)' >/dev/null
cp .env "$backupdir/original.env"
trap 'cp "$backupdir/original.env" .env' EXIT
sed "s/^THOTH_DATABASE=.*/THOTH_DATABASE=$restored/" "$backupdir/original.env" > .env
./bin/thothmemory backup "$backupdir/activated.dump"
docker compose exec -T db pg_restore --list < "$backupdir/activated.dump" > "$backupdir/catalog"
test "$(sed -n '/TABLE public backup_selection_sentinel /p' "$backupdir/catalog" | wc -l)" -eq 1
cp "$backupdir/original.env" .env
trap - EXIT
# Verify runtime cannot attest evidence or administer the database.
privileges=$(docker compose exec -T db psql -U postgres -d thoth -Atc "SELECT NOT has_table_privilege('thoth_app','attestations','INSERT') AND NOT has_schema_privilege('thoth_app','public','CREATE')")
test "$privileges" = t
echo 'Deployment, handoff, restart persistence, backup/restore integrity and least privilege: PASS'
