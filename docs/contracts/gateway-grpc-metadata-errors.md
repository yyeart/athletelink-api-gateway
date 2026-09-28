# Gateway v1: metadata и ошибки — решения шага 2

Статус: metadata, таблица статусов, собственные rich error types и игнорирование
неизвестных JSON полей, публичный фильтр Core ошибок и отказ при ошибке Redis
подтверждены пользователем. Шаг 2 завершён как контракт metadata/ошибок.
Предварительные Auth/Redis параметры не считаются production контрактом.
Схема RPC и DTO из [шага 1](gateway-grpc.md) остаётся без изменений.

## Источники и границы доказательств

- [Core OpenAPI](core-service-openapi.yaml): требование X-User-Id, схема ApiError,
  описания и статусы ошибок каждой операции. Перечень значений ApiError.code не
  задан; пример VALIDATION_ERROR не является полным перечнем.
- [INTEGRATION](../INTEGRATION.md): JWT для всех операций, identity из проверенного
  контекста, новый request ID вместо клиентского, запрет production wiring без JWT.
- [gRPC status codes](https://grpc.io/docs/guides/status-codes/): значения и семантика
  gRPC кодов. Таблица ниже — политика Gateway, не универсальная таблица HTTP → gRPC.
- [Metadata](https://grpc.io/docs/guides/metadata/): request/response headers/trailers.
- [Error handling](https://grpc.io/docs/guides/error/): модель status и rich details.
- [google.rpc.Status](https://github.com/googleapis/googleapis/blob/master/google/rpc/status.proto):
  code, message, repeated Any details. Предлагаемые CoreErrorDetail/GatewayErrorDetail
  ниже — собственные типы Gateway, не стандартные типы Google.

Работающий Core и безопасность содержимого его бизнес-ошибок:
**I cannot verify this**. Документ не подтверждает отсутствие секретов в ApiError.

## A. Подтверждённая metadata policy

| Направление / ключ | Правило |
|---|---|
| Входящий authorization | Ровно одно значение `Bearer <JWT>`. Нет значения, дубликаты, пустой токен или неверный формат — UNAUTHENTICATED без вызова Core. Алгоритм, claims, clock skew и rotation остаются в контракте Auth. |
| Входящий x-user-id | Игнорировать и не пересылать; никогда не источник actor UUID. |
| Входящий x-request-id | Игнорировать независимо от значения/числа значений; заменить новым Gateway UUID. |
| Прочая прикладная metadata | Не пересылать в Core; неизвестные ключи не становятся HTTP headers. |
| Core X-User-Id | Только UUID из доверенного контекста, для операций snapshot с required заголовком. |
| Core X-Request-Id | Один Gateway request ID для вызова; ответ Core не может его заменить. |
| Core Authorization / Cookie | Не передавать клиентский JWT/cookies в Core: snapshot требует identity, а не эти заголовки. |
| Ответная прикладная metadata | Allow-list состоит только из Gateway `x-request-id`; не копировать upstream headers. |

Встроенные gRPC transport headers/trailers, включая status и rich details, не входят
в allow-list прикладных заголовков и не копируются из Core.
Request ID создавать до проверки аутентификации и payload. Для вызовов, дошедших до
прикладной цепочки Gateway, возвращать `x-request-id` в response headers при успехе
и ошибке; тот же ID включать в rich error detail при ошибке. Для неизвестного RPC
unknown-service handler также использует корреляцию и возвращает UNIMPLEMENTED.

На ошибках транспорта до входа в приложение и при разрыве соединения наличие
metadata/detail у клиента не гарантируется. Отмена/deadline клиента может завершить
вызов до доставки Gateway ответа. Не обещать correlation metadata для этих случаев.

Отсутствие доверенного UUID — UNAUTHENTICATED, включая операции чтения.
Проверенный токен с невалидным UUID claim также отклоняется аутентификацией, без
вызова Core. JWT ни в response, ни в error details не возвращается.
В текущих компонентных тестах trusted UUID/request ID предоставляет fixture
interceptor; это не production обход. Генератор request ID реализуется последующим
этапом согласно плану, production цепочка без него не подключается.

## B. Подтверждённое отображение статусов

Сначала проверять состояние context, затем транспорт, затем статус и тело Core.
Успешный status должен совпадать с объявленным для операции; например, CreateRequest
ожидает 201, CompleteRequest — 204. Произвольный 2xx не считать успехом.

| Условие | gRPC code | Источник detail |
|---|---|---|
| Объявленный успех с совместимым телом | OK | Нет error detail |
| Объявленный HTTP 400 с совместимым ApiError | INVALID_ARGUMENT | CORE |
| Объявленный HTTP 403 с совместимым ApiError | PERMISSION_DENIED | CORE |
| Объявленный HTTP 404 с совместимым ApiError | NOT_FOUND | CORE |
| Объявленный HTTP 409 с совместимым ApiError | FAILED_PRECONDITION | CORE |
| Отсутствующая/невалидная аутентификация или trusted identity | UNAUTHENTICATED | GATEWAY: AUTHENTICATION_REQUIRED |
| Проверка denylist невозможна: Redis недоступен/ошибка lookup/timeout при живом RPC context | UNAVAILABLE | GATEWAY: AUTH_CHECK_UNAVAILABLE; без вызова Core |
| Нет обязательного path поля / Timestamp невозможно сериализовать / входной double NaN или Infinity невозможно передать в JSON | INVALID_ARGUMENT | GATEWAY: INVALID_REQUEST |
| context отменён клиентом | CANCELLED | GATEWAY: REQUEST_CANCELLED, если ответ доставлен |
| Истёк deadline вызова либо настроенный timeout ожидания Core | DEADLINE_EXCEEDED | GATEWAY: DEADLINE_EXCEEDED |
| Сетевая ошибка соединения/TLS/чтения при живом context, не timeout | UNAVAILABLE | GATEWAY: UPSTREAM_UNAVAILABLE |
| Core HTTP 500…599 | UNAVAILABLE | GATEWAY: UPSTREAM_UNAVAILABLE; сырое тело не раскрывается |
| Необъявленный иной HTTP status, включая 2xx, 3xx, 401, 405, 429 | INTERNAL | GATEWAY: UPSTREAM_CONTRACT_VIOLATION |
| Несовместимый success JSON или ApiError, включая неизвестный enum/невалидный UUID/null | INTERNAL | GATEWAY: UPSTREAM_CONTRACT_VIOLATION |
| Внутренняя ошибка Gateway, не относящаяся к данным Core | INTERNAL | GATEWAY: INTERNAL_ERROR |
| Неизвестный RPC | UNIMPLEMENTED | GATEWAY: METHOD_NOT_IMPLEMENTED |

HTTP 400/403/404/409 отображаются как бизнес-ошибки только если они объявлены именно
для вызванной операции. Например, GetAllSports не объявляет 404; такой ответ —
рассогласование контракта, а не NOT_FOUND.

Редиректы HTTP adapter не следует автоматически выполнять: 3xx должен попасть
в таблицу необъявленных статусов. Клиентская authorization metadata не пересылается
даже при редиректе. HTTP adapter выполняет один запрос на RPC; retries не добавляются.

### HTTP 409: подтверждённая политика

Следующие причины взяты из описаний 409 snapshot. Это не значения ApiError.code:

| RPC | Объявленные причины |
|---|---|
| UpdateRequest | Заявка не PLANNED или вместимость меньше числа участников |
| RecordRoundResult | Заявка не ACTIVE или результат раунда уже существует |
| CompleteRequest | Заявка не ACTIVE или записаны не все раунды |
| StartRequest | Заявка не PLANNED или недостаточно игроков |
| OpenRegistration, CloseRegistration | Заявка не PLANNED |
| LeaveRequest | Заявка не PLANNED или игрок не может выйти |
| KickParticipant | Заявка не PLANNED или игрок уже вышел/исключён |
| JoinRequest | Регистрация закрыта, заявка заполнена, игрок уже присоединился/исключён |
| CancelRequest | Заявка не PLANNED |

Подтверждено: все перечисленные 409 → FAILED_PRECONDITION; публичный CoreErrorDetail.code
генерируется из согласованного gRPC status,
без разбора человекочитаемого message. ALREADY_EXISTS/ABORTED/RESOURCE_EXHAUSTED
не выводить из текста. Если клиенту нужны отдельные gRPC коды для причин, сначала
получить полный перечень стабильных Core error codes и их семантику от владельца Core.

UNAVAILABLE/DEADLINE_EXCEEDED не означают, что изменение не произошло в Core.
Клиент не должен автоматически повторять изменяющие RPC только на основании кода;
дедупликация/idempotency этой версией не обещаются. Это соответствует ограничениям
[gRPC status codes](https://grpc.io/docs/guides/status-codes/).

## C. Подтверждённые типы и фильтрация ошибок

Ошибки идут через non-OK gRPC status, не через success Response или JSON envelope.
Подтверждён формат: один собственный detail, packed в google.rpc.Status.details:

- CORE: `athletelink.gateway.v1.CoreErrorDetail`:
  `http_status` (int32), `code` (optional string), `message` (optional string),
  `details` (message с map<string,string> values), `request_id` (string).
- GATEWAY: `athletelink.gateway.v1.GatewayErrorDetail`:
  `code` (string из таблицы B), `request_id` (string).

Тип detail отличает источники; не добавлять CoreErrorDetail на сетевую ошибку,
неподдержанный status или невалидный ApiError. При разборе исходного ApiError сохраняется
presence: отсутствие details отличается от пустой map; null запрещён согласно
подтверждённой политике шага 1. Schema не требует code/message, поэтому их отсутствие
само по себе не ошибка адаптера. Неверный тип, null, неверный тип значения map — ошибка.

Пользователь отклонил передачу code/message/details без фильтрации. Поэтому
поля CoreErrorDetail предназначены для разрешённых публичных значений, а не сырых
строк Core. Исходный ApiError.message не подставляется автоматически в Status.message.
Нельзя угадывать безопасное содержимое, вырезать секреты регулярными выражениями
или публиковать все поля только потому, что JSON прошёл проверку типов.

Подтверждено пользователем: default-deny, фиксированный Status.message
`Core request failed`, raw Core code/message/details не раскрываются.
CoreErrorDetail.code формируется из согласованного gRPC status, а не из ApiError.code:

| Объявленный HTTP error | gRPC status и публичный CoreErrorDetail.code |
|---|---|
| 400 | INVALID_ARGUMENT |
| 403 | PERMISSION_DENIED |
| 404 | NOT_FOUND |
| 409 | FAILED_PRECONDITION |

CoreErrorDetail содержит http_status, present code и Gateway request_id.
Поля message и details всегда отсутствуют в публичном detail этой версии, даже если
Core прислал их или пустую map. Они остаются в protobuf schema, но не разрешены к
публичному заполнению. Presence исходного ApiError учитывается при разборе, а фильтр
применяется перед построением detail. Ошибочный тип/null остаётся нарушением контракта
даже для полей, которые затем скрываются. Не сериализовать raw ApiError в metadata.

Предварительный список категорий пользователя не добавляет новых mappings:
ALREADY_EXISTS/RESOURCE_EXHAUSTED не выводятся из текста Core или неизвестного status.
UNAUTHENTICATED относится к Gateway auth failure, а не к необъявленному Core 401.

Status.message для GATEWAY — фиксированные нейтральные строки, не текст err.Error():

| Gateway code | Status.message |
|---|---|
| AUTHENTICATION_REQUIRED | Authentication required |
| AUTH_CHECK_UNAVAILABLE | Authentication service unavailable |
| INVALID_REQUEST | Invalid request |
| REQUEST_CANCELLED | Request cancelled |
| DEADLINE_EXCEEDED | Request deadline exceeded |
| UPSTREAM_UNAVAILABLE | Core service unavailable |
| UPSTREAM_CONTRACT_VIOLATION | Invalid Core response |
| INTERNAL_ERROR | Internal Gateway error |
| METHOD_NOT_IMPLEMENTED | RPC method not implemented |

Не включать токены, cookies, body, stack trace, upstream URL или сырой transport error
в status/details. Подтверждено: unknown Core JSON properties игнорируются, включая
новые поля ApiError; известные поля должны проходить строгую проверку типов и null.
Так добавление поля Core не делает существующий DTO автоматически несовместимым.

Core HTTP 5xx/необъявленные ответы не раскрываются как бизнес ApiError даже при
похожем JSON. Не переносить arbitrary response headers, включая Set-Cookie,
WWW-Authenticate, Location, Retry-After и клиентский/внутренний identity.

Подтверждённые error detail types добавлены в core.proto; публичная семантика
фильтруемых полей определена выше. Наличие поля не разрешает раскрывать raw ApiError.
Не заменять собственным типом стандартный google.rpc.Status и не присваивать
новые числовые gRPC коды. Лимиты размера body/metadata/details — отдельные требования
реализации; числовые пределы этим документом не придуманы.

## Проверка реализации после утверждения

Проверить повторные authorization values, поддельные identity/request ID, отсутствие
identity, все объявленные status каждой операции, каждую строку таблицы B, malformed
ApiError, отсутствие/пустые поля ApiError, неизвестные JSON поля, allow-list metadata,
контекст cancellation/deadline и запрет retry изменяющих запросов. Статусы проверять
через gRPC client, rich details — распаковкой Any в согласованный собственный тип.
Correlation detail и response metadata должны содержать один и тот же ID.

## Дополнительные подтверждённые требования Auth

Источник: уточнение пользователя в текущей задаче.

- userId извлекается из access-токена после проверки подписи.
- Gateway получит secret_key, которым Auth подписывает токены. Сам ключ в
  документацию, репозиторий, логи или вопросы пользователю не помещается.
- Gateway проверяет тип токена и допускает только access-токен.
- Gateway проверяет отсутствие токена в denylist Redis перед созданием доверенного
  контекста. Токен в denylist не допускается к Core.
- Без подписи, проверки типа и проверки denylist доверенную identity не создавать.

### Данные из предоставленного пользователем кода Auth

Источник — Java fragment и пояснения пользователя в текущей задаче; исполняемая
интеграция с Auth не проводилась.

| Поле / свойство | Подтверждённые данные |
|---|---|
| sub | userId.toString(), UUID пользователя |
| jti | UUID.randomUUID().toString(), идентификатор токена |
| iat | issuedAt(Date.from(Instant.now())) |
| exp | expiration(Date.from(expiration)) |
| type | ACCESS или REFRESH; Gateway допускает только ACCESS |
| Access duration | 5 минут, из Duration.of(5, ChronoUnit.MINUTES) |
| Refresh duration | 30 дней, из Duration.of(30, ChronoUnit.DAYS) |
| Формат access/refresh | Один формат, различается type; содержимое secret не предоставлено |
| Подпись | signWith(key); по словам пользователя Auth выбирает HS256/HS384/HS512 по длине key; окончательная политика Gateway не закреплена |
| issuer / audience / rotation | Пользователь сообщил, что отсутствуют |
| Denylist Redis key | `jwt:denylist:<jti>` для access-токена; префикс подтверждён пользователем 2026-09-28 |
| Denylist Redis value | Пустая строка |
| Denylist TTL | Пять минут, подтверждено пользователем 2026-09-28 |

Пустое значение не означает отсутствие записи: критерий отзыва — существование
ключа jti. Не проверять truthiness результата GET и не требовать непустое value.
Ключ строится точной конкатенацией `jwt:denylist:` и проверенного `jti`.
Проверять существование ключа, не значение. Владелец записи и фактическое
поведение работающего Auth/Redis ещё не проверены.

Фраза пользователя «temporal claims ... нету» расходится с fragment: iat и exp
в нём явно установлены. Это не доказательство наличия дополнительных проверок.
Нужна политика Gateway: обязательность iat/exp, проверка exp, допустимый skew,
поведение будущего iat и отсутствие/проверка nbf. Не вычислять exp как iat + 5 минут
при проверке и не заменять отсутствующий exp этим сроком без отдельного решения.

Остаются неизвестны представление secret (raw UTF-8/Base64/другое), точный набор
алгоритмов Gateway, привязка выбранного алгоритма к ключу и правила claim validation.
Не выбирать автоматически алгоритм проверки только по непроверенному JWT header alg.

Отклонённый тип токена/denylist hit — невалидная аутентификация, UNAUTHENTICATED
с нейтральным AUTHENTICATION_REQUIRED без раскрытия причины блокировки.
Неуспешная проверка Redis не равна отсутствию токена в denylist.

Подтверждено пользователем: при Redis timeout/недоступности/ошибке lookup fail-closed,
без вызова Core, UNAVAILABLE с Gateway code AUTH_CHECK_UNAVAILABLE и message
`Authentication service unavailable`. Если раньше завершился context самого RPC,
применяются CANCELLED/DEADLINE_EXCEEDED согласно таблице B, а не Redis error code.

## Предварительные параметры — не production контракт

Источник — последний ответ пользователя; признаки неопределённости сохранены:
«скорее всего», «думаю», «человек не отвечает».

| Параметр | Предварительный вариант | Что ещё требуется |
|---|---|---|
| Secret encoding | UTF-8 bytes | Подтверждение от Auth и точная подготовка key |
| Clock skew | 60 секунд, предложены пользователем | Окончательная validation policy и границы проверки |
| nbf | Необязателен, по сообщению пользователя | Поведение при наличии claim и допустимый skew |
| Redis key | `jwt:denylist:<jti>` | Подтверждён пользователем; runtime совместимость не проверена |
| Redis TTL | Пять минут | Подтверждён пользователем; writer и фактическая политика истечения не проверены |

Подтверждённые ответы не включают окончательный allow-list алгоритмов Gateway.
Не разрешать автоматически HS256/HS384/HS512 и не считать UTF-8 подтверждённым.
Предложение обязательных iat/exp и отклонения будущего iat было задано ранее, но
ответ установил лишь предварительный skew и необязательность nbf; окончательная
политика проверки temporal claims ещё должна быть записана явно.

Вопрос согласованности для Auth: если skew допускает принятие токена после exp,
а запись denylist удаляется через пять минут после отзыва, может возникнуть окно,
когда отозванный токен ещё проходит временную проверку, но ключ уже отсутствует.
Это вывод из предложенной политики, а не подтверждённое поведение сервиса.
Нужно согласовать окно принятия токена и срок denylist; не добавлять exp grace
автоматически.

## Статус завершения и следующий шаг

Шаг 2 завершён: metadata, gRPC statuses, error detail types, публичная фильтрация,
unknown JSON policy, cancellation/deadline и Redis failure response закреплены.
Далее — шаг 3, проверка gRPC компонента и HTTP Core adapter против stub с fixture
identity/request ID. Текущий `cmd/gateway/main.go` уже подключает JWT/Redis verifier
к публичному gRPC listener, но это не означает готовность к production. До
подтверждения key encoding, алгоритмов и temporal policy и проверки реальной
Auth/Redis интеграции такое подключение остаётся открытым нарушением gate.
Предварительные значения не превращать в production defaults.
