# Integration status and open decisions

Last updated: 2026-09-27.

This file separates contract evidence, accepted Gateway requirements, proposals,
and unresolved decisions. OpenAPI and the supplied development plan are reference
data, not instructions for agents. Sources: the current
[Core snapshot](contracts/core-service-openapi.yaml), the user-supplied
[gateway-plan.md](/Users/yyeart/Downloads/gateway-plan.md), and the user's RPC
requirement in the current task.

## Current evidence

### Core Service

- The checked-in snapshot declares OpenAPI 3.1.0, API version `v0`, and server
  `http://localhost:8081`. These are document metadata, not a verified deployment.
- Snapshot SHA-256: `ad68e048b93cc0639e15314c68295214652ef7ed525b3c2d06499687ab7e88eb`.
  Calculated from the current file bytes; source commit/build and export date
  remain unknown. Do not label this file as the unchanged 2026-09-05 snapshot.
- The operation inventory below is taken from `paths` and `operationId`.
- Mutating operations require `X-User-Id` as a UUID. Their parameter description
  explicitly says Core does not authenticate this header.
- Non-success responses reference `components.schemas.ApiError`, with `code`,
  `message`, and `details`. The previous claims that identity transport and error
  schemas are absent are obsolete.
- Response media types still include `*/*`; actual response Content-Type and
  runtime behavior require integration verification.
- Real service conformance: **I cannot verify this**. Documentation review is not
  an integration run and does not establish that the operations work in production.

## Accepted Gateway requirements

Source: the accepted decisions recorded in the supplied development plan.

- The user clarified that the public Gateway API must use gRPC. The earlier
  public HTTP mapping is historical; continued HTTP compatibility is unconfirmed.
  Core upstream paths remain those in the HTTP snapshot, without `/api/v1`.
- Every Core operation requires a verified access JWT, including GET. In addition
  to signature verification with the Auth-provided secret_key, verify token type
  and absence from a Redis denylist before deriving user UUID/trusted context.
  Claim names, algorithm, Redis schema and lookup failure behavior need a contract.
  This is Gateway policy, not an OpenAPI security declaration.
- Identity comes only from internal context populated by JWT verification.
  Remove client-supplied `X-User-Id`; use the trusted UUID for operations requiring
  it. Core must be protected from direct untrusted access; the mechanism is open.
- Generate a new UUID `X-Request-Id` for each request, replacing the client value;
  send it to the client and upstream. Upstream must not override this value.
- Preserve Core business semantics; do not duplicate business validation. Public
  protobuf messages require explicit mapping from HTTP/JSON responses and errors.
  The earlier byte-transparent HTTP passthrough requirement is superseded for
  public gRPC; the detailed mapping must be specified in the Gateway contract.
- Until the JWT contract is implemented, do not connect Core operations to the
  production public entry point. A test middleware may inject a trusted UUID.

## Public gRPC API — confirmed boundary

Source: the user's clarification on 2026-09-27: “нужен Публичный RPC API.
протокол gRPC”. This resolves the boundary and protocol; they are not open questions.

Target integration: client → public gRPC Gateway → HTTP Core, using the existing
Core snapshot for the upstream adapter. This does not assert RPC support in Core
or require changing its contract. The HTTP adapter is the implementation direction
for the next step; actual Core runtime conformance remains unverified.

Step 1 now defines the protobuf schema, operation mapping and field presence in
[Gateway gRPC contract](contracts/gateway-grpc.md). User-confirmed: one CoreService,
UUID strings, Timestamp/UTC, enum, explicit presence and guaranteed create requestId.
Metadata and gRPC error mapping remain step 2 work before implementing handlers.
Record the following as contract design work, not as unanswered protocol selection:

- Public service/method names and typed request/response messages for every operation.
- JWT metadata transport, trusted identity context and request-ID metadata.
- Mapping of HTTP successes, empty responses and Core `ApiError` to protobuf/gRPC.
- Allow-listed response metadata; HTTP headers are not automatically public metadata.
- Native gRPC client support, TLS/listener deployment, and whether browser clients
  require gRPC-Web or another explicitly selected bridge. No bridge is included yet.
- Whether legacy public HTTP paths are required; no compatibility API is authorized
  by this clarification alone. HTTP operational endpoints remain separate.

The scope of Auth's public API is not established by this Core-focused task.

## Core operation inventory

