# AthleteLink API Gateway

Gateway for the AthleteLink microservice backend. The agreed target is an HTTP
proxy: the frontend calls the Gateway over HTTP, and the Gateway forwards Core
requests to Core over HTTP after checking an access JWT and replacing trusted
headers. The current binary still exposes Core through gRPC; this proxy change
has not been implemented. See the integration record for the exact status.

- [Delivery roadmap](docs/ROADMAP.md)
- [Integration status and open decisions](docs/INTEGRATION.md)
- [Domain glossary](CONTEXT.md)
- [Current Core Service OpenAPI snapshot](docs/contracts/core-service-openapi.yaml)
- [Agreed HTTP proxy behavior and implementation status](docs/PROXY.md)

The target proxy does not define a second Core API schema. The Core OpenAPI
snapshot describes Core operations; the proxy document describes routing,
authentication, header handling, and browser access at the Gateway.
