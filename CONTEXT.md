# Domain glossary

## API Gateway

The public HTTP entry point for AthleteLink. It authenticates requests, derives
trusted request metadata, and proxies traffic to internal services. It does not
own business data or business authorization rules.

## Upstream service

An internal service reached through the Gateway. The MVP integrations are Auth
Service and Core Service. Game Service is planned but has no usable contract yet.

## Auth Service

The service that issues access and refresh tokens and owns their lifecycle.

## Core Service

The service that owns sports and game-request operations.

## Game Service

A future upstream service. Its responsibilities and HTTP contract are unresolved.

## Identity propagation

The Gateway extracts a user UUID from a verified access token and sends it to an
upstream service as trusted request metadata. The exact header contract remains
to be agreed with the service developers.

## Business authorization

Permission checks involving domain objects, such as whether a user may cancel a
request, belong to the upstream service that owns those objects.

## Request ID

A unique identifier assigned to an inbound request and propagated to upstream
services for log correlation. Its header name and log format remain unresolved.
