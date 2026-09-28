# Day 6 Evidence — PostgreSQL relational model

## Environment

The schema was verified against PostgreSQL 16.15 on Ubuntu. A disposable database named `workhub_day6` was created from an empty state.

## Commands executed

```bash
sudo -u postgres createdb workhub_day6
sudo -u postgres psql -v ON_ERROR_STOP=1 -d workhub_day6 -f db/migrations/001_init.up.sql
sudo -u postgres psql -v ON_ERROR_STOP=1 -d workhub_day6 -f db/seed/001_demo.sql
sudo -u postgres psql -v ON_ERROR_STOP=1 -d workhub_day6 -f db/tests/constraints.sql
```

## Results

The migration completed successfully. The seed completed successfully and produced the following baseline rows:

| table | rows |
|---|---:|
| `users` | 2 |
| `projects` | 1 |
| `tasks` | 2 |
| `tags` | 1 |

The constraint script completed with `ROLLBACK`, leaving the demo database clean after the test transaction. The following cases passed:

| Constraint | Result |
|---|---|
| Case-insensitive active user email uniqueness | PASS |
| Task status allow-list | PASS |
| Task priority range | PASS |
| Composite membership primary key | PASS |
| `task_tags` foreign keys | PASS |

## Evidence interpretation

The database rejects invalid status and priority values at the database boundary, not only in Go validation. Foreign keys protect project, user, task, and tag relationships. Composite keys prevent duplicate memberships and duplicate task labels. The migration can start from a clean database, and the seed is deterministic for local development.

The next day will add the PostgreSQL repository and transaction boundaries. This schema is therefore the source-of-truth contract for mapping database rows into WorkHub domain models.
