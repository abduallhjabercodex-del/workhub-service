# 🚀 WorkHub API Service — Document & Operations Guide

![Go Version](https://img.shields.io/badge/Go-1.25.0-00ADD8?style=flat&logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat&logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7.0-DC382D?style=flat&logo=redis)
![Architecture](https://img.shields.io/badge/Architecture-Modular%20Monolith-blueviolet)
![License](https://img.shields.io/badge/Status-Production--Ready-brightgreen)

مرحباً بك في وثيقة التوثيق والتشغيل الشاملة لخدمة **WorkHub API**. تم إعداد هذا الملف ليكون مرجعك الكامل والمباشر لتفهم معمارية المشروع، طريقة تشغيله، بناء قواعد البيانات، تشغيل الاختبارات، والتعامل مع واجهات البرمجة (REST APIs) دون أي غموض أو مشكلات.

---

## 📋 جدول المحتويات (Table of Contents)
1. [📌 نبذة عن المشروع (Project Overview)](#-نبذة-عن-المشروع-project-overview)
2. [🏗️ البنية والمعمارية البرمجية (System Architecture)](#️-البنية-والمعمارية-البرمجية-system-architecture)
3. [⚙️ المتطلبات الأساسية والتعديل البيئي (Prerequisites & Setup)](#️-المتطلبات-الأساسية-والتعديل-البيئي-prerequisites--setup)
4. [🗄️ إعداد قاعدة البيانات والمايجريشن (Database & Migrations)](#-إعداد-قاعدة-البيانات-والمايجريشن-database--migrations)
5. [🚀 كيفية التشغيل (How to Run)](#-كيفية-التشغيل-how-to-run)
6. [📡 دليل واجهات API والتوثيق التفاعلي (REST APIs & Swagger)](#-دليل-واجهات-api-والتوثيق-التفاعلي-rest-apis--swagger)
7. [⚡ التخزين المؤقت، الرسائل، والعمال الخفيين (Cache, Messaging & Workers)](#-التخزين-المؤقت-الرسائل-والعمال-الخفيين-cache-messaging--workers)
8. [🧪 دليل الاختبارات وتغطية الكود (Testing & Quality)](#-دليل-الاختبارات-وتغطية-الكود-testing--quality)
9. [📁 بنية المشروع المجلدية (Directory Structure)](#-بنية-المشروع-المجلدية-directory-structure)
10. [🛠️ استكشاف الأخطاء وإصلاحها (Troubleshooting & FAQs)](#️-استكشاف-الأخطاء-وإصلاحها-troubleshooting--faqs)

---

## 📌 نبذة عن المشروع (Project Overview)

**WorkHub Backend Service** هو نظام خلفي متكامل لإدارة المشاريع والمهام متعدد المستخدِمين (Multi-User Project Management System). تم تطويره بلغة **Go** ليكون نموذجاً تطبيقياً فائق الأداء والاعتمادية للمشاريع الإنتاجية.

### 🌟 المميزات الرئيسية:
* **معمارية متعددة الطبقات (Clean Layered Architecture):** فصل كلي بين منطق العمل (Domain Logic) وطبقات النقل والتخزين.
* **إدارة المعاملات الذرية (ACID Transactions):** ضمان حفظ البيانات وسجلات التدقيق (`audit_events`) في معاملة واحدة وتراجعها الآلي عند حدوث أي خطأ (`Rollback`).
* **التخزين المؤقت الذكي (Redis Cache-Aside):** تسريع استعلامات ملخصات المشاريع بـ TTL مع الإبطال الفوري (`Cache Invalidation`) عند التعديل.
* **نظام الأحداث والعمال الخفيين (Pub/Sub Messaging & Background Workers):** وسيط أحداث آمن تزامناً ومعالجة الإشعارات خلفياً مع منع التكرار (`Idempotency`).
* **التوثيق التفاعلي (Swagger UI & OpenAPI 3.0):** واجهة توثيق سريعة ومجربة متوفرة مباشرة عبر المتصفح.
* **مرونة التشغيل والتعافي (Resilience & Graceful Fallback):** يعمل النظام حتى لو كانت قاعدة البيانات أو Redis غير متاحة (عبر ذاكرة في الذاكرة RAM كبديل مؤقت).

---

## 🏗️ البنية والمعمارية البرمجية (System Architecture)

يعتمد المشروع على نمط **Modular Monolith** الموزع طبقاً لمبادئ Clean Architecture و SOLID:

```text
               +-------------------------------------------+
               |        HTTP Request / Clients             |
               +-------------------------------------------+
                                     |
                                     v
               +-------------------------------------------+
               |       Middleware (Log, ReqID, Timeout)    |
               +-------------------------------------------+
                                     |
                                     v
               +-------------------------------------------+
               |        HTTP Handlers & Swagger UI         |
               +-------------------------------------------+
                                     |
                                     v
               +-------------------------------------------+
               |          WorkHub Business Service         |
               +-------------------------------------------+
                /            |              \            \
               v             v               v            v
      +------------------+  +-------------------+  +--------------+  +------------------+
      | Postgres Repos   |  | Memory Repos      |  | Redis Cache  |  | Messaging Broker |
      | (PostgreSQL 16)  |  | (Fallback / Test) |  | Cache-Aside  |  | Event Fan-out    |
      +------------------+  +-------------------+  +--------------+  +------------------+
                                                                             |
                                                                             v
                                                                   +--------------------+
                                                                   | Worker Engine      |
                                                                   | (Idempotent Logs)  |
                                                                   +--------------------+
```

### اتجاه الاعتماديات (Dependency Flow):
```text
cmd/server ──> handler ──> service ──> repository / cache / messaging
                                │
                                └──> domain (Entities & Models)
```
* **`domain`**: يحتوي الكيانات والأنواع الأساسية فقط بدون أي اعتمادية خارجية.
* **`service`**: يحتوي كافة قواعد وإجراءات العمل (Use Cases).
* **`repository`**: يقدم واجهات (`Interfaces`) للتخزين ومُطبقة بالكامل عبر `PostgresRepository` على PostgreSQL.
* **`handler`**: يتعامل مع بروتوكول HTTP، التحقق من المدخلات، وتغليف الاستجابات.

---

## ⚙️ المتطلبات الأساسية والتعديل البيئي (Prerequisites & Setup)

### 1. البرمجيات المطلوبة:
* **Go**: إصدار `1.22+` (يُوصى بـ Go 1.25).
* **PostgreSQL**: إصدار `16+`.
* **Redis**: إصدار `7.0+` (اختياري، يوجد Fallback في حال عدم توفره).
* **Git** و **curl** أو **Postman**.

### 2. إعداد ملف المتغيرات البيئية (`.env`):
قم بإنشاء أو مراجعة ملف [.env](file:///home/abdullah-jaber/Desktop/WorkHup/workhub-service/.env) في جذر المشروع:

```env
# إعدادات قاعدة البيانات
DATABASE_URL=postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber
DEFAULT_OWNER_ID=1

# إعدادات الكاش (Redis)
REDIS_URL=redis://127.0.0.1:6379/0
REDIS_CACHE_TTL=30s

# إعدادات خادم HTTP
HOST=127.0.0.1
PORT=8080
REQUEST_TIMEOUT=5s
```

---

## 🗄️ إعداد قاعدة البيانات والمايجريشن (Database & Migrations)

### 1. إنشاء قاعدة البيانات المحلية:
```bash
createdb workhub_dev
```

### 2. تطبيق ملفات المايجريشن (Database Migrations):
المشروع يحتوي على هجرات منظمة داخل مجلد `db/migrations`:

```bash
# تطبيق المايجريشن الأول (تأسيس الجداول والعلاقات والقيود)
psql -d workhub_dev -f db/migrations/001_init.up.sql

# تطبيق مايجريشن تحسين الأداء (إنشاء الفهارس المركبة والمغطية Index-Only Scan)
psql -d workhub_dev -f db/migrations/002_query_performance.up.sql

# تطبيق مايجريشن صندوق الرسائل (Outbox pattern)
psql -d workhub_dev -f db/migrations/003_messaging_outbox.up.sql
```

### 3. إدخال البيانات التجريبية (Seed Data):
```bash
psql -d workhub_dev -f db/seed/001_demo.sql
```

### 4. اختبار القيود والتصميم الوقائي (Constraint Tests):
لتجربة قوة قيود قاعدة البيانات (`CHECK Constraints`, `Foreign Keys`):
```bash
psql -d workhub_dev -f db/tests/constraints.sql
```

### 5. النسخ الاحتياطي وإعادة الاستعادة (Database Backup & Restore Procedures):

#### 📦 1. إنشاء نسخة احتياطية (Create Database Backup):
```bash
# إنشاء نسخة احتياطية بصيغة Custom Dump آمنة وشاملة للفهارس والبيانات
pg_dump -d workhub_dev -F c -b -v -f db/backups/workhub_dev_backup.dump
```

#### 🔄 2. استعادة النسخة الاحتياطية على قاعدة جديدة نظيفة (Restore Backup to Clean Database):
```bash
# أ) إنشاء قاعدة بيانات فارغة ونظيفة
createdb workhub_restore_test

# ب) استعادة البيانات الهيكلية والقيود بنجاح
pg_restore -d workhub_restore_test -v db/backups/workhub_dev_backup.dump

# ج) التأكد من سلامة واستعادة كافة الجداول والبيانات
psql -d workhub_restore_test -c "SELECT count(*) FROM projects; SELECT count(*) FROM memberships;"
```

---

## 🚀 كيفية التشغيل (How to Run)

### 🔹 الخيار الأول: التشغيل المباشر باستخدام المتغيرات البيئية
```bash
cd /home/abdullah-jaber/Desktop/WorkHup/workhub-service

# قراءة .env وتشغيل السيرفر
export $(cat .env | grep -v '^#' | xargs) && go run ./cmd/server
```

### 🔹 الخيار الثاني: البناء ثم التشغيل (Recommended for Production)
```bash
# 1. بناء الملف التنفيذي
go build -o bin/server ./cmd/server

# 2. تشغيل السيرفر
export $(cat .env | grep -v '^#' | xargs) && ./bin/server
```

عند التشغيل بنجاح، ستشاهد السجلات المهيكلة التالية:
```json
{"time":"...","level":"INFO","msg":"using PostgreSQL repository","defaultOwnerID":1}
{"time":"...","level":"INFO","msg":"project summary cache enabled","ttl":"30s"}
{"time":"...","level":"INFO","msg":"server listening","address":"127.0.0.1:8080"}
```

---

## 📡 دليل واجهات API والتوثيق التفاعلي (REST APIs & Swagger)

### 📖 التوثيق التفاعلي عبر Swagger UI:
بمجرد تشغيل الخادم، يمكنك فتح التوثيق التفاعلي واختبار جميع الواجهات مستندياً مباشرة من المتصفح:
* **رابط الواجهة التفاعلية (Swagger UI):** `http://127.0.0.1:8080/swagger/`
* **رابط ملف OpenAPI JSON:** `http://127.0.0.1:8080/swagger/doc.json`

---

### 🧪 أمثلة `curl` جاهزة للاستخدام والتجربة:

#### 1. فحص صحة الخدمة (Health Check):
```bash
curl -i http://127.0.0.1:8080/health
```

#### 2. إنشاء مشروع جديد (Create Project):
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "مشروع إدارة المهام الجديد"}'
```

#### 3. استعراض المشاريع (List Projects with Search & Pagination):
```bash
curl -i "http://127.0.0.1:8080/api/v1/projects?q=مشروع&sort=name&page=1&limit=10"
```

#### 4. جلب ملخص المشروع والتخزين المؤقت (Get Project Summary - Redis Cached):
```bash
# الطلب الأول: يجلب البيانات من Postgres ويكتبها في Redis (Cache Miss)
curl -i http://127.0.0.1:8080/api/v1/projects/1/summary

# الطلب الثاني: يجلب البيانات مباشرة من Redis بسرعة فائقة (Cache Hit)
curl -i http://127.0.0.1:8080/api/v1/projects/1/summary
```

#### 5. إنشاء مهمة جديدة (Create Task with Atomic Audit Log):
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/projects/1/tasks \
  -H "Content-Type: application/json" \
  -d '{"title": "كتابة التوثيق الهندسي للمشروع"}'
```

#### 6. استعراض المهام مع الفلترة والترتيب (Filter & Paginate Tasks):
```bash
curl -i "http://127.0.0.1:8080/api/v1/projects/1/tasks?status=todo&sort=created_at&page=1&limit=10"

curl -i "http://127.0.0.1:8080/api/v1/projects/1/tasks"
curl -i "http://127.0.0.1:8080/api/v1/tasks/100114"
```

#### 7. التحديث الجزئي لحالة المهمة (Patch Task Status):
```bash
curl -i -X PATCH http://127.0.0.1:8080/api/v1/tasks/1 \
  -H "Content-Type: application/json" \
  -d '{"status": "done"}'
```

#### 8. الحذف الناعم لمهمة (Soft Delete Task):
```bash
curl -i -X DELETE http://127.0.0.1:8080/api/v1/tasks/1
```

#### 9. ملخص وإحصائيات مشروع معين (Project Summary & Stats):
```bash
curl -i http://127.0.0.1:8080/api/v1/projects/1/summary
```

#### 10. استعراض كافة المهام الشاملة عبر جميع المشاريع (Global Tasks Listing):
```bash
curl -i "http://127.0.0.1:8080/api/v1/tasks?status=todo&sort=created_at&page=1&limit=10"
```

---

## ⚡ التخزين المؤقت، الرسائل، والعمال الخفيين (Cache, Messaging & Workers)

1. **إستراتيجية Redis Cache-Aside:**
   * يتم تخزين ملخص المشروع على مفتاح `workhub:project-summary:{project_id}` مع TTL مدته 30 ثانية.
   * عند إضافة أو تعديل أو حذف أي مهمة تابعة للمشروع، يتم استدعاء `InvalidateProjectSummary` فوراً لحذف الكاش وضمان عدم تقديم بيانات قديمة (Stale Data).

2. **وسيط الرسائل (In-Memory Event Broker):**
   * كود مخصص في [broker.go](file:///home/abdullah-jaber/Desktop/WorkHup/workhub-service/internal/messaging/broker.go) يدعم Publish/Subscribe بقنوات تزامنية آمنة 100% من Data Races.
   * يحتوي على قائمة الرسائل الميتة (Dead-Letter Queue - DLQ) لمعالجة الرسائل التي تفشل معالجتها.

3. **العامل الخفي (Notification Worker):**
   * يستمع العامل الخفي لحدث `task.created` وينفذ عمليات معالجة خلفية.
   * يضمن خاصية **Idempotency** باستخدام `DeliveryStore` للتحقق من `AlreadyDelivered` لمنع تكرار الإشعارات.

---

## 🧪 دليل الاختبارات وتغطية الكود (Testing & Quality)

تم إعداد المشروع بمجموعة اختبارات شاملة تغطي اختبارات الوحدات (Unit Tests) واختبارات التصفح والتكامل (Integration Tests):

### 1. تشغيل جميع اختبارات المشروع:
```bash
go test -v ./...
```

### 2. فحص سباق البيانات والتزامن (Data Race Detection):
```bash
go test -v -race ./...
```

### 3. تشغيل اختبارات التكامل مع PostgreSQL الحقيقية:
```bash
export WORKHUB_TEST_DATABASE_URL="postgresql:///workhub_dev?host=/var/run/postgresql&user=abdullah-jaber"
go test ./internal/repository -run 'TestPostgresRepository' -v
```

### 4. توليد تقرير تغطية الكود (Code Coverage Profile):
```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 📁 بنية المشروع المجلدية (Directory Structure)

```text
workhub-service/
├── cmd/
│   └── server/
│       └── main.go                 # نقطة الانطلاق الرئيسية وحاقن الاعتماديات (DI Container)
├── internal/
│   ├── cache/                      # إدارة كاش رديس (Project Summary Cache & Invalidation)
│   ├── config/                     # قراءة وتحليل وإدارة المتغيرات البيئية
│   ├── domain/                     # نماذج الكيانات والأخطاء المستقلة (Core Entities)
│   ├── handler/                    # مسارات REST API والتحقق وسيرفر Swagger UI
│   │   ├── api.go                  # نقاط الـ HTTP REST Endpoints
│   │   ├── health.go               # نقطة فحص الصحة /health
│   │   ├── openapi.yaml            # مواصفات العقد القياسي OpenAPI 3.0
│   │   └── swagger.go              # تقديم التوثيق التفاعلي عبر /swagger/
│   ├── messaging/                  # وسيط الأحداث التزامني (Pub/Sub Broker & DLQ)
│   ├── middleware/                 # الطبقات المساعدة (JSON Logger, Request-ID, Timeouts)
│   ├── repository/                 # طبقة التخزين الأساسية (Postgres SQL Pool)
│   ├── service/                    # قواعد وإجراءات العمل الأساسية (Business Use Cases)
│   └── worker/                     # العمال الخفيون والتحقق من عدم تكرار الرسائل (Idempotency)
├── db/
│   ├── migrations/                 # ملفات مايجريشن SQL الهيكلية والتحسينية (001, 002, 003)
│   ├── queries/                    # استعلامات سريعة وأمثلة تحليلية
│   ├── seed/                       # بيانات البذر التجريبية (001_demo.sql)
│   └── tests/                      # سكريبت اختبار قيود قواعد البيانات (constraints.sql)
├── docs/                           # وثائق الأيام السابقة والأدلة الشاملة والرسوم التوضيحية
├── docs2/                          # كتاب الأوامر التنفيذي ونتائج قياس الأداء
├── .env                            # ملف المتغيرات البيئية للتطوير المحلي
├── go.mod / go.sum                 # شجرة اعتماديات بلغة Go
└── README.md                       # هذا الملف (دليل التشغيل والتوثيق الموحد)
```

---

## 🛠️ استكشاف الأخطاء وإصلاحها (Troubleshooting & FAQs)

### ❓ 1. المشكلة: `database is not reachable` عند بدء تشغيل الخدمة
* **السبب:** عدم تشغيل PostgreSQL أو وجود خطأ في اسم المستخدم أو المسار بـ `DATABASE_URL`.
* **الحل:**
  1. تأكد من تشغيل خادم Postgres: `sudo systemctl status postgresql`.
  2. جرب الاتصال مباشرة: `psql -d workhub_dev`.
  3. تأكد من ضبط متغير البيئة `DATABASE_URL` بشكل صحيح في `.env` لأن قاعدة البيانات PostgreSQL إلزامية الآن.

### ❓ 2. المشكلة: `redis unavailable; continuing without cache`
* **السبب:** خادم Redis غير شغال على المنفذ `6379`.
* **الحل:** هذه رسالة تحذير طبيعية (Graceful Fallback)، السيرفر **لن ينهار** بل سيواصل الخدمة مستعيناً بـ PostgreSQL مباشرة. لتشغيل Redis: `sudo systemctl start redis`.

### ❓ 3. المشكلة: `address already in use` على المنفذ `8080`
* **السبب:** يوجد تطبيق آخر يستمع على المنفذ `8080`.
* **الحل:** يمكنك تغيير المنفذ بسهولة عبر التمرير أثناء التشغيل:
  ```bash
  PORT=8085 go run ./cmd/server
  ```

---

