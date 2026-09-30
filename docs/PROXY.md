# HTTP proxy boundary

Status: Core, Auth, and Game HTTP proxy behavior is implemented in the current
source tree and covered by component tests. Real-service integration remains
unverified. The Gateway exposes one HTTP listener and owns `/healthz` and
`/readyz`. See [integration status](INTEGRATION.md).

The frontend calls the Gateway over HTTP; the Gateway calls the upstream services
over HTTP. The [Core](contracts/core-service-openapi.yaml),
[Auth](contracts/auth-service-openapi.yaml), and
[Game](contracts/game-service-openapi.yaml) OpenAPI snapshots describe service
operations and payloads. This document defines Gateway behavior at their boundary.

## Routing and HTTP behavior

- Route `/api/v1/requests`, descendants of `/api/v1/requests/`, and
  `/api/v1/sports` to Core with the same path. The unprefixed routes are not
  aliases. Do not match lookalikes such as `/api/v1/requests-extra`.
  Forward any method in these routes; Core determines whether it is supported.
  The checked-in Core OpenAPI still describes unprefixed paths, so acceptance of
  `/api/v1` by a running Core build must be verified and its contract corrected.
- Auth and Game expose only the method/path pairs below. Their public and upstream
  paths both include `/api/v1`. `{...}` matches one nonempty path segment, not an
  encoded slash. `GET /health` in the Auth snapshot is not a public Gateway route.

| Service | Method | Path | Access JWT |
|---|---|---|---|
| Auth | PUT | `/api/v1/user/update-user/{userId}` | Required; `{userId}` must match `sub` |
| Auth | PUT | `/api/v1/user/change-password/{userId}` | Required; `{userId}` must match `sub` |
| Auth | PUT | `/api/v1/user/change-email/{userId}` | Required; `{userId}` must match `sub` |
| Auth | POST | `/api/v1/verification/verify-password-reset` | Not required |
| Auth | POST | `/api/v1/verification/verify-email/{userId}` | Required; `{userId}` must match `sub` |
| Auth | POST | `/api/v1/verification/send-password-reset-code` | Not required |
| Auth | POST | `/api/v1/verification/send-email-verification-code/{userId}` | Required; `{userId}` must match `sub` |
| Auth | POST | `/api/v1/user/register` | Not required |
| Auth | POST | `/api/v1/auth/refresh` | Not required |
| Auth | POST | `/api/v1/auth/login` | Not required |
| Auth | GET | `/api/v1/session/get-all/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/user/delete/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/session/terminate/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/session/terminate-all/{userId}` | Required; `{userId}` must match `sub` |
| Auth | DELETE | `/api/v1/auth/logout` | Required |
| Game | GET | `/api/v1/results/by-request/{activityRequestId}` | Required |
| Game | GET | `/api/v1/rank-tiers` | Required |
| Game | GET | `/api/v1/players/{userId}` | Required |
| Game | GET | `/api/v1/players/{userId}/sports/{sportId}` | Required |
| Game | GET | `/api/v1/players/{userId}/matches` | Required |
| Game | GET | `/api/v1/leaderboards/{sportId}` | Required |

- Auth/Game unknown paths return 404; a known path with another method returns
  405 with `Allow`. No implicit `HEAD` route is added for their GET operations.
  Core keeps its existing subtree routing and method policy.
- Preserve method, escaped path, raw query, and body. Do not decode or
  reserialize upstream DTOs or repeat business validation in the Gateway.
- Return upstream status, body, and ordinary end-to-end response headers, including
  business-error bodies and unexpected success statuses. Standard hop-by-hop
  headers are excluded. The Gateway controls `X-Request-Id` and CORS response
  headers and does not let upstreams replace them.
- Do not follow upstream redirects or automatically retry forwarded requests.
  No additional Gateway request-body limit is part of this migration.
- Keep `/healthz` and `/readyz` as Gateway-owned HTTP endpoints. There is no
  generic fallback upstream.

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
  Handle CORS preflight on known routes before authentication; Auth/Game
  preflight accepts only the declared method. Allow the `Authorization` header
  but do not enable credentialed CORS. The planned refresh-cookie browser flow
  therefore requires the frontend and Gateway to share an origin. Exact
  deployment origins are still needed.
- Gateway-generated failures have an HTTP status and `X-Request-Id`, but no
  prescribed response-body schema: 401 for missing/invalid access JWT, 503 for
  a fail-closed Redis lookup, 403 for Auth own-user mismatch, 404 for an
  unknown route, 405 with `Allow` for a known Auth/Game path with the wrong
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
