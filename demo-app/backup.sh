#!/usr/bin/env bash
# Back up the application data. Run inside the app container or with
# DATABASE_URL pointing at the database.
set -euo pipefail
mkdir -p backups
pg_dump "$DATABASE_URL" > "backups/db-$(date +%Y%m%d-%H%M%S).sql"
echo "backup written to backups/"
