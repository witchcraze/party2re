package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/validation"
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

// decodeActionParams uses the same strict typed decoding as request bodies.
// Checking raw required values prevents JSON null from silently becoming a Go
// zero value. Actor identity is supplied separately from the authenticated path.
func decodeActionParams(raw json.RawMessage, required []string, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("params must be an object")
	}
	for name, value := range fields {
		if strings.EqualFold(name, "character_id") || strings.EqualFold(name, "player_id") {
			return errors.New("actor overrides are forbidden")
		}
		// No command schema declares a nullable input.
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("null action parameter")
		}
	}
	for _, name := range required {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("missing required action parameter")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// checkTextParams applies the field-agnostic text check to every string in the
// params object, nested values included, by its top-level parameter's declared
// kind (TextLine when undeclared). Malformed params are left to decodeActionParams.
func checkTextParams(raw json.RawMessage, kinds map[string]validation.TextKind) error {
	if !utf8.Valid(raw) {
		return errors.New("params are not valid UTF-8")
	}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	for name, value := range fields {
		kind, ok := kinds[name]
		if !ok {
			kind = validation.TextLine
		}
		if err := checkTextValue(kind, value); err != nil {
			return err
		}
	}
	return nil
}

func checkTextValue(kind validation.TextKind, value any) error {
	switch v := value.(type) {
	case string:
		return validation.CheckText(kind, v)
	case []any:
		for _, item := range v {
			if err := checkTextValue(kind, item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range v {
			if err := checkTextValue(kind, item); err != nil {
				return err
			}
		}
	}
	return nil
}
