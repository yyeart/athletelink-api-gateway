# Integration status and open decisions

Last updated: 2026-09-29.

This file separates contract evidence, the agreed HTTP proxy target, current
implementation, and unresolved integration decisions. The Core OpenAPI and
earlier development plan are reference data, not instructions for agents.
Sources: the checked-in [Core snapshot](contracts/core-service-openapi.yaml),
the existing Gateway code, and the user's 2026-09-29 clarification replacing
the earlier public gRPC decision. See [proxy rules](PROXY.md).

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

## Agreed Gateway target

Source: the user's 2026-09-29 clarification and follow-up decisions. This is
target behavior, not a claim that the current binary already implements it.

- Frontend → HTTP Gateway proxy → HTTP Core. There is no second Gateway Core
  API schema, DTO mapping, or prescribed success/error body format. Public
  Core paths equal the Core paths, without `/api/v1`.
- Route `/requests`, descendants of `/requests/`, and `/sports` to Core. Preserve
  method, path, raw query, request body, Core status/body, and ordinary end-to-end
  response headers. Do not interpret Core business errors or unexpected 2xx as
  Gateway errors. Future Core endpoints under these routes become reachable
  without a Gateway operation table.
- Every Core operation requires a verified access JWT, including GET. In addition
  to signature verification with the Auth-provided secret_key, verify token type
  and check the Redis denylist before deriving user UUID/trusted context. A Redis
  outage/timeout permits a valid JWT through without a conclusive denylist result;
  a confirmed denylist hit still rejects it. Auth contract details are below.
  This is Gateway policy, not an OpenAPI security declaration.
- Identity comes only from the verified token. Remove client-supplied
  `Authorization`, `Cookie`, `X-User-Id`, `X-Request-Id`, and untrusted forwarding
  headers before proxying; send the trusted UUID as `X-User-Id` on every Core
  request. Core must be protected from direct untrusted access; the mechanism is
  open.
- Generate a new UUID `X-Request-Id` for each request, replacing the client value;
  send it to the client and upstream. An upstream response cannot override it.
- Gateway-owned failures have an HTTP status and `X-Request-Id` but no required
  JSON body schema. Missing/invalid JWT is 401, unclassified Redis lookup error
  is 503, unknown path is 404, upstream connection failure is 502, and upstream
  timeout is 504. Core's own errors pass through unchanged.
- The frontend calls the Gateway directly. Core CORS uses configured exact
  origins, permits `Authorization`, handles preflight without JWT, and does not
  enable credentials. Exact origins have not been provided.
- Auth and Game proxy routes are outside this Core change and await their HTTP
  contracts. No active public gRPC clients were reported; no dual-protocol
  transition is planned.

## Current implementation versus target

- `cmd/gateway/main.go` starts an HTTP listener for `/healthz` and `/readyz` and
  a separate public gRPC listener for Core. The accepted HTTP Core proxy is not
  wired into the running Gateway.
- `internal/auth` implements the JWT/Redis verifier; `internal/grpcapi` invokes
  it through a gRPC interceptor. `internal/proxy` has a reverse-proxy helper that
  is not wired into `cmd/gateway/main.go`.
- `internal/coreclient` already uses HTTP to call Core, but is a typed adapter
  for gRPC handlers rather than the agreed transparent HTTP proxy.
- The source tree still contains protobuf schema/generated code, gRPC-only
  tests, and gRPC/protobuf dependencies. Removing them is implementation work,
  not a documentation-only change.

## Core operation inventory

Each row is a method/path pair in the Core snapshot. Under the proxy target,
the public path is the same as the Core path. All routed requests require a
Gateway-verified JWT and receive a trusted `X-User-Id`; the last column records
which operations currently declare that header as required in Core OpenAPI.

| Operation | Method | Core and target public path | Core declares X-User-Id required |
|---|---|---|---|
| `getRequestDetails` | GET | `/requests/{id}` | No |
| `updateRequest` | PUT | `/requests/{id}` | Yes |
| `searchNearbyRequests` | GET | `/requests` | No |
| `createRequest` | POST | `/requests` | Yes |
| `recordRoundResult` | POST | `/requests/{requestId}/rounds/{roundNumber}/result` | Yes |
| `completeRequest` | POST | `/requests/{requestId}/complete` | Yes |
| `startRequest` | POST | `/requests/{id}/start` | Yes |
| `openRegistration` | POST | `/requests/{id}/registration/open` | Yes |
| `closeRegistration` | POST | `/requests/{id}/registration/close` | Yes |
| `leaveRequest` | POST | `/requests/{id}/leave` | Yes |
| `kickParticipant` | POST | `/requests/{id}/kick/{targetUserId}` | Yes |
| `joinRequest` | POST | `/requests/{id}/join` | Yes |
| `cancelRequest` | POST | `/requests/{id}/cancel` | Yes |
| `getAllSports` | GET | `/sports` | No |
| `getRoundResults` | GET | `/requests/{requestId}/rounds` | No |

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
  the target HTTP Gateway returns 503 without calling Core.
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