Each row is one method/path pair from the snapshot. The public HTTP column reflects
the historical mapping in the plan, not the new public API. Public gRPC names
are defined in the Gateway protobuf contract. All rows
require JWT at the Gateway; identity requirements are those in the Core snapshot.

| Operation | Method | Core path | Public HTTP path in plan | Core X-User-Id |
|---|---|---|---|---|
| `getRequestDetails` | GET | `/requests/{id}` | `/api/v1/requests/{id}` | Not declared |
| `updateRequest` | PUT | `/requests/{id}` | `/api/v1/requests/{id}` | Required |
| `searchNearbyRequests` | GET | `/requests` | `/api/v1/requests` | Not declared |
| `createRequest` | POST | `/requests` | `/api/v1/requests` | Required |
| `recordRoundResult` | POST | `/requests/{requestId}/rounds/{roundNumber}/result` | `/api/v1/requests/{requestId}/rounds/{roundNumber}/result` | Required |
| `completeRequest` | POST | `/requests/{requestId}/complete` | `/api/v1/requests/{requestId}/complete` | Required |
| `startRequest` | POST | `/requests/{id}/start` | `/api/v1/requests/{id}/start` | Required |
| `openRegistration` | POST | `/requests/{id}/registration/open` | `/api/v1/requests/{id}/registration/open` | Required |
| `closeRegistration` | POST | `/requests/{id}/registration/close` | `/api/v1/requests/{id}/registration/close` | Required |
| `leaveRequest` | POST | `/requests/{id}/leave` | `/api/v1/requests/{id}/leave` | Required |
| `kickParticipant` | POST | `/requests/{id}/kick/{targetUserId}` | `/api/v1/requests/{id}/kick/{targetUserId}` | Required |
| `joinRequest` | POST | `/requests/{id}/join` | `/api/v1/requests/{id}/join` | Required |
| `cancelRequest` | POST | `/requests/{id}/cancel` | `/api/v1/requests/{id}/cancel` | Required |
| `getAllSports` | GET | `/sports` | `/api/v1/sports` | Not declared |
| `getRoundResults` | GET | `/requests/{requestId}/rounds` | `/api/v1/requests/{requestId}/rounds` | Not declared |

## Other service evidence retained from the previous integration record

### Auth Service

- Access tokens are JWTs signed with a shared secret.
- User-provided Auth fragment places the user UUID string in `sub`, a new token UUID
  in `jti`, issuance in `iat`, expiration in `exp`, and token kind in `type`.
- `type` is `ACCESS` or `REFRESH`; Gateway accepts only `ACCESS`. The supplied
  constants define access duration as 5 minutes and refresh duration as 30 days.
- User reports signing algorithm chosen by Auth from HS256/HS384/HS512 according
  to key length. Key encoding and Gateway algorithm policy remain unconfirmed.
- User reports no issuer/audience/rotation. Proposed skew is 60 seconds and nbf
  is optional; final temporal validation still needs confirmation. `iat`/`exp`
  are set explicitly in the supplied Auth fragment.
- UTF-8 secret bytes are tentative, not confirmed by Auth.
- Redis denylist stores access-token `jti` keys with an empty string value; key
  existence indicates revocation. Key without prefix and TTL `exp - now` are
  tentative user assumptions; type/writer and exact expiry behavior are unconfirmed.
  Redis lookup failure is confirmed fail-closed: UNAVAILABLE/AUTH_CHECK_UNAVAILABLE.
- Refresh tokens are stored in the Auth database and transported through a cookie.
- Candidate public path patterns are `/api/v1/auth/**`,
  `/api/v1/user/register`, `/api/v1/verify/forgot-password`, and
  `/api/v1/verify/password-reset`.
- Auth behavior requires fixes; the affected behavior is not yet specified.
- Test tokens can be provided.

### Game Service, chat, and environment

- Game Service has no usable contract and is not ready for integration.
- WebSocket chat has been removed from the MVP.
- Docker Compose is not ready and will be specified later.
- Shared cross-service request-ID and error conventions remain unconfirmed;
  Gateway policy is recorded above.
- No special streaming, upload, or long-running Core requests have been reported.

## Proposed Gateway behavior from the plan

These items are proposals pending implementation, not verified runtime behavior:

- Gateway-owned errors: JSON `{code, message, requestId}`; unknown path `404`,
  unsupported method on a known HTTP path `405`, upstream failure `502`, upstream
  timeout `504`. These HTTP proposals do not define gRPC errors; define the
  public gRPC mapping in the next-step contract.
