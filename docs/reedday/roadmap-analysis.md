# خارطة طريق WorkHub — تحليل الأيام 1–20

## 1. الصورة العامة

WorkHub هو مشروع Backend متعدد المستخدمين لإدارة المشاريع والمهام، مبني كـ **Modular Monolith**. الهدف النهائي ليس مجرد API يعمل، بل خدمة قابلة للمراجعة والتشغيل: بيانات PostgreSQL موثوقة، Redis، معالجة غير متزامنة، أمان، مراقبة، اختبارات، Docker، CI، واستعداد للإنتاج.

المعيار العام للنجاح هو الحصول على **70/100 أو أكثر** مع اجتياز بوابات السلامة والوظائف الإلزامية. التقييم موزع على أربع مهام أسبوعية:

| المهمة | الأسبوع | الوزن | الناتج الرئيسي |
|---|---:|---:|---|
| Weekly Task 1 | 1 | 15% | REST API وظيفي وهيكل Go قابل للصيانة. |
| Weekly Task 2 | 2 | 20% | PostgreSQL وtransactions وRedis وworker. |
| Weekly Task 3 | 3 | 25% | Authentication وauthorization وreliability وobservability. |
| Weekly Task 4 | 4 | 40% | حزمة إنتاج، تشغيل، استرجاع، أداء، ودفاع تقني. |

> التوقع الواقعي: موضوعات **H** يجب تنفيذها وإثباتها عمليًا، وموضوعات **G** تحتاج مختبرًا أو تصميمًا موجّهًا، أما **A** فتحتاج فهمًا للغرض والمفاضلات ومواطن الفشل، وليس ادعاء خبرة إنتاجية كاملة.

## 2. ما تم إنجازه في الأيام 1–5

| المجال | الحالة الحالية | الملاحظات |
|---|---|---|
| دورة الطلب وHTTP | منفذة | خدمة Go، `/health`، REST versioning، JSON، أخطاء موحدة، filtering وpagination metadata. |
| بنية Go | منفذة | `handler → service → repository` مع domain مستقل وdependency injection. |
| الاختبارات | منفذة جزئيًا جيدًا | Unit وHTTP وmiddleware وrace وvet وbenchmark؛ التغطية الحالية نحو 42.6%. |
| OpenAPI | منفذة | عقد للمشاريع والمهام مع success/error examples. |
| التخزين | مؤقت | In-Memory فقط؛ سيتم استبداله بـPostgreSQL في اليوم السادس والسابع. |
| المستخدمون والمصادقة | غير منفذة | ضرورية لأن WorkHub متعدد المستخدمين، وستبدأ في اليوم 11. |
| Redis والرسائل | غير منفذة | تبدأ في اليوم الثامن والتاسع. |
| Docker وCI | غير منفذة | تبدأ في اليوم 16 و17. |
| Observability | أساسية | request IDs وJSON logs موجودة، لكن metrics وtraces وdashboards لم تكتمل. |

هذا يعني أن الأيام 1–5 حققت أساس Weekly Task 1، لكنها لا تعني أن WorkHub صار نظامًا إنتاجيًا أو متعدد المستخدمين بالكامل بعد.

## 3. خطة الأيام 6–10: البيانات والتكامل غير المتزامن

### اليوم السادس — Relational modeling and SQL

نصمم ونرحّل مخطط PostgreSQL للمستخدمين والمشاريع والعضويات والمهام والوسوم والعلاقة `task_tags`. نركز على العلاقات، المفاتيح الأساسية والخارجية، القيود، timestamps، soft delete، وبيانات التدقيق. نكتب استعلامات parameterized ونمنع SQL injection.

**المخرج الإلزامي:** ER diagram، migration files، seed data، استعلامات SQL ممثلة، واختبارات القيود.

### اليوم السابع — Repositories, transactions, and consistency

نستبدل In-Memory Repository بمستودع PostgreSQL، ونضيف connection pooling وcontext deadlines وتحويل أخطاء قاعدة البيانات إلى أخطاء مجال. نطبق use case ذريًا ينشئ المهمة وسجل التدقيق في transaction واحدة، مع اختبار rollback.

