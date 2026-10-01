package docs

import "embed"

//go:embed contracts/gateway-openapi.yaml
var GatewayOpenAPI []byte

//go:embed swagger-ui
var SwaggerUI embed.FS
