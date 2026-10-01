# AthleteLink API Gateway

Gateway for the AthleteLink microservice backend. The Gateway is an HTTP proxy:
the frontend calls it over HTTP, and it forwards Core requests over HTTP after
checking an access JWT and replacing trusted headers. The proxy is implemented
in the current source tree; integration with running Auth/Redis and Core remains
to be verified. See the integration record for the exact status.

- [Delivery roadmap](docs/ROADMAP.md)
- [Integration status and open decisions](docs/INTEGRATION.md)
- [Domain glossary](CONTEXT.md)
- [Current Core Service OpenAPI snapshot](docs/contracts/core-service-openapi.yaml)
- [Draft public Gateway OpenAPI](docs/contracts/gateway-openapi.yaml)
- [HTTP proxy behavior and implementation status](docs/PROXY.md)

The proxy does not define a second Core API schema. The Core OpenAPI
snapshot describes Core operations; the proxy document describes routing,
authentication, header handling, and browser access at the Gateway.
The draft Gateway OpenAPI is assembled from the three service snapshots and
the Gateway route table with `ruby scripts/build_gateway_openapi.rb`. It is a
client-facing view, not a second source of Core payload schemas. Run the same
command with `--check` to verify that the checked-in file is current.

`GET /api/v1/players` (also `/api/v1/players/`, without a redirect) uses the
verified JWT subject for the Game upstream path. `GET /api/v1/players/{userId}`
still supports another player's profile. Auth's explicit
`GET /api/v1/user/me/{userId}` requires an ID matching the verified subject.
Auth's current `me` controller DTO is empty; completing its fields and mapper
is an Auth task, and Gateway forwards the response unchanged.

The common [route table](internal/httpapi/routes.go) declares Core, Auth and
Game operations. Unknown paths return 404; wrong methods return 405 with `Allow`.
Core no longer accepts arbitrary subtree operations. Protected user/verification
mutations omit userId publicly and forward the verified JWT subject to Auth.
`GET /api/v1/user/me/{userId}` retains its ID and requires it to match the token;
the short `/api/v1/user/me` route is absent. See [the full route list](docs/PROXY.md).

Gateway emits one JSON `http_request_completed` event per API request, including
request ID, route template, verified user ID, upstream/status, duration, byte
count, authentication and denylist outcomes. `GATEWAY_LOG_LEVEL=info` is the
default; use `debug` to include successful health/readiness/Swagger/preflight
requests. Redis fail-open requests emit WARN. Tokens, cookies, bodies, raw
paths and query strings are excluded. See [the logging fields and levels](docs/PROXY.md#request-logging).

When built with the documentation assets, the Gateway serves the draft
specification at `/openapi.yaml` and the bundled Swagger UI at `/swagger/`
(with `/swagger` redirecting there). These
documentation routes are public and use the Gateway origin, so Swagger UI's
"Try it out" requests target that Gateway. The UI assets are vendored from
`swagger-ui-dist` 5.33.1 under `docs/swagger-ui/`, with its license notices.
The displayed contract remains a draft until the real upstream services are
verified against their snapshots.

For a single-file copy that needs no separate CSS, JavaScript, or YAML files,
use [docs/swagger-standalone.html](docs/swagger-standalone.html). It contains
the draft contract with DTO references expanded for local `file://` viewing,
plus Swagger UI assets. Regenerate it with
`ruby scripts/build_standalone_swagger.rb`; pass `--check` to check freshness.
Open it in a browser or publish that one HTML file on a static host. API calls
from "Try it out" are disabled until the page URL includes
`?gateway=https://your-gateway-origin`; the target Gateway must be reachable
from the browser and allow that page's origin if they differ.
