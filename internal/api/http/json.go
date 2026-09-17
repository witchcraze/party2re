package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxRequestBodyBytes is the maximum accepted request body size (64 KiB).
// No legitimate game API call requires more than this.
const maxRequestBodyBytes = 64 * 1024

// decodeJSON decodes a required JSON request body into dst.
// It enforces Content-Type: application/json, limits body size to 64 KiB,
// disallows unknown fields, and ensures exactly one JSON document is present.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("Content-Type must be application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return false
	}
	return true
}

// decodeOptionalJSON decodes an optional JSON request body into dst.
// If no body was provided (nil, http.NoBody, or 0 content length), it returns true without modifying dst.
// If a body is provided, it enforces Content-Type: application/json, maximum body size (64 KiB),
// disallows unknown fields, and verifies that the body contains exactly one valid JSON document.
// On decoding failure, it writes an appropriate error response (415 or 400) and returns false.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return true
	}

	if r.ContentLength < 0 {
		var buf [1]byte
		n, err := r.Body.Read(buf[:])
		if err == io.EOF && n == 0 {
			return true
		}
		if err != nil && n == 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return false
		}
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf[:n]), r.Body))
	}

	return decodeJSON(w, r, dst)
}
