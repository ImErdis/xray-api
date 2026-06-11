// Package api embeds the OpenAPI specification so it can be served at runtime.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte
