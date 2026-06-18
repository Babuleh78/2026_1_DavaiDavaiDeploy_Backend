#!/bin/bash
# Applies all SQL migrations in build/sql/migrations/ in sorted order.
# Named zzz_ so it runs after createDB.sql and insertDB.sql alphabetically.
set -e

INIT_DIR="/docker-entrypoint-initdb.d"

echo "==> Applying database migrations..."
for f in $(ls "$INIT_DIR/migrations/"*.sql 2>/dev/null | sort); do
    echo "    Applying: $(basename "$f")"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$f"
done
echo "==> All migrations applied successfully."
