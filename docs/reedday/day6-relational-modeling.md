# اليوم السادس — Relational Modeling and SQL

## الهدف

نقل WorkHub من نموذج In-Memory إلى أساس PostgreSQL قابل للتوسع، مع تصميم واضح للعلاقات والقيود وحقول التدقيق. هذه المرحلة لا تستبدل repository البرمجي بعد؛ بل تجهز schema وmigrations وSQL evidence التي سيستخدمها اليوم السابع.

## الكيانات

| الكيان | الغرض |
|---|---|
| `users` | هوية المستخدم وبياناته الأساسية. كلمة المرور تمثل hash فقط، وليس plaintext. |
| `projects` | مساحة العمل التي يملكها مستخدم. |
| `memberships` | علاقة many-to-many بين المستخدمين والمشاريع مع role. |
| `tasks` | وحدات العمل داخل المشروع مع status وpriority وassignee. |
| `tags` | وسوم خاصة بكل مشروع. |
| `task_tags` | علاقة many-to-many بين المهام والوسوم. |
| `audit_events` | سجل تدقيق append-oriented يصف الفعل والفاعل والهدف وpayload. |

## قرارات التصميم

استخدمنا `BIGINT GENERATED ALWAYS AS IDENTITY` للمفاتيح، ومفاتيح خارجية صريحة، و`TIMESTAMPTZ` للتواريخ. استخدمنا `deleted_at` كخيار soft-delete للمستخدمين والمشاريع والمهام، بينما تستخدم العضوية `removed_at` حتى نحافظ على أثر العضوية السابقة.

تم وضع قيود `CHECK` للحالات والأدوار والأولوية والأسماء غير الفارغة، وفهرس unique على `lower(email)` للمستخدمين غير المحذوفين منطقيًا. كما وضعت العلاقة `memberships` مفتاحًا مركبًا يمنع تكرار العضوية، ووضعت `task_tags` مفتاحًا مركبًا يمنع تكرار الوسم على المهمة نفسها.

## الملفات

| الملف | الغرض |
|---|---|
| `db/migrations/001_init.up.sql` | إنشاء الجداول والفهارس والقيود. |
| `db/migrations/001_init.down.sql` | إزالة migration في بيئة disposable. |
| `db/seed/001_demo.sql` | بيانات تجريبية حتمية للتطوير المحلي. |
| `db/queries/day6_examples.sql` | استعلامات JOIN وaggregation وCTE وpagination parameterized. |
| `db/tests/constraints.sql` | اختبارات فشل القيود الأساسية باستخدام `psql`. |
| `docs/diagrams/workhub-relational-model.mmd` | مصدر ER diagram. |
| `docs/diagrams/workhub-relational-model.png` | الرسم المرئي المطلوب للتسليم. |

## تشغيل migration محليًا

بعد تشغيل PostgreSQL وإنشاء قاعدة فارغة:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/migrations/001_init.up.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/seed/001_demo.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/constraints.sql
```

للتأكد من الجداول:

```bash
psql "$DATABASE_URL" -c '\dt'
psql "$DATABASE_URL" -c "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;"
```

## معايير القبول

يعد اليوم السادس مكتملًا عندما تبدأ migration من قاعدة فارغة دون أخطاء، وتنشئ seed بيانات قابلة للقراءة، وترفض قاعدة البيانات القيم غير المسموحة والتكرار والمراجع غير الموجودة، وتنجح الاستعلامات parameterized دون تركيب قيم المستخدم داخل SQL. يجب أن يكون ER diagram متوافقًا مع migration، وأن تكون قرارات soft-delete وownership وaudit موثقة.

## نتيجة التحقق

تم تشغيل PostgreSQL 16.15 محليًا، وإنشاء قاعدة `workhub_day6` من حالة فارغة، ثم تشغيل migration وseed واختبارات القيود بنجاح. نجحت اختبارات uniqueness للبريد، وقيم status وpriority، والمفتاح المركب للعضويات، والمفاتيح الخارجية لـ`task_tags`. اختبارات القيود تعمل داخل transaction وتنتهي بـ`ROLLBACK` حتى لا تترك بيانات اختبار إضافية.
