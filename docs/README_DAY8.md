# مخرجات اليوم الثامن — Query Performance and Caching

تم تنفيذ قياس أداء استعلامات PostgreSQL وإضافة فهرس مبرر، ثم إضافة Redis cache-aside لملخصات المشاريع مع TTL وinvalidation وفشل آمن.

## الملفات

| الملف | الغرض |
|---|---|
| `db/migrations/002_query_performance.up.sql` | فهرس `tasks_project_summary_idx`. |
| `db/migrations/002_query_performance.down.sql` | التراجع عن الفهرس. |
| `docs/day8-explain-before.txt` | خطة ملخص المشروع قبل الفهرس. |
| `docs/day8-explain-after.txt` | الخطة بعد الفهرس على المشروع الكبير. |
| `docs/day8-explain-selective-before.txt` | قياس قبل الفهرس على مشروع انتقائي. |
| `docs/day8-explain-selective-after.txt` | قياس بعد الفهرس على مشروع انتقائي. |
| `internal/cache/project_summary.go` | Redis cache-aside وTTL وlogs. |
| `internal/cache/project_summary_test.go` | اختبارات miss/hit/TTL/invalidation/failure. |
| `internal/repository/postgres.go` | استعلام aggregation لملخص المشروع. |
| `internal/service/workhub.go` | دمج cache وإبطالها بعد mutation. |
| `docs/day8-query-performance-caching.md` | شرح القرارات والنتائج. |

## التشغيل

```bash
export DATABASE_URL='postgresql:///workhub_day8?host=/var/run/postgresql&user=ubuntu'
export REDIS_URL='redis://127.0.0.1:6379/0'
export REDIS_CACHE_TTL='30s'
go run ./cmd/server
```

ثم:

```bash
curl http://127.0.0.1:8080/api/v1/projects/1/summary
curl http://127.0.0.1:8080/api/v1/projects/1/summary
```

الطلب الأول miss والطلب الثاني hit. تحديث أي مهمة في المشروع يبطل مفتاح الملخص.

## التحقق

```bash
go test ./...
go test -race ./...
go vet ./...
```

اختبارات Redis تستخدم miniredis ولا تحتاج خدمة خارجية، بينما تشغيل API مع Redis الحقيقي موثق في `docs/day8-query-performance-caching.md`.
