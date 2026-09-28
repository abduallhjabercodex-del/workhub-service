# 🚀 الدليل الشامل لتشغيل وااختبار ومناقشة مشروع WorkHub (الأيام 1 إلى 8)

مرحباً بك! هذا الدليل مُصمم خصيصاً ليكون **مرجعك التنفيذي الكامل** لتشغيل واختبار واقتطاف نتائج مشروع **WorkHub Backend Service** من اليوم الأول وحتى اليوم الثامن، مع **نقاط الشرح الذهبية أمام لجنة المناقشة**.

---

## 📌 الصورة العامة للمشروع (Project Architecture Overview)

* **اسم النظام:** WorkHub Multi-User Project Management Backend.
* **النمط المعماري:** **Modular Monolith** مبني بلغة Go بتصميم Clean Architecture (Layered Design).
* **المكونات الرئيسية:**
  1. **HTTP REST API:** لتقديم خدمات الإدارة وإرجاع استجابات JSON قياسية.
  2. **PostgreSQL 16:** **مصدر الحقيقة الوحيد (Single Source of Truth)** لتخزين البيانات والعلاقات وتنفيذ معاملات ACID.
  3. **Redis:** طبقة التخزين المؤقت السريعة (**Cache-Aside Pattern**) لخدمة الاستعلامات المكررة في أجزاء من الملي ثانية.
  4. **مجموعة الاختبارات:** Unit Tests, Integration Tests, Constraint Verification, و Mock Redis Tests.

---

## 📅 اليوم الأول إلى الخامس (Days 1–5): التأسيس، طبقة الـ HTTP، وهندسة Go النظيفة

### 🧠 ما تم إنجازه في هذه المرحلة:
1. **تأسيس النمط المعماري Clean Layered Architecture:**
   * `cmd/server/main.go` ⬅️ المدخل وحاوية حقن الاعتماديات (Dependency Injection Container).
   * `internal/domain/models.go` ⬅️ الكيانات المستقلة (`Project`, `Task`, `AuditEvent`, `ProjectSummary`).
   * `internal/handler/api.go` ⬅️ مسارات الـ REST API والتحقق من المدخلات ومسار الصحـة `/health`.
   * `internal/service/workhub.go` ⬅️ قواعد وإجراءات العمل (Business Logic Layer).
   * `internal/middleware/http.go` ⬅️ السجلات المهيكلة JSON, المعرفات الفريدة للطلبات `X-Request-ID`, والمهلات الحاكمة.
   * `internal/repository/memory.go` ⬅️ التخزين المؤقت بالذاكرة العشوائية كـ Fallback أولي.
2. **توثيق الواجهات:** بناء ملف العقد القياسي `docs/openapi.yaml`.

### 💻 أوامر التشغيل والاختبار الحية:
```bash
# الانتقال لمجلد المشروع
cd /home/abdullah-jaber/Desktop/WorkHup/workhub-service

# 1. تشغيل اختبارات الوحدات لطبقات الـ Handler والـ Service والـ Middleware
go test ./internal/handler ./internal/service ./internal/middleware -v

# 2. تشغيل فحص السلوك التزامني والسباق (Race Detector)
go test -race ./...
```

### 🗣️ ماذا تقول للجنة المناقشة عن الأيام 1-5؟
> *"بدأنا المشروع في الأيام الأولى ببناء الهيكل النظيف (Clean Architecture) وفصل طبقة الـ HTTP عن قواعد العمل (Domain Logic) لتطبيق مبادئ SOLID. صممنا الخدمة لتكون قابلة للاختبار والتطوير، مع دعم الـ Structured Logging و Request IDs لتتبع الطلبات، وتأمين التخزين المؤقت بالذاكرة كـ Fallback حتى ربط قاعدة البيانات."*

---

## 📅 اليوم السادس (Day 6): النموذج العلاقي وقواعد بيانات PostgreSQL

### 🧠 ما تم إنجازه في هذه المرحلة:
1. **تأسيس المخطط العلاقي (7 جداول محكمة):**
   * `users`: المستخدمون ومصادقة الهوية.
   * `projects`: المساحات والمشاريع.
   * `memberships`: العضويات مع **PRIMARY KEY مركب `(project_id, user_id)`** والأدوار RBAC.
   * `tasks`: المهام مع الأولويات والحالات والحذف الناعم `deleted_at`.
   * `tags`: الوسوم الملونة المحصورة بنطاق المشروع.
   * `task_tags`: جدول الربط لعلاقة **كثير-إلى-كثير (Many-to-Many)**.
   * `audit_events`: سجل التغييرات التاريخي.
