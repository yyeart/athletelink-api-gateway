# Gateway Core gRPC contract v1

Статус: шаг 1 — схема публичного API и отображение данных спроектированы.
Metadata, таблица gRPC ошибок и typed details подтверждены в
[решениях шага 2](gateway-grpc-metadata-errors.md). Публичный code генерируется из
gRPC status; raw Core code/message/details не раскрываются. Production параметры
Auth/Redis остаются предварительными до подтверждения владельцем Auth.
Это контракт для реализации, а не заявление о работающем сервере.

## Источники и подтверждённые решения

- [Core OpenAPI](core-service-openapi.yaml): операции, upstream пути, параметры,
  тела, ответные DTO и объявленные HTTP-статусы.
- [Задача](../tasks/core-routing.md): границы шага 1 и последующая реализация.
- Ответы пользователя в текущей задаче: один сервис; UUID как string; даты как
  Timestamp с UTC; enum для ролей/статусов; неизвестный enum Core — несовместимый
  ответ; presence для отсутствующих полей; null только у iconUrl и как отсутствие;
  optional description опускается при отсутствии, явно передаётся при пустой строке;
  Core гарантирует ключ requestId в ответе создания.
- [go.mod](../../go.mod): точный Go module path, включая написание `athelete-link`.
  Это написание сохранено в go_package, а не исправлено предположением.

[core.proto](../../api/proto/athletelink/gateway/v1/core.proto) — источник protobuf
схемы. Package: `athletelink.gateway.v1`; сервис: `CoreService`; полный RPC method:
`/athletelink.gateway.v1.CoreService/<Method>`. Все методы unary и требуют JWT,
в том числе методы чтения. JWT-проверка пока не реализуется.

## Соответствие операций

В колонке «Тело» указана схема Core. Protobuf request имеет поля этой схемы на
верхнем уровне; идентификаторы пути не включаются в JSON body. Все upstream пути
ниже используются без `/api/v1`. HTTP-публичные URL не являются частью этого API.
Колонка ошибок фиксирует только объявленные статусы Core; gRPC code/details
проектируются в шаге 2 без угадывания семантики HTTP 409.

| RPC | HTTP Core | Параметры | Тело Core | Успех HTTP | Ошибки HTTP |
|---|---|---|---|---|---|
| `GetRequestDetails` | `GET /requests/{id}` | path `id` ← `request_id` | — | 200 | 400, 404 |
| `UpdateRequest` | `PUT /requests/{id}` | path `id` ← `request_id` | `UpdateActivityRequestDto` | 200 | 400, 403, 404, 409 |
| `SearchNearbyRequests` | `GET /requests` | query `lat` ← `lat`<br>query `lon` ← `lon`<br>query `radius` ← `radius`<br>query `sportId` ← `sport_id`<br>query `startDate` ← `start_date`<br>query `endDate` ← `end_date` | — | 200 | 400 |
| `CreateRequest` | `POST /requests` | — | `CreateActivityRequestDto` | 201 | 400, 404 |
| `RecordRoundResult` | `POST /requests/{requestId}/rounds/{roundNumber}/result` | path `requestId` ← `request_id`<br>path `roundNumber` ← `round_number` | `RecordRoundResultRequestDto` | 201 | 400, 403, 404, 409 |
| `CompleteRequest` | `POST /requests/{requestId}/complete` | path `requestId` ← `request_id` | — | 204 | 400, 403, 404, 409 |
| `StartRequest` | `POST /requests/{id}/start` | path `id` ← `request_id` | — | 204 | 400, 403, 404, 409 |
| `OpenRegistration` | `POST /requests/{id}/registration/open` | path `id` ← `request_id` | — | 204 | 400, 403, 404, 409 |
| `CloseRegistration` | `POST /requests/{id}/registration/close` | path `id` ← `request_id` | — | 204 | 400, 403, 404, 409 |
| `LeaveRequest` | `POST /requests/{id}/leave` | path `id` ← `request_id` | — | 200 | 400, 404, 409 |
| `KickParticipant` | `POST /requests/{id}/kick/{targetUserId}` | path `id` ← `request_id`<br>path `targetUserId` ← `target_user_id` | — | 200 | 400, 403, 404, 409 |
| `JoinRequest` | `POST /requests/{id}/join` | path `id` ← `request_id` | — | 200 | 400, 404, 409 |
| `CancelRequest` | `POST /requests/{id}/cancel` | path `id` ← `request_id` | — | 200 | 400, 403, 404, 409 |
| `GetAllSports` | `GET /sports` | — | — | 200 | — |
| `GetRoundResults` | `GET /requests/{requestId}/rounds` | path `requestId` ← `request_id` | — | 200 | 400, 404 |

## Отображение успешных ответов

