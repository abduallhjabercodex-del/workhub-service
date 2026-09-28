# مخرجات اليوم السابع — Repositories, Transactions, and Consistency

تم في هذا اليوم استبدال التخزين In-Memory بمستودع PostgreSQL قابل للحقن، وإضافة connection pool عبر `pgxpool`، وتمرير context deadlines، وتحويل أخطاء قاعدة البيانات، وتنفيذ transaction ذرية لإنشاء المهمة وسجل التدقيق.

## الملفات الأساسية

| الملف | الغرض |
|---|---|
| `internal/repository/postgres.go` | PostgreSQL repository وpool mapping وtransaction. |
| `internal/repository/postgres_integration_test.go` | اختبارات commit وrollback وtimeout وnot-found. |
| `internal/repository/memory.go` | fallback واختبارات الوحدات القديمة. |
| `internal/config/config.go` | `DATABASE_URL` و`DEFAULT_OWNER_ID`. |
| `cmd/server/main.go` | اختيار PostgreSQL عند وجود `DATABASE_URL`. |
| `docs/day7-evidence.md` | نتائج التشغيل الفعلية. |

## التشغيل

```bash
export DATABASE_URL='postgresql:///workhub_day7?host=/var/run/postgresql&user=ubuntu'
export DEFAULT_OWNER_ID=1
go run ./cmd/server
```

واختبارات التكامل:

```bash
export WORKHUB_TEST_DATABASE_URL="$DATABASE_URL"
go test ./internal/repository -run 'TestPostgresRepository' -count=1 -v
```

بدون `DATABASE_URL` يبقى fallback In-Memory متاحًا لتشغيل الاختبارات القديمة، لكن مسار اليوم السابع المقصود هو PostgreSQL.
