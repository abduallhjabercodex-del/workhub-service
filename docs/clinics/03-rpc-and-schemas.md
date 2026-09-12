# Clinic 3 — RPC and schemas في WorkHub

## الهدف

تقدم هذه العيادة نموذجًا بديلًا لعقد REST الحالي: خدمة gRPC داخلية تستخدم Protocol Buffers كـIDL وكصيغة رسائل. تعريف الخدمة موجود في `api/proto/workhub.proto`.

## نموذج API

العقد يعرّف خدمة `WorkHubService` بالعمليات التالية:

| RPC | الغرض |
|---|---|
| `CreateProject` | إنشاء مشروع. |
| `GetProject` و`ListProjects` | قراءة مشروع أو قائمة مشاريع. |
| `CreateTask` | إنشاء مهمة داخل مشروع. |
| `GetTask` و`UpdateTask` | قراءة مهمة أو تحديثها. |
| `ListTasks` | استعراض مهام مشروع مع query وstatus وpagination. |

مثال مختصر من العقد:

```protobuf
service WorkHubService {
  rpc CreateTask(CreateTaskRequest) returns (Task);
  rpc UpdateTask(UpdateTaskRequest) returns (Task);
}
```

توضح وثائق gRPC أن العميل يستدعي method على خادم بعيد كما لو كان استدعاءً محليًا، وأن ملف `.proto` يعرّف الخدمات والرسائل ثم يولّد `protoc` كود العميل والخادم [4].

## deadlines وmetadata

كل RPC يجب أن يضع deadline من العميل، مثل 2 أو 5 ثوانٍ، وأن يمرر `context.Context` عبر جميع الطبقات. يمكن استخدام metadata لتتبع `request-id`، هوية المستخدم، tenant، وauthorization token. لا ينبغي وضع بيانات كبيرة أو أسرار غير لازمة داخل metadata.

## streaming

العمليات الحالية unary لأن إنشاء أو قراءة مشروع/مهمة غالبًا عملية واحدة. عند الحاجة يمكن إضافة:

```protobuf
rpc WatchProject(WatchProjectRequest) returns (stream ProjectEvent);
```

هذا مناسب لتحديثات المشروع، لكن يحتاج حدودًا للاتصال والـfan-out وbackpressure مثل تصميم SSE في العيادة الثانية.

## schema evolution والتوافق

نستخدم أرقام fields ثابتة، ولا نعيد استخدام رقم حقل حُذف. الإضافات المتوافقة عادة تكون بإضافة field جديد، بينما تغيير نوع field أو معناه قد يكسر العملاء. يجب استخدام `reserved` للأرقام والأسماء المحذوفة، واختبار backward وforward compatibility في CI. توصي وثائق Protocol Buffers بممارسات تحديث تحافظ على قدرة الكود القديم على قراءة الرسائل الجديدة مع تجاهل الحقول غير المعروفة [5].

في `UpdateTaskRequest` استخدمنا `optional` للتمييز بين حقل غير مرسل وحقل مرسل بقيمة، وهو مهم للتحديث الجزئي. أما `TaskStatus` فيبدأ بقيمة `UNSPECIFIED = 0` حتى لا تفسر القيمة الافتراضية تلقائيًا على أنها `done` أو `in_progress`.

## gateway translation

يمكن نشر REST Gateway أمام gRPC:

```text
Web / Mobile Client -- REST/JSON --> API Gateway -- gRPC/Protobuf --> WorkHub Service
```

مثلًا يمكن ترجمة:

```http
PATCH /api/v1/tasks/{taskId}
```

إلى:

```text
UpdateTask(UpdateTaskRequest{id, title?, status?})
```

يجب الحفاظ على semantics نفسها: رموز الأخطاء، authorization، request ID، وdeadline. لا ينبغي أن يصبح الـGateway مكانًا لقواعد العمل؛ تظل قواعد المجال داخل service layer.

## القرار المقترح لـWorkHub

يظل REST هو الواجهة العامة في هذه المرحلة، لأنه مناسب للمتصفح وسهل الفحص بـcurl. يمكن إضافة gRPC لاحقًا للاتصال الداخلي بين وحدات WorkHub أو خدمات مستقبلية، مع إبقاء `workhub.v1` ثابتًا وإضافة `workhub.v2` فقط عند وجود تغيير غير متوافق.

## المراجع

[4]: https://grpc.io/docs/what-is-grpc/introduction/ "gRPC — Introduction"
[5]: https://protobuf.dev/overview/ "Protocol Buffers — Overview"
