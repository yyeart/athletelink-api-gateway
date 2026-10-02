# AthleteLink Gateway

The Gateway is the public entry point to AthleteLink services. It carries
trusted caller identity and request correlation across service boundaries.

## Language

**API Gateway**:

The public entry point that authenticates callers and forwards requests to
internal services. It owns neither business data nor business authorization.

**Upstream service**:

An internal service reached through the Gateway, such as Core Service or Auth
Service. Game Service is a planned upstream service.

**Auth Service**:

The service that issues access and refresh tokens and owns their lifecycle.

**Core Service**:

The service that owns sports and game-request operations.

**Game Service**:

A planned upstream service whose responsibilities are still being defined.

**Trusted identity**:

The user identity derived from a verified access token and conveyed by the
Gateway to an upstream service. A client-supplied identity is not trusted.

**Business authorization**:

Permission checks involving domain objects, such as whether a user may cancel a
request, belong to the upstream service that owns those objects.

**Request ID**:

A unique identifier assigned to an inbound request and propagated to upstream
services for log correlation.
