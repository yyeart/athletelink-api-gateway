# API Gateway roadmap

## Objective

Deliver an integration MVP of a Go API Gateway in two acceptance milestones:

1. Gateway behavior verified against contract stubs and every available service.
2. End-to-end verification against real Auth, Core, and Game services.

There are no calendar estimates yet. Work advances only when each phase's exit
criteria are met.

## MVP boundaries

Included:

- public gRPC Core API with an HTTP adapter based on the Core snapshot;
- other upstream routing only after its public and upstream contracts are settled;
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

Public gRPC is confirmed by the user's clarification. Core upstream remains HTTP
under the supplied snapshot. Define the Gateway protobuf contract, operation and
error mappings, and metadata policy before writing the handlers. Legacy public
HTTP compatibility and browser transport remain open; do not silently add a bridge.

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

## Phase 2 — public gRPC Core API and HTTP adapter

- Define a versioned protobuf service with typed unary methods for every Core
  operation and document the method/path, query/body and response mappings.
- Specify field presence, metadata, success/error mapping and contract generation.
- Implement generated server interfaces and an explicit HTTP Core client adapter;
  do not treat a reverse proxy as a gRPC-to-HTTP mapping implementation.
- Read identity only from trusted internal context; populate required Core headers.
- Keep Core operations disconnected from production until JWT verification is ready.
- Cover every RPC-to-Core mapping through a gRPC test client and HTTP stub.

Exit criterion: the contract and generated code are reproducible, every Core
operation reaches the stub with the agreed data, and gRPC responses/errors match
the documented mapping. Detailed task: `docs/tasks/core-routing.md`.

## Phase 3 — authentication and browser access

- Validate access tokens in a gRPC interceptor using the agreed metadata transport
  and explicit algorithm allow-list. Specify authentication failures in the gRPC
  contract; retain HTTP rules only for separately agreed HTTP endpoints.
- Match public endpoints by exact method/path rules rather than broad wildcards.
- Validate the UUID claim and overwrite the agreed identity header.
- Settle browser transport first; apply CORS only to an agreed HTTP/browser bridge.
  Configure it from explicit frontend origins. Treat preflight separately and
  enable credentials only if the refresh-cookie contract requires them.
- Leave domain permission checks in the service that owns the domain object.

Exit criterion: tests cover token failures, public routes, forged identity headers,
preflight requests, and allowed and denied origins.

## Phase 4 — request correlation and resilience

- Generate a new UUID `X-Request-Id`, replace the client value, and propagate it
  to upstream and client, preserving it even when upstream returns its own value.
- Return the generated request ID in agreed gRPC response metadata and forward
  it to Core as `X-Request-Id`.
- Emit structured logs with request ID, full RPC method, gRPC status, duration,
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

1. Documentation updated: current Core inventory, accepted decisions and confirmed
   public gRPC requirement are in `docs/INTEGRATION.md`.
2. Define the protobuf contract, then implement gRPC handlers and the HTTP Core adapter;
   detailed scope and acceptance are in `docs/tasks/core-routing.md`.
3. Extend Core URL configuration and explicit positive upstream timeouts.
4. Add request ID, structured logs and controlled Gateway transport errors.
5. Prepare the internal identity seam; implement JWT only after Auth supplies its
   contract. Keep production Core routes disabled until verification is ready.
6. Integrate real Auth and Core; keep Game and chat outside the first milestone.

Stub tests do not prove real service integration. Public gRPC supersedes the
original public HTTP routing step; response mapping is explicit contract work.
