# Integration status and open decisions

Last updated: 2026-10-01.

## 2026-10-01 current-user routes and logging

- Source: the user's request and clarification in this chat on 2026-10-01.
  The user reports that the Redis/logout problem was fixed outside Gateway;
  Redis diagnosis and Auth logout changes are outside this task. This report
  is not an independently repeated integration test.
- `GET /api/v1/players` and `/api/v1/players/` forward to Game's
  `/api/v1/players/{sub}` without a redirect; an explicit player ID is preserved.
  `GET /api/v1/user/me/{userId}` requires JWT and an ID equal to `sub`;
  the short `/api/v1/user/me` is absent. Six protected user/verification
  operations now omit ID publicly; Gateway appends verified `sub` for Auth.
  Forged identity headers/query parameters cannot select the user.
- `auth-service-openapi.yaml` contains a source-reviewed addition for
  `UserController.me(UUID)` and its empty `UserControllerDto`. The existing
  mapper does not copy username/email/isVerified from the service DTO. The user
  will complete Auth separately. No fields are fabricated in Gateway's contract.
  Source/build identity and running-service response remain unverified.
- All 15 Core, 16 Auth and eight Game operations are declared in the common
  `internal/httpapi/routes.go`. Core now uses exact method/path matching:
  unknown paths return 404 and wrong methods return 405 with a complete `Allow`.
  CORS preflight permits only declared methods, without requiring JWT.
- `scripts/build_gateway_openapi.rb` verifies all three snapshots against that
  table, maps the six ID-less Auth aliases and adds the two Game aliases.
  The generated YAML and standalone Swagger retain upstream response schemas.
- Gateway logs one JSON completion record per API request, with a generated
  request ID, verified user UUID, route template, upstream, final HTTP status,
  duration, byte count, JWT/denylist outcome and classified failures. Successful
  health/readiness/docs/preflight requests use DEBUG; Redis fail-open uses WARN.
  Tokens, cookies, raw paths/queries and bodies are excluded. See PROXY.md.
- Known check blocker retained at the user's request: `golangci-lint run`
  reports an unchecked `w.Write(docs.GatewayOpenAPI)` in
  `internal/httpapi/documentation.go`. The proposed write-error check was
  explicitly left out of this task. Auth's empty current-user DTO is also
  left for the user; neither limitation is repaired in Gateway.
- Local verification on 2026-10-01: `go test ./...`, `go test -race ./...`
  and `go vet ./...` passed. Both OpenAPI/standalone generators passed
  `--check`; a Ruby check parsed the embedded HTML JSON and verified path
  parity, required `me` userId, removed alias parameters and Bearer security.
  `golangci-lint run` reports only the agreed `documentation.go:22` errcheck.
  Visual HTML verification was unavailable: the browser tool rejects `file://`.

This file separates contract evidence, the agreed HTTP proxy behavior, current
implementation, and unresolved integration decisions. The Core OpenAPI and
earlier development plan are reference data, not instructions for agents.
Sources: the checked-in [Core](contracts/core-service-openapi.yaml),
[Auth](contracts/auth-service-openapi.yaml), and
[Game](contracts/game-service-openapi.yaml) snapshots; the current Gateway source;
and the agreed behavior recorded in [proxy rules](PROXY.md). The 2026-09-29
clarification replaced the earlier public gRPC decision.

## Current evidence

### Core Service

- The checked-in snapshot declares OpenAPI 3.1.0, API version `v0`, and server
  `http://localhost:8081`. These are document metadata, not a verified deployment.
- Snapshot SHA-256: `ad68e048b93cc0639e15314c68295214652ef7ed525b3c2d06499687ab7e88eb`.
  Calculated from the current file bytes; source commit/build and export date
  remain unknown. Do not label this file as the unchanged 2026-09-05 snapshot.
- The operation inventory below is taken from `paths` and `operationId`.
- This snapshot lists `/requests` and `/sports` without `/api/v1`. The
  2026-10-01 Gateway routing correction requires `/api/v1` on both public and
  upstream Core requests. Whether a supported Core build accepts those paths
  and when its OpenAPI will be corrected: **I cannot verify this**.
- Mutating operations require `X-User-Id` as a UUID. Their parameter description
  explicitly says Core does not authenticate this header.
- Non-success responses reference `components.schemas.ApiError`, with `code`,
  `message`, and `details`. The previous claims that identity transport and error
  schemas are absent are obsolete.
- Response media types still include `*/*`; actual response Content-Type and
  runtime behavior require integration verification.