2. **التصميم الوقائي (Defensive Database Design):**
   * إضافة قيود التحقق الصارمة: `CHECK (btrim(...) <> '')` لمنع النصوص الفارغة، و `CHECK (priority BETWEEN 1 AND 5)`.
   * استخدام السياسات التراكمية الحازمة: `ON DELETE CASCADE` و `ON DELETE RESTRICT` و `ON DELETE SET NULL`.
3. **ملفات المايجريشن واختبار القيود:**
   * `db/migrations/001_init.up.sql` و `001_init.down.sql`.
   * سكريبت اختبارات القيود `db/tests/constraints.sql` بلغة PL/pgSQL لتجربة كسر القيود عمداً.

### 💻 أوامر التشغيل والاختبار الحية:
```bash
# 1. إنشاء قاعدة البيانات المحلية
createdb workhub_dev

# 2. تركيب المايجريشن الرئيسي وتأسيس الجداول
psql -d workhub_dev -f db/migrations/001_init.up.sql

# 3. إدخال البيانات التجريبية الأولية
psql -d workhub_dev -f db/seed/001_demo.sql

# 4. تشغيل سكريبت اختبار القيود وتأكيد رفض البيانات الخاطئة
psql -d workhub_dev -f db/tests/constraints.sql
```

### 🗣️ ماذا تقول للجنة المناقشة عن اليوم السادس؟
> *"في اليوم السادس، صممنا المخطط العلاقي النظيف (Relational Schema) مع حماية خرسانية على مستوى قواعد البيانات بـ CHECK Constraints لحظر النصوص الفارغة والأرقام غير المسموحة. وطبقنا سياسات Foreign Keys متنوعة لحماية المرجعية (مثل منع حذف منشئ المهمة وتصفير المكلّف وحذف الوسوم تلقائياً). كما كتبنا سكريبت اختبار قيود ينتهي بـ ROLLBACK للتأكد من رفض Postgres للبيانات الفاسدة دون تلوث قاعدة البيانات."*

---

## 📅 اليوم السابع (Day 7): ربط PostgreSQL، المعاملات الذرية، وخصائص ACID

### 🧠 ما تم إنجازه في هذه المرحلة:
1. **الربط كودياً بـ PostgreSQL Repository:**
   * إنشاء `internal/repository/postgres.go` باستخدام `github.com/jackc/pgx/v5` ودعم Connection Pool عبر `pgxpool.Pool`.
2. **تطبيق خاصية Atomicity في ACID (الدالة `CreateTaskWithAudit`):**
   * كتابة المهمة وسجل التدقيق داخل معاملة ذرية واحدة (`tx.BeginTx`).
   * استخدام حماية التراجع التلقائي `defer tx.Rollback(ctx)` لضمان عدم حفظ أي مهمة بدون أثر تدقيق في حال فشل إدخال الـ Audit.
3. **حدود تحويل الأخطاء (Error Boundaries):**
   * كتابة دالة `mapDBError` لترجمة أخطاء Postgres المباشرة (مثل `23505` و `23503`) إلى Sentinel Errors مثل `ErrConflict` و `ErrConstraint` و `ErrNotFound`.
4. **اختبارات التصفح والتكامل:**
   * إنشاء `internal/repository/postgres_integration_test.go` لتجربة نجاح الـ Commit وفشل الـ Rollback عملياً.

### 💻 أوامر التشغيل والاختبار الحية:
```bash
# تشغيل اختبارات التصفح والتكامل الخاصة بـ PostgreSQL
export WORKHUB_TEST_DATABASE_URL="postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber"
go test ./internal/repository -run 'TestPostgresRepository' -count=1 -v
```

### 🗣️ ماذا تقول للجنة المناقشة عن اليوم السابع؟
> *"في اليوم السابع، قمنا بالربط البرمجي الحقيقي مع PostgreSQL، وطبقنا خواص ACID الأربعة. أبرز ما قمنا به هو الدالة الذرية `CreateTaskWithAudit`؛ حيث نحفظ المهمة وسجل التدقيق داخل Transaction واحدة بـ Commit و Rollback محكمين، مما يضمن ألا تُحفظ مهمة يتيمة إذا فشل سجل التدقيق. كما بنينا طبقة ترجمة أخطاء `mapDBError` تمنع تسريب تفاصيل قواعد البيانات الداخلية للـ API."*

---

## 📅 اليوم الثامن (Day 8): أداء الاستعلامات، الفهارس المغطية، والتخزين المؤقت بـ Redis

