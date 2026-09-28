# Database Backup & Restore Guide for PostgreSQL

This document outlines the standard operating procedures for generating database backups and restoring them to a clean PostgreSQL instance for WorkHub.

## 1. Prerequisites
- PostgreSQL 16 `pg_dump` and `pg_restore` utilities.
- Access to the target database URL (e.g. `workhub_dev`).

## 2. Backup Execution (pg_dump)

To create a full custom-format database backup containing schema, indices, constraints, and data:

```bash
# Create backups directory if not exists
mkdir -p db/backups

# Execute backup
pg_dump -d workhub_dev -F c -b -v -f db/backups/workhub_dev_$(date +%Y%m%d_%H%M%S).dump
```

## 3. Restoration Procedure to Clean Database (pg_restore)

To verify restore capability on a new clean database:

```bash
# 1. Create a clean test database
createdb workhub_restore_test

# 2. Restore backup
pg_restore -d workhub_restore_test -v db/backups/workhub_dev_latest.dump

# 3. Verify integrity
psql -d workhub_restore_test -c "
SELECT table_name, (xpath('/row/cnt/text()', xml_count))[1]::text::int AS row_count
FROM (
  SELECT table_name, query_to_xml(format('SELECT COUNT(*) AS cnt FROM %I', table_name), false, false, '') AS xml_count
  FROM information_schema.tables
  WHERE table_schema = 'public'
) t;
"
```