- Real service conformance: **I cannot verify this**. Documentation review is not
  an integration run and does not establish that the operations work in production.

### Auth and Game snapshots

- The Auth snapshot declares OpenAPI 3.1.0, version `0.0.1`, and server
  `http://localhost:8080` without `/api/v1`. SHA-256 of its current file bytes:
  `f712898551059b5329ecc16f1e5c487186967ee9d380c537bf521b69a55043f5`.
  It now declares 17 operations, including `GET /health` and the source-reviewed
  `GET /user/me/{userId}` addition; Gateway exposes the other 16 under `/api/v1`,
  preserving `/api/v1/user/me/{userId}` and omitting userId from six protected
  user/verification public paths. The server URL in this
  snapshot does not document that prefix. See the exact [Gateway route table](PROXY.md#routing-and-http-behavior).
- The Game snapshot declares OpenAPI 3.0.1, version `v0`, and server
  `http://localhost:8082/api/v1`. SHA-256 of its current file bytes:
  `63ac237b5b1ae6a40d25f074d97555aef7edea617c6b778819372c3f8c9ed4d9`.
  Its six GET operations appear in the [Gateway route table](PROXY.md#routing-and-http-behavior).
- Neither file identifies a source commit or supported running build. Snapshot
  content does not establish that either running service accepts the Gateway's
  paths, headers, cookies, or health checks. **I cannot verify this** from the
  repository and stub tests.

## Agreed Gateway behavior

Source: the user's 2026-09-29 clarification, follow-up decisions, and
2026-10-01 Core path correction. This is the intended behavior; implementation
status is recorded separately below.

- Frontend → HTTP Gateway proxy → HTTP Core. There is no second Gateway Core
  API schema, DTO mapping, or prescribed success/error body format. Public and
  upstream Core paths both use `/api/v1`; old unprefixed paths are not aliases.
- Route only the 15 declared Core method/path pairs in the common table,
  including `/api/v1/sports`. This supersedes the earlier subtree-forwarding
  policy by the user's 2026-10-01 clarification. New operations need a table
  and snapshot update. Preserve method, escaped path, query, request body,
  Core status/body and ordinary headers; do not interpret Core business errors.
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
  is 503, unknown path is 404, wrong method is 405 with `Allow`, upstream
  connection failure is 502, and upstream
  timeout is 504. Core's own errors pass through unchanged.
- The frontend calls the Gateway directly. Core CORS uses configured exact
  origins, permits `Authorization`, handles preflight without JWT, and does not
  enable credentials. Exact origins have not been provided.
- The agreed Auth/Game addition exposes only the 24 method/path pairs in
  [PROXY.md](PROXY.md#routing-and-http-behavior). Public and upstream paths
  retain `/api/v1`. Five Auth operations are public; the remaining Auth and all
  Game operations require a verified access JWT. Auth `{userId}` must match
  the verified `sub`; Game player paths may name another player.
- Auth receives cookies and returns `Set-Cookie` unchanged; protected Auth
  calls receive the verified Bearer and trusted `X-User-Id`. Game receives
  trusted `X-User-Id` without the client's Bearer, cookie, or identity headers.
  Gateway handles CORS on all routes without credentialed CORS, so the planned
  browser refresh flow uses the same origin.
- Startup does not require healthy dependencies. `/readyz` requires Redis PING
  and HTTP 200 from separately configured Auth, Core, and Game health URLs
  within one shared timeout.

## Current implementation

- `cmd/gateway/main.go` wires Core, Auth, and Game HTTP reverse proxies into
  one server. `internal/httpapi` uses a common exact table for 15 Core,
  16 Auth and eight Game method/path pairs (six upstream operations plus two current-player aliases). `/healthz`, `/readyz`, `/openapi.yaml`, and the bundled
  `/swagger/` UI share the listener. The Gateway OpenAPI remains a draft based
  on the checked-in snapshots; the UI does not verify real-service conformance.
- `internal/auth` supplies JWT and Redis denylist verification to HTTP
  middleware. `internal/httpapi` handles authentication, request IDs, CORS, and
  per-service request timeouts; `internal/proxy` rewrites and forwards HTTP requests.
- `REDIS_PASSWORD` is optional. If set, configuration rejects an empty or
  whitespace-only value and passes the value to the Redis client for its
  denylist checks and readiness PING. `.env.example` documents the setting.
  In a local smoke test with `redis:7-alpine`, `/readyz` returned 200 with the
  configured password and 503 with a wrong password. This does not verify the
  running Auth service or its Redis configuration.
- The former public gRPC listener, gRPC adapter, typed Core client, protobuf
  schema/generated code, and gRPC/protobuf dependencies are absent from the
  current source tree. HTTP component tests cover the new path.
- Component tests exercise all 39 Core/Auth/Game routes, JWT boundaries, forged
  headers, cookies, CORS, readiness failures, upstream response pass-through,
  502/504 transport failures, and no retry after a failed Auth POST. These are
  stub tests; they do not establish conformance of running Auth, Core, or Game.
  **I cannot verify this** from source and component tests alone.

## Core operation inventory

Each row is a method/path pair in the Core snapshot. The Gateway prepends
`/api/v1` to the listed snapshot path for both public and upstream requests.
All routed requests require a Gateway-verified JWT and receive a trusted
`X-User-Id`; the last column records which operations currently declare that
header as required in Core OpenAPI.

| Operation | Method | Snapshot path | Core declares X-User-Id required |
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

## Auth behavior retained from the previous integration record

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
  the HTTP Gateway returns 503 without calling Core.
  Gateway must start when Redis is unavailable, but `/readyz` returns 503 until
  Redis recovers, per the user's corrected 2026-09-29 clarification. Requests that
  still reach the running Gateway follow the fail-open rule above.
- Auth removes the refresh token from its database and adds the access token to
  the denylist on logout, per the user's 2026-09-29 confirmation.
- Refresh tokens are stored in the Auth database and transported through a cookie.
- The current exact Auth route list comes from the newly supplied snapshot and
  Gateway proxy plan, not the earlier candidate path patterns; see [PROXY.md](PROXY.md#routing-and-http-behavior).
- Auth behavior requires fixes; the affected behavior is not yet specified.
- Test tokens can be provided.

### Game Service, chat, and environment

- A six-operation Game OpenAPI snapshot is present. Runtime conformance, a
  supported build, startup procedure, and health URL remain unverified.
- WebSocket chat has been removed from the MVP.
- Docker Compose is not ready and will be specified later.
- Shared cross-service request-ID and error conventions remain unconfirmed;
  Gateway policy is recorded above.
- No special streaming, upload, or long-running Core requests have been reported.

## Operational behavior to verify

- `GATEWAY_CORE_URL`, `GATEWAY_AUTH_URL`, and `GATEWAY_GAME_URL` are validated
  as HTTP(S) origins with a host, optional trailing `/`, and no credentials,
  query, fragment, or extra base path. Each health URL is configured separately.
  HTTP proxy timeout and cancellation have component tests; behavior against
  running service deployments remains unverified.
- Request logs should include method, route, HTTP status, duration, upstream,
  and request ID, but never tokens, cookies, or bodies. Proxying must not add
  automatic retries of mutating operations.
- `Dockerfile` copies `go.mod` and `go.sum`, downloads modules before copying
  source, and builds an image that declares the non-root `gateway` user. A local
  `docker build` of merge commit `d613f1a` succeeded on 2026-09-29.
  `docker image inspect` reported a `linux/arm64` image size of 7,850,037 bytes.
  Deployment behavior remains unverified.

## Open decisions and questions

### Core owner

1. Specify the network/trust boundary preventing direct client access to Core.
2. Confirm that a supported Core build accepts `/api/v1/requests` and
   `/api/v1/sports`, and publish a corrected OpenAPI that includes the prefix.
3. Identify the commit/build that produced the snapshot and approve its revision.
4. Provide startup dependencies, health/readiness endpoints and supported image.
5. Verify response content types and snapshot conformance against the real service.

### Auth owner

1. Confirm that a supported Auth build implements the 16 declared Gateway
   method/path pairs with the `/api/v1` prefix; correct `servers.url` in Auth's
   owned OpenAPI snapshot.
2. Verify required `iat`/`exp`, absence of additional temporal claims and zero
   clock skew against a supported Auth build; document its clock requirements.
3. Confirm HS512/UTF-8 and no-`kid`/no-overlap against a supported Auth build;
   provide its revision.
4. Document required Bearer and `X-User-Id` behavior, login `Set-Cookie`, the
   refresh response's access-JWT format, and every refresh-cookie attribute.
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
2. Confirm the same-origin browser deployment for Auth refresh and Auth's CSRF
   handling; cross-origin credentialed CORS is not configured in Gateway.
3. Confirm the frontend uses `/api/v1/requests` and `/api/v1/sports`; the old
   unprefixed paths are no longer Gateway routes.

### Game owner

1. Identify the supported Game build matching the six-operation snapshot and
   confirm its `/api/v1` paths, authentication boundary, and response behavior.
2. Supply a supported container startup contract and a working health URL.

### Platform or team lead

1. Assign ownership of images, Compose dependencies, health checks and startup;
   provide the actual health URLs for all three services.
2. Establish HTTPS for the public browser connection; the TLS termination point
   remains undecided.
3. Prevent direct untrusted access to Core, which does not authenticate the
   `X-User-Id` header.
4. Decide whether upstreams adopt shared request-ID and error conventions.

## Decision log

| Decision | Value | Evidence | Status |
|---|---|---|---|
| Core upstream and public paths | 15 exact method/path pairs in the common table; `/api/v1` paths preserved; unprefixed and unknown routes return 404, wrong methods return 405 | User corrections, 2026-10-01; snapshot still unprefixed | Implemented in source; supported Core build and corrected OpenAPI pending |
| Core authentication | JWT for every routed Core request, including GET | Supplied plan + user confirmation, 2026-09-29 | HTTP middleware wired; real Auth/Redis verification open |
| Identity | Trusted UUID in `X-User-Id` on every Core request; client value removed | User clarification, 2026-09-29 | Implemented in source; Core network boundary open |
| Request ID | New Gateway UUID in `X-Request-Id`, replacing client and Core values | Supplied plan + user confirmation, 2026-09-29 | HTTP middleware wired |
| Core responses | Preserve Core status, body, and ordinary end-to-end headers, including business errors | User clarification, 2026-09-29 | HTTP proxy wired; real Core verification open |
| Gateway failures | HTTP status and request-ID header; no prescribed JSON body | User clarification, 2026-09-29 | Implemented in source |
| Browser Core access | Explicit origin allow-list, Bearer header, preflight without JWT, no credentialed CORS | User clarification, 2026-09-29 | Implemented in source; exact origins pending |
| Redis unavailable/timeout | Continue for otherwise valid JWT if the request reaches Gateway; start without Redis, but `/readyz` returns 503; a denylist hit still rejects | User's corrected clarification, 2026-09-29 | Confirmed |
| Other Redis lookup errors | Fail-closed; target Gateway HTTP status 503 | Previous decision + HTTP clarification, 2026-09-29 | Confirmed |
| Access JWT / Redis denylist | HS512 with UTF-8 secret bytes; `sub`/`jti`/`iat`/`exp`/`type`; zero skew; denylist at logout | User confirmations through 2026-09-29 | Confirmed design; runtime verification open |
| Public boundary | HTTP proxy; remove gRPC entirely | Latest user clarification, 2026-09-29 | Implemented in source; supersedes 2026-09-27 gRPC decision |
| Auth routes and JWT | 16 exact `/api/v1` routes; five public, eleven JWT-protected; HS512, UTF-8, required `sub`/`jti`/`iat`/`exp`/`type`, zero skew | Auth snapshot + agreed proxy plan | Implemented against stubs; supported Auth build and corrected snapshot pending |
| Refresh/browser | Gateway passes Auth cookies, uses same-origin browser flow, and does not enable credentialed CORS; cookie attributes and CSRF belong to Auth | Agreed proxy plan + current source | Stub-tested; real browser/Auth flow unverified |
| Game routes | Eight exact GET paths under `/api/v1` for six upstream operations, all JWT-protected; trusted `X-User-Id` forwarded | Game snapshot + agreed proxy plan | Implemented against stubs; supported Game build unverified |
| Readiness | Redis and HTTP 200 from configured Auth, Core, and Game health URLs within one timeout | Agreed proxy plan + current source | Component-tested; actual URLs pending |
| Compose | Supported images, startup, and service dependencies required | Roadmap | Open |

## Next step

Verify [the HTTP proxy](PROXY.md) against running Auth/Redis, Core, and Game,
then record every deviation from their snapshots. The current verifier accepts
HS512 with UTF-8 key bytes and zero clock skew. It continues only for
classified Redis connection failures and lookup timeouts after successful JWT
validation; other lookup errors remain fail-closed. Gateway startup does not
require a successful Redis PING, while `/readyz` returns 503 until Redis and
all configured health URLs respond successfully.

Compatibility with running Auth/Redis, Core, and Game deployments:
**I cannot verify this** from the repository and stub tests. Supported builds,
health URLs, exact frontend origins, public HTTPS, and upstream network isolation
remain release conditions.