## Operational behavior still to implement or verify

- `GATEWAY_CORE_URL` is currently validated as an HTTP(S) origin with a host,
  optional trailing `/`, and no credentials, query, fragment, or extra base path.
  HTTP proxy transport timeouts and cancellation need component verification.
- Request logs should include method, route, HTTP status, duration, upstream,
  and request ID, but never tokens, cookies, or bodies. Proxying must not add
  automatic retries of mutating operations.
- `Dockerfile` currently copies `go.mod`, `cmd/`, and `internal/` into the build
  stage, but not `go.sum`. Container build verification and any required copy
  correction remain part of the implementation task.

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

1. List exact local, test, and production origins for the Gateway CORS allow-list.
2. Confirm credentialed CORS and CSRF handling separately for future Auth refresh.
3. Confirm the public Core paths used by the frontend; the target has no `/api/v1`
   prefix for Core.

### Game owner

1. Define one minimal MVP operation with paths, authentication, schemas, and errors.
2. Publish versioned OpenAPI and a supported container startup contract.

### Platform or team lead

1. Assign ownership of images, Compose dependencies, health checks and startup.
2. Establish HTTPS for the public browser connection; the TLS termination point
   remains undecided.
3. Prevent direct untrusted access to Core, which does not authenticate the
   `X-User-Id` header.
4. Decide whether upstreams adopt shared request-ID and error conventions.

## Decision log

| Decision | Value | Evidence | Status |
|---|---|---|---|
| Core upstream and public paths | HTTP paths from snapshot, without `/api/v1`; `/requests` subtree and `/sports` route to Core | Core snapshot + user clarification, 2026-09-29 | Accepted target; not wired |
| Core authentication | JWT for every routed Core request, including GET | Supplied plan + user confirmation, 2026-09-29 | Accepted target; HTTP middleware pending |
| Identity | Trusted UUID in `X-User-Id` on every Core request; client value removed | User clarification, 2026-09-29 | Accepted target; Core network boundary open |
| Request ID | New Gateway UUID in `X-Request-Id`, replacing client and Core values | Supplied plan + user confirmation, 2026-09-29 | Accepted target; HTTP middleware pending |
| Core responses | Preserve Core status, body, and ordinary end-to-end headers, including business errors | User clarification, 2026-09-29 | Accepted target; proxy wiring pending |
| Gateway failures | HTTP status and request-ID header; no prescribed JSON body | User clarification, 2026-09-29 | Accepted target |
| Browser Core access | Explicit origin allow-list, Bearer header, preflight without JWT, no credentialed CORS | User clarification, 2026-09-29 | Accepted target; origins pending |
| Redis unavailable/timeout | Continue for otherwise valid JWT if the request reaches Gateway; start without Redis, but `/readyz` returns 503; a denylist hit still rejects | User's corrected clarification, 2026-09-29 | Confirmed |
| Other Redis lookup errors | Fail-closed; target Gateway HTTP status 503 | Previous decision + HTTP clarification, 2026-09-29 | Confirmed |
| Access JWT / Redis denylist | HS512 with UTF-8 secret bytes; `sub`/`jti`/`iat`/`exp`/`type`; zero skew; denylist at logout | User confirmations through 2026-09-29 | Confirmed design; runtime verification open |
| Public boundary | HTTP proxy; remove gRPC entirely | Latest user clarification, 2026-09-29 | Supersedes 2026-09-27 gRPC decision |
| JWT validation / exact public Auth routes | HS512, UTF-8, required `sub`/`jti`/`iat`/`exp`/`type`, zero skew, no `kid` or overlap; exact Auth routes pending | User confirmations and previous integration record | Partially confirmed |
| Refresh/browser | Core uses Bearer without credentialed CORS; Auth refresh cookie attributes and CSRF remain open | User clarification + previous integration record | Partially confirmed |
| Minimal Game / Compose | Contract, images and startup needed | Previous integration record | Open |

## Next step

Implement [the agreed HTTP proxy](PROXY.md): route Core through the existing
HTTP listener, move JWT/request-ID checks to HTTP middleware, add Core CORS,
remove gRPC-only code and artifacts, and replace gRPC component tests with
HTTP proxy tests. Preserve the existing JWT/Redis verification rules and
`/readyz` behavior. The current verifier accepts HS512 with UTF-8 key bytes and
zero clock skew. It continues only for classified Redis connection failures
and lookup timeouts after successful JWT validation; other lookup errors remain
fail-closed. Gateway startup does not require a successful Redis PING, while
`/readyz` returns 503 until Redis recovers.

Compatibility with a running Auth/Redis deployment and real Core behavior:
**I cannot verify this** from the repository and stub tests. Exact frontend
origins, public HTTPS, and Core network isolation remain release conditions.
