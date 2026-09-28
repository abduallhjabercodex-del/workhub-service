# اليوم السابع — Repositories, Transactions, and Consistency

## الهدف

استبدل WorkHub التخزين In-Memory بمستودع PostgreSQL قابل للحقن، مع `pgxpool` لإدارة connection pool، وتمرير `context.Context` إلى كل عملية قاعدة بيانات، وتحويل أخطاء PostgreSQL إلى أخطاء persistence مفهومة للطبقات الأعلى.

## ما تم تنفيذه

| المجال | التنفيذ |
|---|---|
| Driver and pool | `github.com/jackc/pgx/v5` و`pgxpool.Pool` مع `Ping` عند بدء الخدمة. |
| Repository boundary | `ProjectRepository` و`TaskRepository` بواجهات لا تعتمد على HTTP أو PostgreSQL مباشرة. |
| Error mapping | `ErrNotFound` و`ErrConflict` و`ErrConstraint` مع الحفاظ على `context.Canceled` و`context.DeadlineExceeded`. |
| Transaction | `CreateTaskWithAudit` ينشئ المهمة وسجل `audit_events` داخل transaction واحدة. |
| Rollback | فشل audit constraint يؤدي إلى rollback ولا يترك task orphan. |
| Identity IDs | PostgreSQL يولد IDs عبر identity columns، والrepository يعيد mapping إلى domain string IDs. |
| Pagination | `page` و`limit` يمران إلى repository ويطبقان في SQL عبر `LIMIT/OFFSET`. |
| Runtime | `DATABASE_URL` يشغل PostgreSQL، وغيابه يبقي fallback In-Memory للتجارب البسيطة. |

## نطاق transaction

العملية الذرية هي:

```text
BEGIN
  INSERT INTO tasks
  INSERT INTO audit_events
COMMIT
```

إذا فشل الإدخال الثاني، لا تُحفظ المهمة الأولى. هذا يمنع وجود task بلا أثر تدقيق، وهو تطبيق عملي لفكرة atomicity من ACID. لا نحاول ادعاء exactly-once على مستوى شبكة موزعة؛ حدود transaction هنا هي قاعدة PostgreSQL الواحدة.

## اختبارات التكامل

الملف `internal/repository/postgres_integration_test.go` يشغّل الاختبارات عند تحديد:

```bash
export WORKHUB_TEST_DATABASE_URL='postgresql:///workhub_day7?host=/var/run/postgresql&user=ubuntu'
go test ./internal/repository -run 'TestPostgresRepository' -count=1 -v
```

الاختبارات تغطي:

1. Commit ذري: وجود task وaudit بعد نجاح العملية.
2. Rollback: فشل audit بسبب constraint وعدم بقاء task.
3. Mapping لـnot found.
4. احترام context timeout/cancellation.
5. وجود ملفات migration المطلوبة.

## التشغيل باستخدام PostgreSQL

بعد تشغيل migration وseed:

```bash
export DATABASE_URL='postgresql:///workhub_day7?host=/var/run/postgresql&user=ubuntu'
export DEFAULT_OWNER_ID=1
go run ./cmd/server
```

ثم اختبر:

```bash
curl -i http://127.0.0.1:8080/health
curl -i http://127.0.0.1:8080/api/v1/projects
```

إذا لم تحدد `DATABASE_URL`، يسجل التطبيق تحذيرًا ويستخدم In-Memory fallback. هذا مفيد للاختبارات القديمة، لكن التشغيل المقصود من اليوم السابع يستخدم PostgreSQL.

## نتائج التحقق

نجحت الأوامر التالية:

```bash
go test ./...
go vet ./...
WORKHUB_TEST_DATABASE_URL="$WORKHUB_TEST_DATABASE_URL" go test ./internal/repository -run 'TestPostgresRepository' -count=1 -v
go build -o bin/server ./cmd/server
```

كما نجحت اختبارات PostgreSQL الفعلية على PostgreSQL 16.15 لكل من commit وrollback وtimeout وnot-found mapping.
