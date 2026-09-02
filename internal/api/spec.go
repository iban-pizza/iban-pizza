package api

import _ "embed"

// openAPISpec is the API description, compiled into the binary so that a
// running instance always serves the description of the code it is running.
//
//go:embed openapi.yaml
var openAPISpec []byte
