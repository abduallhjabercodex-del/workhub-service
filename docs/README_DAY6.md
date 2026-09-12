# مخرجات اليوم السادس — Relational Modeling and SQL

تم في هذا اليوم تصميم نموذج PostgreSQL لمشروع WorkHub وبناء migration وseed واختبارات قيود قابلة للتكرار.

## ما تم إنجازه

يشمل النموذج جداول `users` و`projects` و`memberships` و`tasks` و`tags` و`task_tags` و`audit_events`. أضيفت المفاتيح الأساسية والخارجية، القيود الخاصة بالحالات والأدوار والأولوية، الفهارس، حقول timestamps، وخيارات soft-delete والتدقيق.

## الأدلة

| الدليل | المسار |
|---|---|
| ER diagram المصدر | `docs/diagrams/workhub-relational-model.mmd` |
| ER diagram الصورة | `docs/diagrams/workhub-relational-model.png` |
| Migration صاعدة | `db/migrations/001_init.up.sql` |
| Migration عكسية | `db/migrations/001_init.down.sql` |
| Seed | `db/seed/001_demo.sql` |
| أمثلة SQL آمنة | `db/queries/day6_examples.sql` |
| اختبارات القيود | `db/tests/constraints.sql` |
| نتائج التشغيل | `docs/day6-evidence.md` |
| شرح التصميم | `docs/day6-relational-modeling.md` |

## معيار القبول

نجحت migration من قاعدة فارغة على PostgreSQL 16.15، ونجح seed، كما نجحت اختبارات uniqueness للبريد، وقيم status وpriority، والمفتاح المركب للعضويات، والمفاتيح الخارجية. اختبارات القيود تنتهي بـ`ROLLBACK` ولا تترك بيانات اختبار إضافية.

## التشغيل

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/migrations/001_init.up.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/seed/001_demo.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/constraints.sql
```

ينتقل اليوم السابع من هذا العقد العلاقي إلى PostgreSQL repository وtransaction boundaries، مع الحفاظ على domain models وواجهات الخدمة.
