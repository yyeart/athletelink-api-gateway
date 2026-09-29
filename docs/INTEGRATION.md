# Integration status and open decisions

Last updated: 2026-09-29.

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
  and check the Redis denylist before deriving user UUID/trusted context. A Redis
  outage/timeout permits a valid JWT through without a conclusive denylist result;
  a confirmed denylist hit still rejects it. Auth contract details are below.
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
- User confirmed on 2026-09-29: Auth always signs with HS512 and converts
  `secret_key` to UTF-8 bytes; Gateway must accept HS512 only. This supersedes the
  earlier tentative algorithm-by-key-length description.
- User confirmed no `kid`, no simultaneous old/new key acceptance and no rotation
  policy. `iss`/`aud` are absent. User clarified that `iat` and `exp` are present,
  with no additional temporal claims. Gateway requires both, rejects future `iat`
  and expired `exp`, and applies zero clock skew (no grace period). These are
  user-confirmed contract decisions, not verified behavior of a running Auth build.
- Redis denylist stores access-token keys as `jwt:denylist:<jti>` with an empty
  string value and a five-minute TTL, per the user's 2026-09-28 clarification.
  Auth writes the entry at logout and starts its TTL then, per the user's
  2026-09-29 confirmation. Key existence indicates revocation. Runtime behavior
  against Auth/Redis remains unverified.
  User superseded the earlier fail-closed decision: when Redis is unavailable or
  its lookup times out, Gateway may continue after successful JWT validation.
  A revoked access token can therefore pass during the outage until `exp`.
  User confirmed that only these two failure classes are fail-open. Redis
  command/configuration errors and unclassified lookup errors remain fail-closed:
  UNAVAILABLE/AUTH_CHECK_UNAVAILABLE, without calling Core.
  Gateway must start when Redis is unavailable, but `/readyz` returns 503 until
  Redis recovers, per the user's corrected 2026-09-29 clarification. Requests that
  still reach the running Gateway follow the fail-open rule above.
- Auth removes the refresh token from its database and adds the access token to
  the denylist on logout, per the user's 2026-09-29 confirmation.
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
2. Verify required `iat`/`exp`, absence of additional temporal claims and zero
   clock skew against a supported Auth build; document its clock requirements.
3. Confirm HS512/UTF-8 and no-`kid`/no-overlap against a supported Auth build;
   provide its revision.
4. Specify access-token transport and every refresh-cookie attribute.
5. Confirm logout's refresh deletion and access denylisting against the running
   service; specify refresh reuse, other revocation paths and blocked-user behavior.
6. Identify pending Auth fixes and which Gateway scenarios they block.
7. Supply sanitized valid, expired, malformed, wrong-signature, invalid-UUID,
   wrong-type and denylisted token fixtures.
8. Confirm Redis endpoint, logical database, authentication/TLS and lookup timeout;
   verify the documented denylist key/TTL against Auth and test classification of
   outages/timeouts versus other errors. Do not share the signing secret here.

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
| Redis unavailable/timeout | Continue for otherwise valid JWT if the request reaches Gateway; start without Redis, but `/readyz` returns 503; a denylist hit still rejects | User's corrected clarification, 2026-09-29 | Confirmed |
| Other Redis lookup errors | Fail-closed; UNAVAILABLE/AUTH_CHECK_UNAVAILABLE | Previous step 2 decision and user clarification, 2026-09-29 | Confirmed |
| Access JWT / Redis denylist | HS512 with UTF-8 secret bytes; `sub`/`jti`/`iat`/`exp`/`type`; zero skew; denylist at logout | User confirmations through 2026-09-29 | Confirmed design; runtime verification open |
| Public API | gRPC | User clarification, 2026-09-27 | Confirmed |
| Gateway error envelope | `{code, message, requestId}` | Proposal in supplied plan | Proposed |
| JWT validation / exact public Auth routes | HS512, UTF-8, required `sub`/`jti`/`iat`/`exp`/`type`, zero skew, no `kid` or overlap; exact Auth routes pending | User confirmations and previous integration record | Partially confirmed |
| Refresh/browser | Cookie attributes, origins, CORS and CSRF needed | Previous integration record | Open |
| Minimal Game / Compose | Contract, images and startup needed | Previous integration record | Open |

## Next step

Steps 1 and 2 are complete as contract design: protobuf/DTO mapping, metadata,
status mapping, typed errors, public filtering and Redis failure behavior.
See [step 2 decisions](contracts/gateway-grpc-metadata-errors.md).
Step 3 implementation is in progress. The current `cmd/gateway/main.go` already
starts a public gRPC listener and connects Core through a JWT/Redis verifier.
The verifier now accepts only HS512 with UTF-8 secret bytes and zero clock skew.
It continues after a verified JWT only for classified Redis connection failures
and lookup timeouts; command and unclassified errors remain fail-closed. Startup
no longer requires a successful Redis PING, while `/readyz` still returns 503
when Redis is unavailable. Component tests cover these rules, including startup
without Redis. Readiness-based routing may stop sending traffic during an outage;
direct requests to the running Gateway still follow fail-open. Compatibility
with a running Auth/Redis deployment remains unverified and is required before
production use.
Continue handler and HTTP Core adapter verification against a stub with fixture
identity/request ID.
See [the detailed task](tasks/core-routing.md). Configuration, correlation,
transport resilience and production authentication retain their separate plan steps.