| RPC | JSON Core → protobuf |
|---|---|
| GetRequestDetails, UpdateRequest | ActivityRequestDetailsDto → поле `request` типа ActivityRequestDetails |
| SearchNearbyRequests | массив ActivityRequestFeedDto → repeated `requests` |
| CreateRequest | гарантированный ключ `requestId` → `request_id`; отсутствие/невалидный UUID — несовместимый ответ |
| RecordRoundResult | RoundResultResponseDto → поле `result` типа RoundResult |
| GetAllSports | массив SportDto → repeated `sports` |
| GetRoundResults | массив RoundResultResponseDto → repeated `results`, порядок сохраняется |
| CompleteRequest, StartRequest, OpenRegistration, CloseRegistration | HTTP 204 без тела → отдельное пустое Response сообщение |
| JoinRequest, LeaveRequest, KickParticipant, CancelRequest | HTTP 200 без объявленного content → отдельное пустое Response сообщение |

Последние две группы не декодируют бизнес-DTO: в snapshot для этих успешных ответов
нет схемы тела. Пустые Response сообщения отдельны для каждого RPC, чтобы будущие
поля можно было добавлять независимо. Списочные root-ответы Core представлены
repeated внутри собственного Response; пустой массив даёт пустой repeated.

## Правила преобразования

Таблица и правила ниже — проект механики адаптера на основе подтверждённых типов,
а не утверждение о runtime поведении Core.

| Core JSON | Protobuf |
|---|---|
| camelCase свойства | snake_case поля: например `numberOfRounds` → `number_of_rounds` |
| UUID string | string, без bytes/числовой интерпретации |
| int32 / int64 | int32 / int64 с проверкой представимости; без преобразования через float64 |
| double | double, без округления координат или радиуса |
| boolean / string | bool / string |
| date-time | google.protobuf.Timestamp; сохраняется instant и доступная наносекундная точность, вывод в Core — RFC 3339 UTC с `Z` |
| enum string | точное соответствие таблицам enum ниже |
| optional scalar | optional поле, присутствие сохраняется даже для `0`, `false` и `""` |
| optional object/timestamp | присутствие message |
| массив в ответном DTO | message-обёртка с repeated `values`, чтобы сохранить присутствие массива |

Не использовать универсальный ProtoJSON marshal как HTTP body Core: поля, enum,
обёртки и int64 отображляются по этому документу явно. Для запросов query кодируется
отдельно от path, JSON body — отдельно от обоих. `request_id` соответствует `id`
или `requestId` пути; `target_user_id` — `targetUserId`; `round_number` — `roundNumber`.
Идентификатор клиента в payload отсутствует: actor UUID берётся из доверенного
контекста и передаётся как X-User-Id только для требующих его операций snapshot.

### Присутствие и обязательность

Proto3 optional не заменяет required Core. Все входные scalar используют explicit
presence, включая обязательные поля: отсутствие не превращается в zero value.
Обязательность берётся из snapshot:

- SearchNearbyRequests: query `lat`, `lon`; `radius`, `sport_id`, `start_date`,
  `end_date` необязательны. Отсутствующий radius не посылать и не подставлять 5000:
  это значение default Core из snapshot, не Gateway.
- CreateRequest: `title`, `sport_id`, `max_players`, `event_date`, `number_of_rounds`,
  `address_text`, `latitude`, `longitude`; `description` необязателен.
- UpdateRequest: path `request_id`; body `title`, `event_date`, `number_of_rounds`,
  `max_players`; `description` необязателен. Это PUT, не частичный PATCH.
- RecordRoundResult: path `request_id`, `round_number`; body `winners`, `losers`.
- Остальные path-параметры обязательны согласно таблице операций.

При отсутствии query/body поля адаптер его не отправляет; обязательность и
бизнес-ограничения проверяет Core. Если отсутствует path-поле, построить маршрут
невозможно: это структурная ошибка запроса, без вызова Core. Невалидный Timestamp
также нельзя сериализовать как date-time. gRPC-коды этих ошибок фиксируются в шаге 2.
Обёртка `UuidList` в запросе сохраняет отсутствие массива; присутствующая обёртка
без values сериализуется как `[]`. Проверки minItems, пересечений списков, диапазонов,
времени события и прав организатора остаются Core.

В ответных DTO нет required: отсутствующие scalar/object/list остаются отсутствующими.
`ParticipantList`/`UuidList` присутствуют для явно пустого массива и отсутствуют для
отсутствующего JSON свойства. Null разрешён только для Sport.iconUrl, где он означает
отсутствующее icon_url. Явная пустая строка iconUrl сохраняется как present `""`.
Остальные null, включая null элементы массива, несовместимы с этим контрактом.