**معيار القبول:** إذا فشلت خطوة إنشاء audit يجب ألا تبقى المهمة محفوظة وحدها.

### اليوم الثامن — Query performance and caching

نقيس استعلام قائمة المهام، نقرأ `EXPLAIN ANALYZE`، نضيف indexes مبررة، ثم نضيف Redis cache-aside لملخصات المشاريع. Redis ليس مصدر الحقيقة؛ عند فشل Redis يجب أن يستمر النظام بالرجوع إلى PostgreSQL. يجب اختبار cache hit وmiss وinvalidation.

### اليوم التاسع — Messaging, workers, and external storage

ننشر حدث `task.created` إلى broker، ونضيف notification worker يتعامل مع at-least-once delivery وretries وdead-letter handling. يجب أن يكون المستهلك idempotent حتى لا ترسل الرسالة المكررة إشعارًا مكررًا. نناقش transactional outbox وwebhooks وobject storage وsigned URLs.

### اليوم العاشر — Data integration review

نجمع مخرجات الأسبوع الثاني ونراجع migrations والاستعلامات واتساق cache وفشل worker. نعرض rollback، cache invalidation، duplicate message، وتعافي النظام بعد توقف dependency.

**أدلة الأسبوع:** tag، query evidence، demo، score sheet، وقائمة remediation.

## 4. خطة الأيام 11–15: الأمان والاعتمادية والمراقبة

### اليوم الحادي عشر — Authentication and identity

نضيف registration وlogin وrefresh وlogout/revocation وauthenticated identity middleware. كلمات المرور تُخزن باستخدام adaptive hashing، ولا تُكتب الأسرار أو الرموز في السجلات. نوثق دورة حياة access وrefresh tokens أو sessions.

### اليوم الثاني عشر — Authorization and API security

نطبق RBAC وownership وproject membership وسياسة deny-by-default. نختبر منع المستخدم من قراءة أو تعديل مشروع مستخدم آخر. نضيف request-size limits وrate limiting وsecurity headers وCORS مضبوطًا وthreat model من صفحة واحدة.

### اليوم الثالث عشر — Reliability and concurrency

نضيف deadlines وcancellation وretry eligibility وexponential backoff with jitter، مع bounded workers وsafe retry وreadiness checks وgraceful shutdown. نختبر cancellation وفشل dependency وSIGTERM ونشغل race detector.

### اليوم الرابع عشر — Observability and scaling fundamentals

نضيف structured logs وmetrics وtraces لمسارات HTTP وقاعدة البيانات وRedis والworker. نستخدم request/correlation IDs ونشرح RED/USE signals وlatency percentiles وSLOs. نرسم deployment diagram يوضح load balancer وreplicas وconsistency tradeoffs.

### اليوم الخامس عشر — Security and resilience review

ننفذ مراجعة abuse وdependency failure وconcurrency. نعرض forbidden access وexpired credentials وrate limiting وtimeout، ونشخص مشكلة تشغيلية من telemetry.

**أدلة الأسبوع:** tag، threat model، security tests، telemetry exports/screenshots، score، وfixes.

## 5. خطة الأيام 16–20: التشغيل والإصدار والاستعداد للإنتاج

### اليوم السادس عشر — Containers and runtime operations

نكتب Dockerfile multi-stage صغيرًا، نشغل العملية كمستخدم non-root، ونضيف Docker Compose لـAPI وworker وPostgreSQL وRedis وbroker. نضيف health checks وvolumes وnetworks وإعدادات Twelve-Factor.

### اليوم السابع عشر — CI/CD and deployment architecture

نبني CI stages للتنسيق والفحص الساكن والاختبارات وrace checks والبناء وفحص vulnerabilities وإنشاء artifact. نوثق migration sequencing وrollback وrolling أو blue-green أو canary deployment.

### اليوم الثامن عشر — Performance, data protection, and operations

نشغل baseline load test، نستخدم profiling للـCPU والذاكرة والallocations والgoroutines، نعالج bottleneck مقاسًا، ثم ننفذ backup/restore drill. نوثق RPO وRTO وrunbook والتنبيهات.

