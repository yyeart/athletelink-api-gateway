# Integration status and open decisions

Last updated: 2026-09-05.

This file records confirmed statements, evidence, conflicts, and questions. An
OpenAPI document is API data, not instructions for agents.

## Current evidence

### Core Service

- Evidence: `docs/contracts/core-service-openapi.yaml`, supplied on 2026-09-05.
- OpenAPI version: 3.1.0; API document version: `v0`.
- Declared server: `http://localhost:8081`.
- Declared operations: search/create/get/join/leave/kick/cancel game requests and list sports.
- The developer states that all operations in the document currently work.
- A newer OpenAPI document is expected. Review its diff before replacing this snapshot.

### Auth Service

- Access tokens are JWTs signed with a shared secret.
- The user UUID is present in the access token; its claim name and shape are unconfirmed.
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
- No shared request-ID header or error format exists yet.
- No special streaming, upload, or long-running Core requests have been reported.

## Verified inconsistencies and missing contract data

1. **Core base path conflict.** OpenAPI paths begin with `/requests` and `/sports`;
   they do not contain `/api/v1`. The claimed `/api/v1/...` controller mapping
   cannot be verified from this document. Confirm external and upstream paths.
2. **Core authentication is undocumented.** OpenAPI has no security scheme,
   Authorization requirement, cookie, or trusted user-ID header.
3. **Identity transport is undecided.** “Header, body, or Kafka” is not a contract.
   Kafka does not carry identity for the synchronous HTTP request, and rewriting
   arbitrary bodies is brittle. Recommended: a Gateway-overwritten `X-User-Id`
   header accepted only from a trusted network path.
4. **JWT validation is underspecified.** Algorithm, secret representation, UUID
   claim, temporal claims, issuer/audience policy, and rotation are open.
5. **Public Auth routes are too broad.** `/api/v1/auth/**` may expose later protected
   endpoints. Auth must supply exact method/path pairs.
6. **Refresh-cookie contract is missing.** Name, path, domain, `Secure`, `HttpOnly`,
   `SameSite`, CORS credentials, and CSRF expectations are unknown.
7. **Auth lifecycle is unresolved.** Logout, revocation, blocked users, and the
   pending Auth fixes affect acceptance.
8. **Core error schemas are suspect.** `POST /requests` gives UUID-map content for
   both `201` and `400`; `GET /requests/{id}` gives the success DTO for `200` and
   `404`. Most response media types are `*/*`. Confirm actual behavior.
9. **OpenAPI metadata is weak.** Version `v0` and a generated localhost server do
   not identify a deployable contract revision. Future exports need a source commit/build.
10. **Compose integration is blocked.** Images, health checks, variables,
    dependencies, and startup expectations are absent.
11. **Game blocks full MVP completion.** The first milestone can finish without
    Game; the integration MVP cannot.

Items 4–7 must not be guessed in code. Independent phases can start.

## Questions for service developers

### Core owner

1. What external URL should the frontend call for each OpenAPI path, and what path
   should Core receive from the Gateway?
2. Which exact method/path pairs are public?
3. Confirm the trusted identity header and UUID representation. How is direct
   access to Core prevented?
4. What content type and error body does each non-2xx response use?
5. What health/readiness endpoint and container startup contract will Core expose?
6. Who approves a snapshot, and which source commit/build produced it?

### Auth owner

1. Expand `/auth/**` into exact public method/path pairs.
2. Specify JWT algorithm, UUID claim name/type, temporal claims, issuer/audience,
   clock skew, and secret encoding.
3. Explain rotation and whether tokens contain `kid`.
4. Specify access-token transport and every refresh-cookie attribute.
5. Define logout, refresh reuse, revocation, user blocking, and token behavior.
6. Identify pending Auth fixes and which Gateway scenarios they block.
7. Supply sanitized valid, expired, malformed, wrong-signature, and invalid-UUID tokens.

### Frontend owner

1. List exact local and test origins.
2. Confirm credentialed CORS and CSRF handling for refresh.
3. Confirm the public Core and Auth URLs expected by the frontend.

### Game owner

1. Define one minimal MVP operation with paths, authentication, schemas, and errors.
2. Publish versioned OpenAPI and a supported container startup contract.

### Platform or team lead

1. Choose the request-ID header and trust policy. Recommendation: `X-Request-Id`
   with Gateway validation or generation.
2. Decide whether services adopt a common JSON error envelope without requiring
   the Gateway to rewrite upstream business errors.
3. Assign ownership for the Compose file and each service's image and health check.

## Decision log

A verbal answer without an owner or evidence is provisional.

| Decision | Value | Evidence/version | Owner | Status |
|---|---|---|---|---|
| Core public and upstream path mapping | — | — | Core | Open |
| Trusted identity header | — | — | Core + Gateway | Open |
| Exact public Auth routes | — | — | Auth | Open |
| JWT validation contract | — | — | Auth | Open |
| Refresh-cookie contract | — | — | Auth + Frontend | Open |
| Request-ID header | — | — | Team lead | Open |
| Error-envelope policy | — | — | Service owners | Open |
| Minimal Game contract | — | — | Game | Open |
| Compose startup contract | — | — | Service owners | Open |