Для description: отсутствие свойства не отправляется в Core, пустая строка
отправляется. Gateway не утверждает, что отсутствие при PUT сохраняет или очищает
текущее описание: это бизнес-семантика Core.

### Enum

| Protobuf enum | Значения Core в порядке номеров 1…N |
|---|---|
| ActivityRequestStatus | PLANNED, ACTIVE, CONFIRMATION, COMPLETED, CANCELLED, FROZEN |
| ParticipantRole | ORGANIZER, PLAYER |
| ParticipantStatus | PENDING, ACCEPTED, KICKED, LEFT |

Каждый enum имеет уникально префиксованные значения; номер 0 — UNSPECIFIED,
техническое значение protobuf без соответствующего значения Core. Если Core не
вернул поле, optional enum остаётся отсутствующим. Если вернул незнакомую строку,
адаптер не подменяет её на UNSPECIFIED и не выдаёт частичный успешный ответ.
Номера 1…N назначены в порядке значений snapshot; это новые protobuf номера,
не числовые значения enum Core.

### Несовместимый upstream-ответ

Невалидный JSON, несовпадение типа, неизвестный enum, недопустимый null, невалидный
UUID/date-time либо число вне protobuf диапазона не дают успешный DTO. Частичная
выдача и молчаливая подстановка значений запрещены. Окончательный gRPC status,
публичное сообщение и details для ошибки адаптера — предмет шага 2.
Необъявленный HTTP status тоже нельзя автоматически принять как успешный.
Политика unknown JSON properties и публичных error details зафиксирована в шаге 2:
unknown properties игнорировать, raw Core errors не раскрывать.

## Совместимость

v1 — часть package. Поля имеют стабильные номера; запрещено повторно использовать
номер или имя удалённого поля/enum: при удалении оба помещаются в reserved.
Сейчас удалённых полей нет, поэтому искусственных reserved в схеме нет.
Не менять тип и смысл существующего поля. Неизвестный Core enum требует обновления
Gateway enum/адаптера до поддержки новой Core версии; не маскировать рассогласование.

## Инструменты и команды генерации

Закреплённый набор для этого проекта, без заявления «самые новые версии»:

| Инструмент | Версия | Источник релиза |
|---|---|---|
| protoc | 33.5 | [protobuf v33.5](https://github.com/protocolbuffers/protobuf/releases/tag/v33.5) |
| protoc-gen-go | v1.36.11 | [protobuf-go v1.36.11](https://github.com/protocolbuffers/protobuf-go/releases/tag/v1.36.11) |
| protoc-gen-go-grpc | v1.6.2 | [grpc-go generator v1.6.2](https://github.com/grpc/grpc-go/releases/tag/cmd%2Fprotoc-gen-go-grpc%2Fv1.6.2) |

Установить protoc из официального release asset для своей платформы вместе с его
include/google/protobuf. `PROTOC_ROOT` ниже — каталог распакованного protoc 33.5.
Команды из корня api-gateway; плагины устанавливаются в локальный каталог:

```sh
mkdir -p .tools/bin
env GOBIN="$PWD/.tools/bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
env GOBIN="$PWD/.tools/bin" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
"$PROTOC_ROOT/bin/protoc" --version
.tools/bin/protoc-gen-go --version
.tools/bin/protoc-gen-go-grpc --version
mkdir -p api/gen
"$PROTOC_ROOT/bin/protoc" \
  -I api/proto -I "$PROTOC_ROOT/include" \
  --plugin=protoc-gen-go=.tools/bin/protoc-gen-go \
  --plugin=protoc-gen-go-grpc=.tools/bin/protoc-gen-go-grpc \
  --go_out=api/gen --go_opt=paths=source_relative \
  --go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
  api/proto/athletelink/gateway/v1/core.proto
```

Ожидаемые файлы: `api/gen/athletelink/gateway/v1/core.pb.go` и `core_grpc.pb.go`.
Генерация Go и добавление runtime-зависимостей выполняются на этапе реализации;
в этом шаге go.mod и сервер не меняются. Перед генерацией проверить вывод версий
на соответствие таблице. Не использовать @latest.

Основания механики protobuf: [field presence](https://protobuf.dev/programming-guides/field_presence/),
[Timestamp](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp).
Repeated сам по себе не отслеживает presence — поэтому для DTO массивов введены
message-обёртки; Timestamp представляет момент времени, исходный offset не сохраняется.

## Проверка этого шага

Схема скомпилирована protoc 33.5 в descriptor set с include_imports. Проверяются
полный состав RPC, соответствие полей исходным DTO и отдельные request/response.
Go generation, runtime JWT, обработчики, metadata, gRPC ошибки и реальная интеграция
не выполнены этим шагом. Конформность работающего Core: **I cannot verify this**.
