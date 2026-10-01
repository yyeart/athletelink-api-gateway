# HTTP proxy boundary

Status: Core, Auth, and Game HTTP proxy behavior is implemented in the current
source tree and covered by component tests. Real-service integration remains
unverified. The Gateway exposes one HTTP listener and owns `/healthz`,
`/readyz`, `/openapi.yaml`, and `/swagger/`. See [integration status](INTEGRATION.md).

The frontend calls the Gateway over HTTP; the Gateway calls the upstream services
over HTTP. The [Core](contracts/core-service-openapi.yaml),
[Auth](contracts/auth-service-openapi.yaml), and
[Game](contracts/game-service-openapi.yaml) OpenAPI snapshots describe service
operations and payloads. This document defines Gateway behavior at their boundary.

## Routing and HTTP behavior

- Core, Auth and Game expose only the method/path pairs in
  `internal/httpapi/routes.go`, listed below. Core no longer forwards arbitrary
  descendants or methods under `/api/v1/requests/`; new operations require a
  route-table and snapshot update. Unprefixed routes are not aliases.
- Public and upstream paths include `/api/v1`. `{...}` matches one nonempty
  path segment, not an encoded slash or a dot segment. Auth current-user aliases
  append the verified JWT subject when forwarding. `GET /health` in the Auth
  snapshot is not a public Gateway route. Core's snapshot still uses unprefixed
  paths, so acceptance of `/api/v1` by a running Core build remains unverified.

| Service | Method | Path | Access JWT |
|---|---|---|---|
| Core | GET | `/api/v1/requests/{id}` | Required |
| Core | PUT | `/api/v1/requests/{id}` | Required |
| Core | GET | `/api/v1/requests` | Required |
| Core | POST | `/api/v1/requests` | Required |
| Core | POST | `/api/v1/requests/{requestId}/rounds/{roundNumber}/result` | Required |
| Core | POST | `/api/v1/requests/{requestId}/complete` | Required |
| Core | POST | `/api/v1/requests/{id}/start` | Required |
| Core | POST | `/api/v1/requests/{id}/registration/open` | Required |
| Core | POST | `/api/v1/requests/{id}/registration/close` | Required |
| Core | POST | `/api/v1/requests/{id}/leave` | Required |
| Core | POST | `/api/v1/requests/{id}/kick/{targetUserId}` | Required |
| Core | POST | `/api/v1/requests/{id}/join` | Required |
| Core | POST | `/api/v1/requests/{id}/cancel` | Required |
| Core | GET | `/api/v1/sports` | Required |
| Core | GET | `/api/v1/requests/{requestId}/rounds` | Required |
| Auth | PUT | `/api/v1/user/update-user` | Required; upstream userId is the verified `sub` |
| Auth | PUT | `/api/v1/user/change-password` | Required; upstream userId is the verified `sub` |
| Auth | PUT | `/api/v1/user/change-email` | Required; upstream userId is the verified `sub` |
| Auth | POST | `/api/v1/verification/verify-password-reset` | Not required |
| Auth | POST | `/api/v1/verification/verify-email` | Required; upstream userId is the verified `sub` |
| Auth | POST | `/api/v1/verification/send-password-reset-code` | Not required |
| Auth | POST | `/api/v1/verification/send-email-verification-code` | Required; upstream userId is the verified `sub` |
| Auth | POST | `/api/v1/user/register` | Not required |
| Auth | GET | `/api/v1/user/me/{userId}` | Required; `{userId}` must match `sub` |
| Auth | POST | `/api/v1/auth/refresh` | Not required |
| Auth | POST | `/api/v1/auth/login` | Not required |
| Auth | GET | `/api/v1/session/get-all/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/user/delete` | Required; upstream userId is the verified `sub` |
| Auth | DELETE | `/api/v1/session/terminate/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/session/terminate-all/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/auth/logout` | Required |
| Game | GET | `/api/v1/results/by-request/{activityRequestId}` | Required |
| Game | GET | `/api/v1/rank-tiers` | Required |
| Game | GET | `/api/v1/players/{userId}` | Required |
| Game | GET | `/api/v1/players` | Required; upstream userId is the verified `sub` |
| Game | GET | `/api/v1/players/` | Required; same behavior, without a redirect |
| Game | GET | `/api/v1/players/{userId}/sports/{sportId}` | Required |
| Game | GET | `/api/v1/players/{userId}/matches` | Required |
| Game | GET | `/api/v1/leaderboards/{sportId}` | Required |

