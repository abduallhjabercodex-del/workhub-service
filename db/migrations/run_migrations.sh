#!/bin/sh
set -e

echo "[MIGRATE] Waiting for PostgreSQL database to be ready..."
until pg_isready -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE"; do
  echo "[MIGRATE] Database is unavailable - sleeping 1s..."
  sleep 1
done

echo "[MIGRATE] Database is ready. Initializing migration schema tracking..."

# Create tracking table if it doesn't exist
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 <<EOF
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
EOF

# If the database was already initialized on prior runs before schema_migrations was created,
# backfill all existing migrations into schema_migrations so they are not re-executed.
users_exist=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -tAc "SELECT 1 FROM information_schema.tables WHERE table_name='users';")
if [ "$users_exist" = "1" ]; then
  for f in $(ls -1 /migrations/*.up.sql | sort); do
    v=$(basename "$f")
    psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c "INSERT INTO schema_migrations (version) VALUES ('$v') ON CONFLICT DO NOTHING;" >/dev/null 2>&1
  done
  echo "[MIGRATE] Existing database detected, schema_migrations synchronized."
fi

# Run any pending migrations
for sql_file in $(ls -1 /migrations/*.up.sql | sort); do
  version=$(basename "$sql_file")
  already=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -tAc "SELECT 1 FROM schema_migrations WHERE version='$version';")
  
  if [ "$already" = "1" ]; then
    echo "[MIGRATE] Migration already applied: $version (skipping)"
    continue
  fi

  echo "[MIGRATE] Applying migration: $version"
  psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 -f "$sql_file"
  psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c "INSERT INTO schema_migrations (version) VALUES ('$version') ON CONFLICT DO NOTHING;"
done

echo "[MIGRATE] All database migrations applied successfully!"
