package http

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openapiJSON []byte

// OpenAPISpec returns the embedded OpenAPI 3.1 JSON specification bytes.
func OpenAPISpec() []byte {
	return openapiJSON
}

func (h *Handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiJSON)
}
