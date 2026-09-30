# API Gateway roadmap

## Objective

Deliver an integration MVP of a Go API Gateway in two acceptance milestones:

1. Gateway behavior verified against contract stubs and every available service.
2. End-to-end verification against real Auth, Core, and Game services.

There are no calendar estimates yet. Work advances only when each phase's exit
criteria are met.

## MVP boundaries

Included:

- public HTTP proxy to Core using the Core OpenAPI snapshot for service behavior;
- exact Auth and Game HTTP routes from their supplied OpenAPI snapshots and the
  agreed [proxy rules](PROXY.md);
- access-token authentication at the Gateway;
- propagation of trusted user identity and a request ID;
- CORS for agreed frontend origins;
- structured request logs, timeouts, health/readiness endpoints, and graceful shutdown;
- multi-stage container image and a later Docker Compose integration environment.

Deferred:

- WebSocket chat;
- notifications routing;
- deployment integration with a running Game Service until its supported build,
  health endpoint, and startup procedure are provided;
- business authorization, which remains in the owning upstream service;
- metrics, distributed tracing, and rate limiting unless they become explicit MVP requirements.

## Phase 0 — settle contracts that affect implementation

Record answers in `docs/INTEGRATION.md`. Version every OpenAPI document used for implementation.

The 2026-09-29 clarification replaces the earlier public gRPC decision. The
agreed flow is frontend → HTTP Gateway proxy → HTTP Core, with no separate Gateway
Core DTO or error schema. Record routing, trusted-header, JWT, CORS, and
Gateway-owned failure rules in [the proxy boundary](PROXY.md). The checked-in
Core OpenAPI remains the source for Core operations and responses.
The 2026-10-01 Core routing correction adds `/api/v1` to both public and
upstream paths. The checked-in Core snapshot still lacks that prefix; confirm
the supported Core build and correct its OpenAPI before claiming conformance.

Auth and Game snapshots now provide route inventories. Before production use,
verify the recorded JWT/Redis and Auth/Game proxy behavior against supported
running builds, obtain test fixtures and health URLs, and have the Auth owner
correct its OpenAPI `/api/v1`, Bearer, identity-header, and cookie details.

Exit criterion: each required item is confirmed by the responsible service owner
or explicitly marked as a temporary test-only assumption.

## Phase 1 — executable Gateway foundation

- Initialize the Go module and separate configuration, HTTP composition,
  authentication, proxying, and observability.
- Parse and validate listen address, upstream URLs, JWT settings, CORS origins,
  and timeouts at startup. Fail fast on invalid required configuration.
- Add `/healthz`, `/readyz`, graceful shutdown, and bounded server timeouts.
- Provide a multi-stage Dockerfile with a non-root runtime user. Measure the image
  against the existing 20–30 MB target.

Exit criterion: the binary starts from validated configuration, health endpoints
work, shutdown is graceful, and the container runs as a non-root user.

## Phase 2 — HTTP Core proxy

- Route `/api/v1/requests`, descendants of `/api/v1/requests/`, and
  `/api/v1/sports` to the HTTP Core upstream with the same path and no
  per-operation DTO mapping. Do not expose the old unprefixed paths.
- Preserve request method/path/query/body and Core status/body/end-to-end response
  headers. Core owns business validation and its response format.
- Require a verified access JWT for all Core routes. Replace client identity and
  request ID headers before the upstream call.
- Remove the public gRPC listener, protobuf schema/generated code, gRPC adapter,
  obsolete dependencies, and gRPC-only tests and documentation.
- Test the public HTTP path against a Core stub, including authentication,
  forged headers, response passthrough, unavailable upstream, and cancellation.

Exit criterion: the Gateway exposes Core through HTTP only; the stub sees the
original HTTP request with trusted Gateway headers, and the client sees Core's
HTTP response except for Gateway-controlled headers. See [proxy rules](PROXY.md).

## Phase 3 — authentication and browser access

- Move the existing access-token verification from the gRPC interceptor into
  HTTP middleware; retain the established algorithm, claim, and Redis policies.
- Authenticate every Core request, including GET. Keep health, readiness, and
  CORS preflight outside this check.
- Configure exact frontend origins for Core CORS. Allow `Authorization` and
  handle preflight without JWT; do not enable credentials for Core requests.
- Pass Auth refresh cookies and `Set-Cookie` through the Gateway; use the agreed
  same-origin browser flow without credentialed CORS. Auth owns cookie attributes
  and CSRF behavior, which still require runtime confirmation.
- Leave domain permission checks in the service that owns the domain object.

Exit criterion: tests cover token failures, public routes, forged identity headers,
preflight requests, and allowed and denied origins.

## Phase 4 — request correlation and resilience

- Generate a new UUID `X-Request-Id`, replace the client value, and propagate it
  to upstream and client, preserving it even when upstream returns its own value.
- Return the generated request ID in the HTTP response header and forward it
  to Core as `X-Request-Id`.
- Emit structured logs with request ID, HTTP method/path, status, duration,
  and upstream. Exclude tokens, cookies, and bodies.
- Bound upstream connection and response waits and preserve client cancellation.
- Use HTTP statuses for Gateway-owned authentication, routing, and transport
  failures without introducing a Gateway response-body schema.

Exit criterion: tests distinguish Gateway failures from upstream responses, logs
correlate calls across the stub boundary, and captured logs contain no secrets.

## Phase 5 — first acceptance milestone

- Maintain contract stubs from the Core, Auth, and Game OpenAPI snapshots.
- Run component tests for all declared Auth/Game operations, routing,
  authentication, headers, cookies, CORS, errors, timeouts, health, and shutdown.
- Run available Auth, Core, and Game services through their supported startup
  procedures once their builds and health URLs are provided.
- Record and resolve every deviation between OpenAPI and observed behavior.

Exit criterion: the stub suite and verified scenarios with every currently
available real service pass. This is not full MVP completion.

## Phase 6 — Compose and complete integration MVP

- Add Auth, Core, Game, Gateway, and required infrastructure to Docker Compose
  after teams publish supported images and health checks.
- Verify the implemented Game proxy against the supported Game build and its
  versioned contract.
- Run end-to-end Auth, Core, refresh, upstream-outage, and Game scenarios.
- Publish a runbook covering configuration, startup, readiness, tests, and known limitations.

Exit criterion: all three real services pass agreed end-to-end scenarios in a
repeatable Compose environment.

## Recommended next steps

The HTTP Core, Auth, and Game proxies, JWT/request-ID/CORS middleware, gRPC
removal, configuration, container inputs, and HTTP component tests are present
in the source tree. Stub tests do not establish compatibility with running services.

1. Obtain supported Auth, Core, and Game builds, their health URLs, and an
   Auth OpenAPI correction; verify Gateway against them and record deviations.
2. Confirm the same-origin frontend deployment, Auth refresh-cookie/CSRF
   behavior, and exact frontend origins. Keep chat outside the MVP.
3. Establish public HTTPS and protect Core from direct untrusted access before
   release; the deployment mechanisms have not yet been chosen.