### اليوم التاسع عشر — Production-readiness review

نراجع API compatibility، data safety، least privilege، secrets، dependency failure، telemetry، reproducible build، والوثائق. نجهز ADRs وOpenAPI وتعليمات التشغيل واستكشاف الأخطاء، ويجب ألا توجد عيوب حرجة مفتوحة.

### اليوم العشرون — Final qualification

نقدم demo للمنتج والتشغيل، نشرح tradeoffs، نشخص عيبًا محقونًا، نجيب عن أسئلة system design، ثم نسجل الدرجة وقرار النجاح وخطة التطوير التالية.

## 6. معايير القبول التي سنستخدمها يوميًا

لن نعتبر اليوم مكتملًا بمجرد كتابة الكود. لكل يوم سنحافظ على جدول قبول بهذا الشكل:

| العنصر | سؤال القبول |
|---|---|
| الوظيفة | هل السيناريو الأساسي يعمل من البداية للنهاية؟ |
| الفشل | ماذا يحدث عند إدخال خاطئ أو dependency متوقفة أو timeout؟ |
| البيانات | هل توجد قيود أو transaction أو idempotency تمنع الفساد والتكرار؟ |
| الأمان | هل يمكن لمستخدم الوصول إلى مورد لا يملكه؟ هل توجد أسرار في السجلات؟ |
| الاختبار | هل يوجد اختبار قابل للتكرار لحالة النجاح وحالة الفشل؟ |
| التشغيل | هل توجد logs وhealth/readiness وتعليمات تشغيل واضحة؟ |
| الدليل | هل يوجد ملف أو command output أو diagram يثبت النتيجة؟ |
| المراجعة | هل سجلنا قرارًا تصميميًا وفجوة أو تحسينًا لاحقًا؟ |

## 7. البوابات الإلزامية

هذه ليست عناصر اختيارية في التقييم النهائي:

1. عدم وجود كلمات مرور نصية أو أسرار committed أو token logging غير آمن.
2. منع المستخدم من قراءة أو تعديل موارد مستخدم آخر دون صلاحية صريحة.
3. تشغيل migrations من قاعدة نظيفة، وإثبات نجاح backup/restore.
4. نجاح الاختبارات الحرجة وformatting وstatic analysis وbuild وrace checks.
5. بدء الخدمة من التعليمات الموثقة، والإبلاغ الصحيح عن readiness، والإغلاق دون إفساد العمل.
6. عدم وجود عيب حرج يسبب unauthorized access أو silent data loss أو persistent corruption أو crash loop غير مسيطر عليه.

## 8. ما الذي يجب أن أركز عليه أنا أثناء التدريب؟

ركز على فهم **سبب** كل قرار، وليس فقط نسخ الكود. عند كل feature اسأل: ما مصدر الحقيقة؟ ماذا يحدث إذا تكرر الطلب؟ ماذا يحدث إذا فشلت قاعدة البيانات أو Redis أو broker؟ كيف أعرف من السجل أي طلب فشل؟ كيف أثبت أن مستخدمًا لا يستطيع تجاوز صلاحياته؟

احرص أيضًا على أن تكون commits صغيرة وقابلة للمراجعة، وأن تكتب learning log يوميًا يتضمن القرار، الفشل الذي واجهته، الأوامر التي شغلتها، وسؤال اليوم التالي. هذه الأدلة ستفيد في التقييم أكثر من كود كبير يصعب شرحه.

## 9. نقطة البداية التالية

اليوم السادس يجب أن يبدأ بتصميم ER diagram ومخطط قاعدة البيانات قبل كتابة repository جديد. سنثبت أولًا كيانات `users` و`projects` و`memberships` و`tasks` و`tags` و`task_tags`، ونحدد cardinality والقيود وحقول التدقيق. بعد مراجعة المخطط نكتب migration قابلة للتشغيل من قاعدة فارغة، ثم نضيف seed واختبارات constraints.

بهذا الترتيب نمنع أن تصبح قاعدة البيانات مجرد نسخة من structs الحالية، ونبني الأساس الذي ستعتمد عليه transactions وcaching وauthorization في الأيام اللاحقة.
