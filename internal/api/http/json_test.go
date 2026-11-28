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

func TestCleanTextParams(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		want     string
		wantFail bool
	}{
		{raw: ``, want: ``},
		{raw: `[]`, want: `[]`},
		{raw: `{"name":"a,;<>","body":"a\r\nb","count":1}`, want: `{"name":"a,;<>","body":"ab","count":1}`},
		{raw: `{"name":"e\u0300\u0301\u036f","thai":"ก\u0E34\u0E3A\u0E47\u0E4Eข"}`, want: `{"name":"e","thai":"กข"}`},
		{raw: `{"name":["ok\n","か\u3099"]}`, want: "{\"name\":[\"ok\",\"か\u3099\"]}"},
		{raw: `{"bio":{"hobby":"a\r\nb\u0301"}}`, want: `{"bio":{"hobby":"ab"}}`},
		{raw: `{"name":"a\tb"}`, wantFail: true},
		{raw: `{"body":"a\u0007b"}`, wantFail: true},
		{raw: `{"body":"a\u202eb"}`, wantFail: true},
		{raw: `{"name":["ok","a\u200bb"]}`, wantFail: true},
		{raw: `{"name":[{"id":"a\u0085b"}]}`, wantFail: true},
		{raw: `{"name":"Z\u20D0\u20D1\u20D2"}`, wantFail: true},
		{raw: "{\"name\":\"\xff\"}", wantFail: true},
		{raw: `{"name":"a\u202eb","name":null}`, wantFail: true},
		{raw: `{"name":"ok","NAME":"ok"}`, wantFail: true},
		{raw: `{"name":[{"id":"ok","id":null}]}`, wantFail: true},
		{raw: `{"name":[{"id":"ok","ID":"ok"}]}`, wantFail: true},
		{raw: `{"name":`, wantFail: true},
		{raw: `{} {}`, wantFail: true},
	} {
		got, err := cleanTextParams([]byte(tc.raw))
		if (err != nil) != tc.wantFail {
			t.Errorf("cleanTextParams(%q) err = %v; wantFail=%v", tc.raw, err, tc.wantFail)
			continue
		}
		if !tc.wantFail && string(got) != tc.want {
			t.Errorf("cleanTextParams(%q) = %s; want %s", tc.raw, got, tc.want)
		}
	}
}
