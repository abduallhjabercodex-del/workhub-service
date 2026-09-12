# Week 1 Clinics — WorkHub

تحتوي هذه المجلدات على مخرجات العيادات الثلاث الخاصة بالبروتوكولات وواجهات API وحدود الخدمات.

| العيادة | المخرج |
|---|---|
| Modern HTTP | `01-modern-http.md` — مقارنة HTTP/1.1 وHTTP/2 وHTTP/3، TLS termination، keep-alive، compression، proxy headers، وإعادة استخدام الاتصال. |
| Real-time delivery | `02-realtime-delivery.md` و`../diagrams/realtime-lifecycle.png` — مقارنة polling وlong polling وSSE وWebSockets وstreaming، مع دورة الاتصال والـheartbeats والـfan-out والـbackpressure. |
| RPC and schemas | `03-rpc-and-schemas.md` و`../../api/proto/workhub.proto` — عقد gRPC/Protobuf، deadlines، metadata، streaming، schema evolution، compatibility، وgateway translation. |

هذه المخرجات تصميمية وتعليمية مرتبطة بالـBackend الحالي. REST هو الواجهة العامة الحالية، بينما SSE وgRPC موثقان كمسارات توسعة منضبطة لاختبارها قبل إدخال تعقيد تشغيلي إضافي.
