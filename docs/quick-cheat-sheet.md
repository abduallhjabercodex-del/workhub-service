# دليل أوامر تشغيل واختبار واستعراض مشروع WorkHub API
*(WorkHub Developer Cheat Sheet)*

هذا الملف يحتوي على أهم الأوامر اليومية وسيناريوهات الاستخدام السريعة لتشغيل الخادم، تنفييذ المايجريشن، تشغيل الاختبارات، واستعراض الواجهات والنظام.

---

## 1. تشغيل الخادم والتطبيق (Server Commands)

### تشغيل التطبيق بالبيئة المحلية:
```bash
# تحميل متغيرات البيئة وتغيل الخادم
source .env && go run cmd/server/main.go
```

### تشغيل الخادم مع تفعيل التتبع الموزع (OpenTelemetry Tracing):
```bash
ENABLE_TRACING=true source .env && go run cmd/server/main.go
```

---

## 2. إدارة البنية التحتية والخدمات المساندة (Docker Compose)

```bash
# تشغيل جميع الخدمات (PostgreSQL, Redis, Prometheus, Grafana) في الخلفية
docker compose up -d

# إيقاف وعرض حالة الخدمات
docker compose ps
docker compose down
```

---

## 3. المايجريشن وقواعد البيانات (Database & Migrations)

### الاتصال المباشر بقاعدة البيانات عبر `psql`:
```bash
psql "postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber"
```

### تنفيذ ملف مايجريشن معين مباشرة:
```bash
# تنفيذ الترقية (Up) لملف المؤشرات Indexes:
psql "postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber" -f db/migrations/0006_indexes.up.sql

# تنفيذ الترقية لملف المشغلات Triggers:
psql "postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber" -f db/migrations/0007_updated_at_triggers.up.sql
```

### تشغيل المايجريشن باستخدام أداة `migrate` CLI:
```bash
# تطبيق جميع المايجريشن الصاعدة:
migrate -path db/migrations -database "postgres://postgres:postgres@localhost:5432/workhub_dev?sslmode=disable" up

# التراجع خطوة واحدة للخلف:
migrate -path db/migrations -database "postgres://postgres:postgres@localhost:5432/workhub_dev?sslmode=disable" down 1

# الانتقال لإصدار محدد (مثلاً إصدار 4):
migrate -path db/migrations -database "postgres://postgres:postgres@localhost:5432/workhub_dev?sslmode=disable" goto 4
```

---

## 4. تشغيل الاختبارات والفحوصات الشاملة (Testing Commands)

```bash
# 1. تشغيل كافة اختبارات الوحدات (Unit Tests)
go test ./...

# 2. تشغيل كاشف سباق البيانات بالتزامن (Race Detector)
go test -race -count=1 ./...

# 3. تشغيل اختبارات اختراق وإساءة الاستخدام (Abuse & Security Tests)
go test -v ./tests/abuse/...

# 4. تشغيل اختبارات محاكاة انقطاع الاعتماديات (Dependency Failure Tests)
go test -v ./tests/dependency/...
```

---

## 5. روابط الواجهات والمراقبة في المتصفح (Web URLs)

| الخدمة / الواجهة       | الرابط في المتصفح                                | الوصف                                         |
| :--------------------- | :----------------------------------------------- | :-------------------------------------------- |
| **Swagger UI Portal**  | `http://localhost:8080/api/v1/docs`              | واجهة تجربة واختبار نقاط النهاية تفاعلياً     |
| **OpenAPI Schema**     | `http://localhost:8080/api/v1/docs/openapi.yaml` | المخطط المكتمل بصيغة OpenAPI 3.0              |
| **Liveness Probe**     | `http://localhost:8080/health`                   | فحص حياة التطبيق (`200 OK`)                   |
| **Readiness Probe**    | `http://localhost:8080/ready`                    | فحص جاهزية الخادم والاتصال بقاعدة البيانات    |
| **Prometheus Metrics** | `http://localhost:8080/metrics`                  | مقاييس الأداء والمراقبة (RED/USE Metrics)     |
| **Grafana Dashboards** | `http://localhost:3000`                          | لوحات مراقبة الأداء التفاعلية (admin / admin) |

---

## 6. سيناريو تجربة وسلسلة طلبات سريعة عبر `curl` (Quick Demo Flow)

### أ. تسجيل الدخول والحصول على التوكن (Login):
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@example.com","password":"Secret123!"}'
```

### ب. إنشاء مشروع جديد (Create Project):
```bash
curl -s -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer <YOUR_ACCESS_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name":"مشروع الانطلاق الفعلي"}'
```

### جـ. إضافة عضو للمشروع وتحديد صلاحية RBAC (Add Project Member):
```bash
curl -s -X POST http://localhost:8080/api/v1/projects/1/members \
  -H "Authorization: Bearer <YOUR_ACCESS_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"userId":"2","role":"admin"}'
```

### د. إنشاء مهمة جديدة للمشروع (Create Task):
```bash
curl -s -X POST http://localhost:8080/api/v1/projects/1/tasks \
  -H "Authorization: Bearer <YOUR_ACCESS_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"title":"إعداد بيئة الإنتاج وتكوين الخادم","status":"todo"}'
```

### هـ. تسجيل الخروج وإبطال الجلسة (Logout):
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refreshToken":"<YOUR_REFRESH_TOKEN>"}'
```
