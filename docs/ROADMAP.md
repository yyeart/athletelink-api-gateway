# API Gateway roadmap

## Objective

Deliver an integration MVP of a Go API Gateway in two acceptance milestones:

1. Gateway behavior verified against contract stubs and every available service.
2. End-to-end verification against real Auth, Core, and Game services.

There are no calendar estimates yet. Work advances only when each phase's exit
criteria are met.

## MVP boundaries

Included:

- HTTP routing and reverse proxying for available upstream services;
- access-token authentication at the Gateway;
- propagation of trusted user identity and a request ID;
- CORS for agreed frontend origins;
- structured request logs, timeouts, health/readiness endpoints, and graceful shutdown;
- multi-stage container image and a later Docker Compose integration environment.

Deferred:

- WebSocket chat;
- notifications routing;
- a real Game Service integration until its contract exists;
- business authorization, which remains in the owning upstream service;
- metrics, distributed tracing, and rate limiting unless they become explicit MVP requirements.

## Phase 0 — settle contracts that affect implementation

Record answers in `docs/INTEGRATION.md`. Version every OpenAPI document used for implementation.

Before Core proxy behavior is stable, confirm the public and upstream path mapping,
trusted user identity transport, request-ID header, and error-envelope expectations.

Before JWT middleware implementation, confirm the UUID claim, signing algorithm,
secret encoding, claim validation, exact public method/path pairs, refresh-cookie
contract, token lifecycle, rotation behavior, and test fixtures.

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

## Phase 2 — routing and Core reverse proxy

- Register explicit method/path families; return `404` for unknown routes and
  `503` for an intentionally unavailable configured service.
- Build one `httputil.ReverseProxy` per upstream with explicit URL and path rewrite,
  preserved query/body data, and a controlled transport-error response.
- Remove inbound trusted headers before adding Gateway-derived values.
- Forward standard proxy metadata under a documented trusted-proxy policy.
- Cover every current Core operation with table-driven routing tests against an
  `httptest` upstream.

Exit criterion: every current Core operation reaches the stub with the agreed
method, path, query, body, and headers; invalid routes fail as specified.

## Phase 3 — authentication and browser access

- Parse Bearer access tokens using an explicit algorithm allow-list. Return `401`
  for missing, malformed, expired, or invalidly signed tokens.
- Match public endpoints by exact method/path rules rather than broad wildcards.
- Validate the UUID claim and overwrite the agreed identity header.
- Configure CORS from explicit frontend origins. Treat preflight separately and
  enable credentials only if the refresh-cookie contract requires them.
- Leave domain permission checks in the service that owns the domain object.

Exit criterion: tests cover token failures, public routes, forged identity headers,
preflight requests, and allowed and denied origins.

## Phase 4 — request correlation and resilience

- Generate or validate a request ID according to the agreed trust policy and
  propagate the selected header.
- Emit structured logs with request ID, method, normalized route, status, duration,
  and upstream. Exclude tokens, cookies, and bodies.
- Bound upstream connection and response waits and preserve client cancellation.
- Return stable Gateway-owned errors for authentication, routing, configuration,
  and upstream transport failure.

Exit criterion: tests distinguish Gateway failures from upstream responses, logs
correlate calls across the stub boundary, and captured logs contain no secrets.

## Phase 5 — first acceptance milestone

- Maintain contract stubs from the versioned Core OpenAPI document.
- Run component tests for routing, authentication, headers, CORS, errors, timeouts,
  health, and shutdown.
- Run available Auth and Core services through their supported startup procedures.
- Record and resolve every deviation between OpenAPI and observed behavior.

Exit criterion: the stub suite and verified scenarios with every currently
available real service pass. This is not full MVP completion.

## Phase 6 — Compose and complete integration MVP

- Add Auth, Core, Game, Gateway, and required infrastructure to Docker Compose
  after teams publish supported images and health checks.
- Replace the Game placeholder using its versioned contract.
- Run end-to-end Auth, Core, refresh, upstream-outage, and Game scenarios.
- Publish a runbook covering configuration, startup, readiness, tests, and known limitations.

Exit criterion: all three real services pass agreed end-to-end scenarios in a
repeatable Compose environment.

## Recommended implementation order now

1. Settle only the Core path mapping and identity questions needed for proxying.
2. Build Phase 1 independently of unfinished services.
3. Implement Core routing from the supplied OpenAPI snapshot against an HTTP stub.
4. Add request correlation, logging, CORS, and proxy failure handling.
5. Implement JWT only after Auth supplies the validation contract.
6. Integrate real Auth and Core; keep Game and chat outside the first milestone.