### 🧠 ما تم إنجازه في هذه المرحلة:
1. **تحسين أداء الاستعلامات (Query Performance Tuning):**
   * قياس خطة استعلام ملخص المشروع باستخدام `EXPLAIN ANALYZE`.
   * إضافة المايجريشن `db/migrations/002_query_performance.up.sql` بإنشاء **Covering Index**:
     ```sql
     CREATE INDEX tasks_project_summary_idx ON tasks (project_id) INCLUDE (status) WHERE deleted_at IS NULL;
     ```
   * خفض التكلفة التقديرية للاستعلام من `37.99` إلى `16.99` وزمن التنفيذ من `0.146 ms` إلى `0.102 ms` بفضل تفعيل **`Index-Only Scan`**.
2. **التخزين المؤقت النظيف (Redis Cache-Aside Pattern):**
   * إنشاء `internal/cache/project_summary.go` لقراءة ملخص المشروع من Redis أولاً (`Cache Hit`)، وعند غيابه (`Cache Miss`) يقرأ من PostgreSQL ويكتب في Redis بـ TTL قدره 30 ثانية.
   * دعم **إبطال الكاش (Invalidation)** فوراً عند إضافة أو تعديل مهمة لمنع عرض بيانات قديمة.
3. **الفشل الآمن والمرونة (Resilience / Fail-Safe):**
   * عند تعطل خادم Redis، لا ينهار السيرفر بل يطبع تحذيراً وتستمر الخدمة بسلام عبر PostgreSQL كمصدر الحقيقة الوحيد.

### 💻 أوامر التشغيل والاختبار الحية:

```bash
# 1. تشغيل اختبارات وحدات الـ Cache (تستخدم miniredis وهمياً بالذاكرة)
go test ./internal/cache -v

# 2. تركيب مايجريشن الأداء لليوم الثامن على قاعدة البيانات
psql -d workhub_dev -f db/migrations/002_query_performance.up.sql

# 3. تشغيل السيرفر الكامل ببيانات الـ .env
export $(cat .env | xargs) && go run ./cmd/server
```

**في نافذة تيرمينال أخرى، قُم باختبار الـ APIs وتتبع السجلات:**
```bash
# أ. فحص صحة الخدمة
curl -i http://127.0.0.1:8080/health

# ب. جلب قائمة المشاريع من PostgreSQL
curl -i http://127.0.0.1:8080/api/v1/projects

# ج. جلب ملخص المشروع (الطلب الأول -> Cache Miss، الطلب الثاني -> Cache Hit)
curl -i http://127.0.0.1:8080/api/v1/projects/1/summary
curl -i http://127.0.0.1:8080/api/v1/projects/1/summary
```

### 🗣️ ماذا تقول للجنة المناقشة عن اليوم الثامن؟
> *"في اليوم الثامن، ركزنا على تحسين الأداء والتخزين المؤقت. أضفنا الفهرس المتقدم `tasks_project_summary_idx` مع حقل `INCLUDE (status)` لتفعيل الـ Index-Only Scan وخفض تكلفة الاستعلام لأكثر من 50%. كما طبقنا نمط Cache-Aside بـ Redis مع TTL قدره 30 ثانية وإبطال آلي عند التعديل، مع ضمان أن PostgreSQL تظل مصدر الحقيقة الأبدي حتى لو انقطع اتصال Redis."*

---

## 🛠️ جدول الأوامر السريعة الشامل (Master Quick Reference Command Sheet)

إليك الجدول الذي يمكنك استخدامه كـ Cheat-Sheet أثناء العرض التقديمي:

| اليوم | الهدف والمفهوم | الأمر التنفيذي للتجربة في التيرمينال |
| :---: | :--- | :--- |
| **الأيام 1–5** | Clean Architecture, Handlers, Middlewares | `go test ./internal/handler ./internal/service ./internal/middleware -v` |
| **اليوم 6** | Relational Models & SQL Constraints | `psql -d workhub_dev -f db/tests/constraints.sql` |
| **اليوم 7** | ACID Transactions & Integration Tests | `export WORKHUB_TEST_DATABASE_URL="postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber" && go test ./internal/repository -run 'TestPostgresRepository' -v` |
| **اليوم 8** | Redis Cache-Aside & Unit Tests | `go test ./internal/cache -v` |
| **التشغيل الشامل** | Full Application Runtime with PostgreSQL & .env | `export $(cat .env | xargs) && go run ./cmd/server` |
| **فحص الأداء** | Full Test Coverage Suite | `go test ./...` |

---

### 💡 نصيحة ختامية للمناقشة:
عند سؤالك من اللجنة عن سر قوة هذا المشروع، أجبهم بثقة:
> *"مشروع WorkHub لم يُبنَ فقط ليكون مجرد API يعيد بيانات، بل تم تصميمه كـ Production-Ready Service تحمي اتساق البيانات بـ ACID المعاملاتي، وتسرّع الأداء بـ Covering Indexes والـ Redis Cache-Aside، وتتحمل الأعطال بـ Graceful Degradation و Error Mapping محكم."*
