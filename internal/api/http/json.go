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

// cleanTextParams strips legacy decode_pt characters (CR/LF and combining-mark
// runs) from every string value token in params (nested values included), checks
// the cleaned strings for remaining unsafe characters, and rejects invalid UTF-8,
// case-insensitive duplicate keys, or trailing data. It returns the cleaned JSON.
func cleanTextParams(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	if !utf8.Valid(raw) {
		return nil, errors.New("params are not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var buf bytes.Buffer
	changed, err := cleanTextValue(dec, &buf)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data in params")
	}
	if !changed {
		return raw, nil
	}
	return buf.Bytes(), nil
}

// cleanTextValue reads one JSON value, normalizing and checking string values.
func cleanTextValue(dec *json.Decoder, buf *bytes.Buffer) (bool, error) {
	tok, err := dec.Token()
	if err != nil {
		return false, err
	}
	switch t := tok.(type) {
	case string:
		cleaned := validation.StripLegacyText(t)
		if err := validation.CheckText(cleaned); err != nil {
			return false, err
		}
		writeJSONString(buf, cleaned)
		return cleaned != t, nil
	case json.Number:
		buf.WriteString(t.String())
		return false, nil
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return false, nil
	case nil:
		buf.WriteString("null")
		return false, nil
	case json.Delim:
		if t != '{' && t != '[' {
			return false, errors.New("unexpected JSON delimiter")
		}
		buf.WriteByte(byte(t))
		seen := map[string]bool{}
		changed := false
		first := true
		for dec.More() {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			if t == '{' {
				key, err := dec.Token()
				if err != nil {
					return false, err
				}
				name, ok := key.(string)
				if !ok {
					return false, errors.New("expected JSON string key")
				}
				// Typed decoding matches field names case-insensitively, keeping the last.
				folded := strings.ToLower(name)
				if seen[folded] {
					return false, errors.New("duplicate action parameter")
				}
				seen[folded] = true
				writeJSONString(buf, name)
				buf.WriteByte(':')
			}
			elemChanged, err := cleanTextValue(dec, buf)
			if err != nil {
				return false, err
			}
			if elemChanged {
				changed = true
			}
		}
		end, err := dec.Token()
		if err != nil {
			return false, err
		}
		endDelim, ok := end.(json.Delim)
		if !ok {
			return false, errors.New("expected closing JSON delimiter")
		}
		buf.WriteByte(byte(endDelim))
		return changed, nil
	default:
		return false, errors.New("unexpected JSON token")
	}
}

func writeJSONString(buf *bytes.Buffer, s string) {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	// json.Encoder.Encode appends a trailing newline.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		buf.Truncate(len(b) - 1)
	}
}
