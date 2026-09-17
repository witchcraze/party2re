package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeOptionalJSON_Direct(t *testing.T) {
	tests := []struct {
		name        string
		body        io.Reader
		contentType string
		wantOK      bool
		wantStatus  int
		wantPayload testPayload
	}{
		{
			name:        "nil body is accepted as absent",
			body:        nil,
			wantOK:      true,
			wantPayload: testPayload{},
		},
		{
			name:        "http.NoBody is accepted as absent",
			body:        http.NoBody,
			wantOK:      true,
			wantPayload: testPayload{},
		},
		{
			name:        "empty reader is accepted as absent",
			body:        strings.NewReader(""),
			wantOK:      true,
			wantPayload: testPayload{},
		},
		{
			name:        "valid json document with application/json succeeds",
			body:        strings.NewReader(`{"name":"hero","count":42}`),
			contentType: "application/json",
			wantOK:      true,
			wantPayload: testPayload{Name: "hero", Count: 42},
		},
		{
			name:        "valid json with surrounding whitespace succeeds",
			body:        strings.NewReader("  \n  {\"name\":\"hero\",\"count\":42}  \n  "),
			contentType: "application/json",
			wantOK:      true,
			wantPayload: testPayload{Name: "hero", Count: 42},
		},
		{
			name:        "malformed json returns 400",
			body:        strings.NewReader(`{"name":`),
			contentType: "application/json",
			wantOK:      false,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unknown field returns 400",
			body:        strings.NewReader(`{"name":"hero","extra":1}`),
			contentType: "application/json",
			wantOK:      false,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unsupported content type returns 415",
			body:        strings.NewReader(`{"name":"hero"}`),
			contentType: "text/plain",
			wantOK:      false,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "missing content type on non-empty body returns 415",
			body:        strings.NewReader(`{"name":"hero"}`),
			contentType: "",
			wantOK:      false,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "multiple json documents returns 400",
			body:        strings.NewReader(`{"name":"hero"}{"name":"villain"}`),
			contentType: "application/json",
			wantOK:      false,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "trailing garbage returns 400",
			body:        strings.NewReader(`{"name":"hero"} unexpected trailing content`),
			contentType: "application/json",
			wantOK:      false,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized body (> 64 KiB) returns 400",
			body:        strings.NewReader(`{"name":"` + strings.Repeat("x", 65*1024) + `"}`),
			contentType: "application/json",
			wantOK:      false,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				req = httptest.NewRequest(http.MethodPost, "/test", tc.body)
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
			} else {
				req = httptest.NewRequest(http.MethodPost, "/test", nil)
			}

			rec := httptest.NewRecorder()
			var got testPayload
			ok := decodeOptionalJSON(rec, req, &got)

			if ok != tc.wantOK {
				t.Fatalf("decodeOptionalJSON returned ok=%v, want %v", ok, tc.wantOK)
			}

			if !tc.wantOK {
				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
				}
			} else {
				if got != tc.wantPayload {
					t.Errorf("got payload %+v, want %+v", got, tc.wantPayload)
				}
			}
		})
	}
}

func TestDecodeOptionalJSON_ChunkedEmptyBody(t *testing.T) {
	// Simulate unknown Content-Length (-1) with empty body reader
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(nil))
	req.ContentLength = -1
	rec := httptest.NewRecorder()

	var got testPayload
	ok := decodeOptionalJSON(rec, req, &got)
	if !ok {
		t.Fatalf("expected chunked empty body to be accepted as optional, got ok=false (code=%d)", rec.Code)
	}
}
