package http

import (
	"fmt"
	"net/http"
)

// SuccessResponse defines the standardized envelope for successful API operations.
// It cleanly decouples machine-readable domain data from optional human-facing presentation messages.
type SuccessResponse[T any] struct {
	Data    T      `json:"data"`
	Message string `json:"message,omitempty"`
}

// writeSuccess encodes a SuccessResponse into JSON and writes it to the response writer.
func writeSuccess[T any](w http.ResponseWriter, status int, data T, message string) {
	writeJSON(w, status, SuccessResponse[T]{
		Data:    data,
		Message: message,
	})
}

// ErrorDetail contains machine-readable error codes for client/agent branching
// alongside user-facing explanation messages.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StructuredErrorResponse defines the standardized envelope for API errors.
type StructuredErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// writeAppError encodes a StructuredErrorResponse into JSON and writes it to the response writer.
func writeAppError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, StructuredErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// writeAppErrorf formats a message and writes a StructuredErrorResponse.
func writeAppErrorf(w http.ResponseWriter, status int, code string, format string, args ...any) {
	writeAppError(w, status, code, fmt.Sprintf(format, args...))
}
