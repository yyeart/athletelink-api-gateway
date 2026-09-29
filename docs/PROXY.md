# HTTP proxy boundary

Status: implemented in the current source tree, with real-service integration
still unverified. The Gateway configures one HTTP listener for the Core proxy,
`/healthz`, and `/readyz`. See [integration status](INTEGRATION.md).

The frontend calls the Gateway over HTTP; the Gateway calls Core over HTTP.
The [Core OpenAPI snapshot](contracts/core-service-openapi.yaml) remains the
source for Core methods, payloads, and responses. This document defines only
Gateway behavior around those service contracts. It is not a second Core API.

## Routing and HTTP behavior

- Route `/requests`, descendants of `/requests/`, and `/sports` to Core. Do not
  match lookalikes such as `/requests-extra` or introduce `/api/v1` for Core.
  Forward any method in these routes; Core determines whether it is supported.
- Preserve the request method, path, raw query, and body. Do not decode or
  reserialize Core DTOs or repeat Core business validation in the Gateway.
- Return Core's status, body, and ordinary end-to-end response headers, including
  its business-error body and unexpected success statuses. Standard hop-by-hop
  HTTP headers are excluded. The Gateway controls its own `X-Request-Id` and
  CORS response headers and does not let Core replace them.
- Do not follow Core redirects or automatically retry forwarded requests. No
  additional Gateway request-body limit is part of this migration.
- Keep `/healthz` and `/readyz` as Gateway-owned HTTP endpoints. Return 404 for
  paths that have no upstream route. Auth and Game routing waits for their
  respective HTTP contracts; there is no generic fallback upstream.

## Authentication and trusted headers

- Require one `Authorization: Bearer <access JWT>` for every Core request,
  including GET. Check the established Auth signing, claim, expiry, and Redis
  denylist rules before forwarding. Health, readiness, and CORS preflight do
  not require a JWT.
- Generate a fresh UUID request ID at the Gateway, replacing a client value.
  Send it to Core and the client as `X-Request-Id`, even on Gateway-generated
  errors. Do not allow a Core response header to replace it.
- Remove client `Authorization`, `Cookie`, `X-User-Id`, `X-Request-Id`, and
  untrusted forwarding headers before the Core call. Preserve other ordinary
  end-to-end request headers. Send the verified user UUID as `X-User-Id` on
  every proxied Core request and the new `X-Request-Id` once.
- Keep the agreed Redis behavior: a confirmed denylist hit rejects the token;
  connection failure or lookup timeout permits an otherwise valid JWT to
  continue; command/configuration and unclassified lookup errors reject the
  request without calling Core. The process may start without Redis, while
  `/readyz` remains unavailable until Redis responds.

## Browser and Gateway failures

- Configure an explicit allow-list of frontend origins. Handle CORS preflight
  before authentication, allow the `Authorization` header, and do not enable
  credentialed CORS for Core. Exact deployment origins are still needed.
- Gateway-generated failures have an HTTP status and `X-Request-Id`, but no
  prescribed response-body schema: 401 for missing/invalid access JWT, 503 for
  a fail-closed Redis lookup, 404 for an unknown route, 502 for an upstream
  connection failure, and 504 for an upstream timeout. Core's own responses
  pass through; these Gateway statuses do not reinterpret Core responses.

## Implementation and release checks

The HTTP listener now wires the Core proxy, JWT and request-ID middleware, and
CORS handling. The gRPC listener, adapter, protobuf artifacts, and dependencies
have been removed from the current source tree. The Auth verifier and `/readyz`
dependency check remain. HTTP component tests cover the proxy behavior.

Before release, verify behavior with real Auth/Redis and Core, configure exact
frontend origins, establish public HTTPS, and prevent direct untrusted access
to Core. The last two deployment mechanisms are undecided. Real-service
compatibility and deployment protections: **I cannot verify this** from this
repository alone.