- Unknown API paths return 404; a known path with another method returns
  405 with `Allow` listing all declared methods for that path. No implicit
  `HEAD` or ordinary `OPTIONS` operation is added. CORS preflight remains available.
- Preserve method, escaped path, raw query, and body on explicit-ID routes.
  Do not decode or reserialize upstream DTOs or repeat business validation.
- `GET /api/v1/players` and `/api/v1/players/` forward to Game as
  `/api/v1/players/{sub}`. Explicit player IDs remain unchanged and may identify
  another user. The six ID-less Auth user/verification routes above forward to
  the same path plus `/{sub}`. Client identity headers and query parameters cannot
  choose that ID; queries and bodies are preserved.
- `GET /api/v1/user/me/{userId}` is the explicit-ID exception: the ID must match
  the verified JWT subject or Gateway returns 403 without calling Auth.
  `/api/v1/user/me` and old ID-bearing forms of the six aliases return 404.
- The source-reviewed Auth `me` operation currently returns an empty
  `UserControllerDto`; its mapper does not transfer the existing service fields.
  Completing Auth's response is owned by the user. Gateway passes the upstream
  response through; it does not synthesize username, email, or verification data.
- Return upstream status, body, and ordinary end-to-end response headers, including
  business-error bodies and unexpected success statuses. Standard hop-by-hop
  headers are excluded. The Gateway controls `X-Request-Id` and CORS response
  headers and does not let upstreams replace them.
- Do not follow upstream redirects or automatically retry forwarded requests.
  No additional Gateway request-body limit is part of this migration.
- Keep `/healthz` and `/readyz` as Gateway-owned HTTP endpoints. The public
  `/openapi.yaml` and `/swagger/` routes serve the draft contract and bundled
  Swagger UI without calling an upstream. There is no generic fallback upstream.

## Authentication and trusted headers

- Require one `Authorization: Bearer <access JWT>` for every Core and Game
  request and the protected Auth operations listed above. Check the established
  Auth signing, claim, expiry, and Redis denylist rules before forwarding.
  Public Auth operations, health, readiness, and CORS preflight do not require
  a JWT. A protected Auth `{userId}` that differs from the verified `sub`
  returns 403 before calling Auth; Game player paths retain the requested
  `{userId}` independently of the caller's identity.
- Generate a fresh UUID request ID at the Gateway, replacing a client value.
  Send it to the selected upstream and the client as `X-Request-Id`, even on
  Gateway-generated errors. Do not allow an upstream response to replace it.
- Remove client `Authorization`, `Cookie`, `X-User-Id`, `X-Request-Id`, and
  untrusted forwarding headers before the Core call. Preserve other ordinary
  end-to-end request headers. Send the verified user UUID as `X-User-Id` on
  every proxied Core request and the new `X-Request-Id` once.
- Auth retains client `Cookie` and forwards upstream `Set-Cookie` without creating
  a Gateway cookie. Public Auth operations discard client `Authorization`;
  protected Auth operations forward only the verified Bearer token and trusted
  `X-User-Id`. Game receives trusted `X-User-Id`, but no client Bearer token,
  cookie, or identity/forwarding headers.
- Keep the agreed Redis behavior: a confirmed denylist hit rejects the token;
  connection failure or lookup timeout permits an otherwise valid JWT to
  continue; command/configuration and unclassified lookup errors reject the
  request without calling Core. The process may start without Redis, while
  `/readyz` remains unavailable until Redis responds.

## Browser and Gateway failures