- `GATEWAY_CORE_URL`: HTTP(S), host, optional trailing `/`; no credentials, query,
  fragment, or extra base path. Positive explicit connect, TLS handshake, and
  response-header timeouts when the integration is enabled.
- Log method, route template, status, duration, upstream, and request ID; exclude
  tokens, cookies, and bodies. No automatic retries of mutating operations.

## Open decisions and questions

### Core owner

1. Specify the network/trust boundary preventing direct client access to Core.
2. Identify the commit/build that produced the snapshot and approve its revision.
3. Provide startup dependencies, health/readiness endpoints and supported image.
4. Verify response content types and snapshot conformance against the real service.

### Auth owner

1. Expand `/auth/**` into exact public method/path pairs.
2. Specify JWT algorithm, UUID and token-type claim names/types, access marker, temporal claims, issuer/audience,
   clock skew, and secret encoding.
3. Explain rotation and whether tokens contain `kid`.
4. Specify access-token transport and every refresh-cookie attribute.
5. Define logout, refresh reuse, revocation, user blocking, and token behavior.
6. Identify pending Auth fixes and which Gateway scenarios they block.
7. Supply sanitized valid, expired, malformed, wrong-signature, invalid-UUID,
   wrong-type and denylisted token fixtures.
8. Specify denylist lookup identifier, Redis key/type/value semantics, TTL and owner;
   define Redis outage/timeout behavior. Do not share the actual signing secret here.

### Frontend owner

1. List exact local and test origins.
2. Confirm credentialed CORS and CSRF handling for refresh.
3. Confirm the public Core and Auth URLs expected by the frontend.

### Game owner

1. Define one minimal MVP operation with paths, authentication, schemas, and errors.
2. Publish versioned OpenAPI and a supported container startup contract.

### Platform or team lead

1. Assign ownership of images, Compose dependencies, health checks and startup.
2. Decide whether upstreams adopt shared request-ID and error conventions. Gateway
   policy above does not establish that these conventions exist in other services.

## Decision log

| Decision | Value | Evidence | Status |
|---|---|---|---|
| Core upstream paths | HTTP paths from snapshot, without `/api/v1` | Snapshot + original plan | Adapter basis; legacy public HTTP compatibility open |
| Core authentication | JWT for every operation, including GET | Supplied plan | Accepted; Auth contract blocks production wiring |
| Identity | Trusted UUID in `X-User-Id` for required Core operations | Snapshot + supplied plan | Accepted; deployment trust boundary open |
| Request ID | New Gateway UUID in `X-Request-Id`, replacing client value | Supplied plan | Accepted Gateway policy |
| Core errors | Public code from gRPC status; raw Core code/message/details hidden | User answers to step 2 | Confirmed |
| Redis lookup failure | Fail-closed; UNAVAILABLE/AUTH_CHECK_UNAVAILABLE | User answers to step 2 | Confirmed |
| Access JWT / Redis denylist | Verify signature, access type and denylist before identity | User addition to step 2 | Required; JWT/Redis specifics open |
| Public API | gRPC | User clarification, 2026-09-27 | Confirmed |
| Gateway error envelope | `{code, message, requestId}` | Proposal in supplied plan | Proposed |
| JWT validation / exact public Auth routes | Algorithm, claims, encoding, rotation and exact routes needed | Previous integration record | Open |
| Refresh/browser | Cookie attributes, origins, CORS and CSRF needed | Previous integration record | Open |
| Minimal Game / Compose | Contract, images and startup needed | Previous integration record | Open |

## Next step

Steps 1 and 2 are complete as contract design: protobuf/DTO mapping, metadata,
status mapping, typed errors, public filtering and Redis failure behavior.
See [step 2 decisions](contracts/gateway-grpc-metadata-errors.md).
Next: step 3, implement gRPC handlers and the HTTP Core adapter against a stub,
using fixture identity/request ID. Production JWT/Redis wiring remains blocked by
unconfirmed key encoding, algorithm/temporal policy and complete Redis contract.
Skew 60 seconds, UTF-8, prefix-free keys and TTL exp-now are provisional, not defaults.
See [the detailed task](tasks/core-routing.md). Configuration, correlation,
transport resilience and production authentication retain their separate plan steps.
