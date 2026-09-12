# Clinic 1 — Modern HTTP في WorkHub

## الهدف

تشرح هذه الوثيقة كيف يختار WorkHub إصدار HTTP المناسب، وكيف تتعامل البنية مع TLS termination وkeep-alive والضغط ورؤوس الوكيل وإعادة استخدام الاتصال.

## مقارنة الإصدارات

| الإصدار | النقل | التعددية | الأثر على WorkHub |
|---|---|---|---|
| HTTP/1.1 | TCP، رسائل نصية، غالبًا عدة اتصالات متوازية | محدودة؛ لا توجد طبقة multiplexing أصلية | مناسب للتوافق والبساطة، لكنه قد يحتاج اتصالات متعددة عند كثرة الطلبات. |
| HTTP/2 | TCP مع framing ثنائي وmultiplexing للطلبات | عدة streams داخل اتصال واحد | يقلل عدد الاتصالات ويحسن تحميل API، لكن فقد حزمة TCP قد يوقف تدفقات نشطة حتى تتم المعالجة. |
| HTTP/3 | QUIC فوق UDP مع TLS 1.3 مدمج | streams مستقلة مع flow control لكل stream | يقلل أثر فقد حزمة على الطلبات الأخرى، ويحتاج دعم QUIC/UDP في العميل والشبكة. |

يوضح RFC 9114 أن HTTP/3 ينقل semantics الخاصة بـHTTP فوق QUIC، وأن QUIC يوفر multiplexing وflow control لكل stream وإنشاء اتصال بزمن تأخير منخفض [1]. كما يذكر أن HTTP/2 أضاف framing وmultiplexing فوق TCP، لكن فقد الحزمة قد يوقف المعاملات النشطة بسبب اعتماد TCP على ترتيب البايتات [1].

## TLS termination

في إنتاج WorkHub يُفضّل وضع Load Balancer أو Reverse Proxy أمام الخدمة:

```text
Client -- HTTPS/TLS --> Reverse Proxy -- HTTP/1.1 أو HTTP/2 --> WorkHub
```

الـProxy ينهي TLS، ويتحقق من الشهادة، ثم يمرر الرؤوس الآمنة مثل `X-Forwarded-For` و`X-Forwarded-Proto` مع تقييد مصادرها. يجب ألا يثق WorkHub بهذه الرؤوس من عميل مباشر غير موثوق. ويمكن لاحقًا استخدام TLS داخلي أيضًا إذا كانت الشبكة بين الـProxy والخدمة غير موثوقة.

## keep-alive وإعادة استخدام الاتصال

يحافظ العميل أو الوكيل على اتصال TCP/QUIC لإرسال عدة طلبات بدل إنشاء اتصال جديد لكل طلب. هذا يقلل تكلفة handshake ويُحسن latency. في WorkHub، يجب ضبط `ReadHeaderTimeout`، وحدود body، ومهلات الطلب حتى لا يتحول keep-alive إلى وسيلة لاستنزاف الموارد.

## compression

ضغط JSON مناسب للاستجابات الكبيرة مثل قوائم المشاريع والمهام، لكنه يضيف CPU وقد لا يفيد الاستجابات الصغيرة. يُفعّل عادة في الـReverse Proxy عبر `gzip` أو `br`، مع احترام `Accept-Encoding` وعدم ضغط البيانات الحساسة بطريقة تكشف ارتباط الحجم بالمحتوى.

## proxy headers وconnection reuse

الرؤوس المقترحة للتتبع هي:

| الرأس | الوظيفة |
|---|---|
| `X-Request-ID` | ربط سجل الطلب بين العميل والـProxy وWorkHub. |
| `X-Forwarded-For` | عنوان العميل الأصلي، ويُقبل فقط من Proxy موثوق. |
| `X-Forwarded-Proto` | توضيح أن الطلب الأصلي كان HTTPS. |
| `Content-Encoding` | بيان الضغط المستخدم للاستجابة. |
| `Cache-Control` | التحكم في التخزين المؤقت؛ API المتغير في WorkHub يستخدم `no-store` افتراضيًا. |

## قرار WorkHub

في المرحلة الحالية يبقى التطبيق Go HTTP/1.1-compatible خلف Proxy، لأن ذلك أبسط للتشغيل والاختبار. يمكن تفعيل HTTP/2 تلقائيًا عند استخدام TLS مناسب، ثم إضافة HTTP/3 على طبقة edge عندما تتوفر بنية QUIC. لا ينبغي أن يغير ذلك عقد `/api/v1` أو منطق الخدمات.

## المراجع

[1]: https://www.rfc-editor.org/rfc/rfc9114 "RFC 9114 — HTTP/3"