- Configure an explicit allow-list of frontend origins for all three services.
  Handle CORS preflight on known routes before authentication; API
  preflight accepts only a declared method. Allow the `Authorization` header
  but do not enable credentialed CORS. The planned refresh-cookie browser flow
  therefore requires the frontend and Gateway to share an origin. Exact
  deployment origins are still needed.
- Gateway-generated failures have an HTTP status and `X-Request-Id`, but no
  prescribed response-body schema: 401 for missing/invalid access JWT, 503 for
  a fail-closed Redis lookup, 403 for Auth own-user mismatch, 404 for an
  unknown route, 405 with `Allow` for a known API path with the wrong
  method, 502 for an upstream connection failure, and 504 for an upstream
  timeout. Upstream responses pass through without reinterpretation.

## Readiness

- `GATEWAY_CORE_URL`, `GATEWAY_AUTH_URL`, and `GATEWAY_GAME_URL` are required
  HTTP(S) origins. Each upstream has its own timeout, shorter than the Gateway
  write timeout.
- `GATEWAY_CORE_HEALTH_URL`, `GATEWAY_AUTH_HEALTH_URL`, and
  `GATEWAY_GAME_HEALTH_URL` are required full HTTP(S) URLs. `/readyz` returns
  200 only while the listener is ready, Redis PING succeeds, and each health
  URL returns HTTP 200. A redirect or any other status yields 503. All checks
  share one configurable timeout (`GATEWAY_HEALTH_TIMEOUT`, default `2s`).
- Startup does not wait for Redis or upstream health. `/healthz` reports the
  Gateway process independently of `/readyz`.

## Request logging

`internal/requestcontext/diagnostics.go` holds one in-memory accumulator per
HTTP request, shared through `context`. Authentication, readiness and proxy code
record their results there; the outer logging middleware takes a snapshot when
the request ends and writes one JSON event. A mutex protects updates and the
snapshot. Diagnostics is not a persistent store or a public endpoint. It holds
only the fields listed below, not credentials or request/response bodies.

- `GATEWAY_LOG_LEVEL` controls the existing JSON slog handler; default is `info`.
  API requests produce one `http_request_completed` record with `service`,
  `request_id`, standard HTTP `method`, route template, selected `upstream`,
  `upstream_called`, `upstream_status`, final `status`, `duration_ms`,
  `response_bytes`, `auth_result`, and `denylist_result`. A verified successful
  identity adds `user_id`; failures add a classified `error_kind`, and failed
  readiness checks add `dependency`.
- All declared API paths use their route templates; unknown paths use
  `unmatched`. Logs exclude raw paths,
  queries, tokens, token IDs, cookies, bodies, email, IP addresses and raw
  dependency-error text. Unknown HTTP methods are logged as `OTHER`.
- Normal API responses use INFO, 4xx use WARN, 5xx use ERROR. A Redis outage
  uses WARN even when a valid JWT is allowed through; the record has
  `auth_result=allowed_without_denylist` and `denylist_result=unavailable`.
  Successful health/readiness/documentation/preflight requests use DEBUG.
  Client cancellation uses INFO. `status=0` means no final response status was
  observed for an ended or aborted request; it does not imply success.
- Logout records only `logout_refresh_cookie_present` for the expected
  `jwtRefreshToken` cookie. An Auth HTTP 204 is not proof of a Redis write or
  token revocation. Startup records contain safe upstream origins and Redis
  address/database, never password or signing secret. Normal server lifecycle
  events use INFO.

## Implementation and release checks

The HTTP listener wires Core, Auth, and Game proxies, JWT and request-ID
middleware, CORS handling, and the composite readiness check. Component tests
cover the declared routes and transport behavior. The former gRPC listener,
adapter, protobuf artifacts, and dependencies are absent from the source tree.

Before release, verify behavior with running Auth/Redis, Core, and Game; obtain
their supported build versions and actual health URLs; configure exact frontend
origins; establish public HTTPS; and prevent direct untrusted access to
upstreams that trust `X-User-Id`. Real-service compatibility and deployment
protections: **I cannot verify this** from this repository alone.
