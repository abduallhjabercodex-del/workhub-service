# WorkHub API — Days 1–3

هذا المشروع التدريبي يطبّق متطلبات الأيام الثلاثة الأولى من برنامج **Backend Qualification**. وهو نموذج **Modular Monolith** صغير بلغة Go، يستخدم مستودعًا داخل الذاكرة، ويعرض عقد REST versioned تحت `/api/v1`.

## ما تم تنفيذه

| اليوم | المتطلبات المنفذة |
|---|---|
| 1 | خدمة Go، إعدادات، `/health`، build command، ومخطط مسار الطلب. |
| 2 | موارد ومفاهيم REST، HTTP methods، status codes، JSON envelopes، validation، API versioning، filtering، sorting، pagination metadata، وOpenAPI contract. |
| 3 | فصل Router/Handlers/Middleware/Services/Repositories، dependency injection، context propagation، request timeout، request IDs، structured JSON logs، panic recovery، وأخطاء المجال. |

## التشغيل

```bash
go run ./cmd/server
```

الإعدادات المدعومة:

```bash
HOST=127.0.0.1 PORT=8080 REQUEST_TIMEOUT=5s go run ./cmd/server
```

يفشل التطبيق مبكرًا إذا كان `PORT` خارج المجال `1..65535` أو كانت `REQUEST_TIMEOUT` غير صالحة.

## الاختبار والبناء

```bash
go test ./...
go build -o bin/server ./cmd/server
```

## أمثلة API

إنشاء مشروع:

```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"Demo Project"}'
```

إنشاء مهمة، بعد وضع معرّف المشروع في المتغير:

```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/projects/$PROJECT_ID/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"First task"}'
```

تصفية وترتيب المهام مع بيانات pagination:

```bash
curl -i 'http://127.0.0.1:8080/api/v1/projects/'"$PROJECT_ID"'/tasks?status=todo&q=first&sort=title&page=1&limit=20'
```

تحديث جزئي لمهمة:

```bash
curl -i -X PATCH http://127.0.0.1:8080/api/v1/tasks/$TASK_ID \
  -H 'Content-Type: application/json' \
  -d '{"status":"done"}'
```

كل الاستجابات JSON وتستخدم غلافًا موحدًا. مثال الخطأ:

```json
{"error":{"code":"invalid_input","message":"request validation failed","requestId":"..."}}
```

## بنية المشروع

```text
.
├── cmd/server/main.go
├── internal/config/config.go
├── internal/domain/models.go
├── internal/handler/health.go
├── internal/handler/api.go
├── internal/handler/api_test.go
├── internal/middleware/http.go
├── internal/repository/memory.go
├── internal/service/workhub.go
├── docs/openapi.yaml
├── docs/request-flow.mmd
├── docs/request-flow.png
├── go.mod
└── README.md
```

اتجاه الاعتماديات هو: `handler → service → repository`; أما `domain` فلا يعتمد على طبقات النقل. يتم تركيب الاعتماديات يدويًا في `cmd/server/main.go`، مما يجعل استبدال المستودع داخل الذاكرة بمستودع آخر ممكنًا دون تغيير حالات الاستخدام.

## الأدلة

ملف `docs/openapi.yaml` هو عقد REST المقترح قبل تنفيذ handlers. وملف `docs/request-flow.mmd` يوضح مسار الطلب. أما `internal/handler/api_test.go` فيثبت تدفق إنشاء مشروع، إنشاء مهمة، تحديثها، والتحقق من المدخلات. وتظهر سجلات التشغيل بصيغة JSON وتتضمن `request_id` لكل طلب.
