# اليوم الثامن — Query Performance and Caching

## النتيجة

أصبح WorkHub يملك قياسًا فعليًا لخطة استعلام ملخص المشروع، وفهرسًا مبررًا، وRedis cache-aside لملخصات المشاريع. PostgreSQL يظل **مصدر الحقيقة**؛ Redis مجرد نسخة مؤقتة يمكن حذفها أو تعطيلها دون فقدان البيانات.

## الفهرس والقرار

أضيفت migration `002_query_performance.up.sql` الفهرس التالي:

```sql
CREATE INDEX tasks_project_summary_idx
    ON tasks (project_id)
    INCLUDE (status)
    WHERE deleted_at IS NULL;
```

اختيار `project_id` أولًا يطابق شرط التصفية في ملخص المشروع. و`INCLUDE (status)` يساعد على قراءة الحالة من بنية الفهرس عند اختيار خطة index-only مناسبة، بينما predicate الجزئي يطابق كل الاستعلامات التي تستبعد soft-deleted tasks. لا نعتبر الفهرس مفيدًا لكل توزيع بيانات؛ يجب إعادة القياس بعد تغيير حجم البيانات وتوزيعها.

## EXPLAIN evidence

استخدمنا قاعدة PostgreSQL 16.15 مع 50,000 مهمة في مشروع كبير و10 مهام في مشروع صغير. على المشروع الصغير تغيّرت الخطة من استخدام `tasks_project_status_created_idx` إلى `tasks_project_summary_idx`، وانخفضت التكلفة المقدرة من `37.99` إلى `16.99`، وانخفض زمن التنفيذ في القياس من `0.146 ms` إلى `0.102 ms`.

| الحالة | الخطة المستخدمة | التكلفة المقدرة | التنفيذ المقاس |
|---|---|---:|---:|
| قبل الفهرس الجديد، المشروع الانتقائي | `tasks_project_status_created_idx` | `37.99` | `0.146 ms` |
| بعد الفهرس الجديد، المشروع الانتقائي | `tasks_project_summary_idx` | `16.99` | `0.102 ms` |
| مشروع كبير جدًا، قبل وبعد | `Seq Scan` ظل أفضل من index scan | `2376.25` | نحو `44.5 ms` قبل و`42.7 ms` بعد |

النتيجة المهمة ليست أن كل استعلام سيستخدم الفهرس؛ optimizer اختار sequential scan عندما كان سيقرأ تقريبًا كل صفوف المشروع الكبير. هذا سلوك صحيح، ويؤكد ضرورة الاعتماد على `EXPLAIN ANALYZE` بدل افتراض أن إضافة الفهرس دائمًا تحسن الأداء.

الملفات الخام هي `docs/day8-explain-before.txt` و`docs/day8-explain-after.txt` ونسختا المشروع الانتقائي `docs/day8-explain-selective-before.txt` و`docs/day8-explain-selective-after.txt`.

## Redis cache-aside

أضيف endpoint:

```text
GET /api/v1/projects/{projectID}/summary
```

المسار يقرأ Redis أولًا. عند cache miss يقرأ PostgreSQL، ثم يكتب النتيجة إلى Redis لمدة TTL. عند إنشاء أو تحديث مهمة ناجح، يُحذف مفتاح ملخص المشروع. عند تعطل Redis تفشل عمليات القراءة والكتابة والإبطال بشكل آمن، ويستمر التطبيق باستخدام PostgreSQL.

| السلوك | الدليل |
|---|---|
| Miss أول مرة | سجل `cache miss` ثم قراءة من PostgreSQL. |
| Hit | سجل `cache hit` وإرجاع الملخص دون قراءة المصدر في ذلك الطلب. |
| Invalidation | تحديث المهمة سجل `cache invalidated`. |
| Miss بعد التحديث | الطلب التالي أعاد القراءة من PostgreSQL وسجل `cache miss`. |
| TTL | اختبار miniredis أثبت انتهاء القيمة بعد TTL. |
| Redis failure | الاختبار أغلق Redis وأثبت عدم تحويل فشل cache إلى نجاح زائف أو panic. |

## Pool sizing

يُنشأ `pgxpool.Pool` من `DATABASE_URL`. لم نضع أرقامًا عشوائية لـ`MaxConns` في هذه المرحلة؛ الحجم الصحيح يعتمد على عدد نسخ الخدمة، قدرة PostgreSQL، وزمن الاستعلامات. القياس التالي في بيئة حمل حقيقية سيحدد `MaxConns` و`MinConns` بدل تخمينهما.

## معايير القبول

يعد اليوم الثامن مكتملًا عندما توجد خطة EXPLAIN قبل وبعد، ويكون كل فهرس مرتبطًا باستعلام وشرط استخدام واضح، ويعمل cache-aside مع TTL، وتظهر hit/miss وinvalidation، ويظل PostgreSQL مصدر الحقيقة، وتوجد اختبارات لسلوك Redis عند التوقف. تحققت هذه الشروط في هذه النسخة.
