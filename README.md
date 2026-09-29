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
- [HTTP proxy behavior and implementation status](docs/PROXY.md)

The proxy does not define a second Core API schema. The Core OpenAPI
snapshot describes Core operations; the proxy document describes routing,
authentication, header handling, and browser access at the Gateway.
