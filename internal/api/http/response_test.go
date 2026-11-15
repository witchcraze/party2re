package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testPayloadData struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

func TestWriteSuccess(t *testing.T) {
	t.Run("with message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		data := testPayloadData{ID: "char-1", Count: 10}
		msg := "お買い上げありがとうございます！"

		writeSuccess(rec, http.StatusOK, data, msg)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %q", ct)
		}

		var resp SuccessResponse[testPayloadData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data != data {
			t.Errorf("expected data %+v, got %+v", data, resp.Data)
		}
		if resp.Message != msg {
			t.Errorf("expected message %q, got %q", msg, resp.Message)
		}
	})

	t.Run("empty message omits message field in json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		data := testPayloadData{ID: "char-2", Count: 0}

		writeSuccess(rec, http.StatusCreated, data, "")

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
		}

		var raw map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw json: %v", err)
		}

		if _, exists := raw["message"]; exists {
			t.Errorf("expected message field to be omitted when empty, got %v", raw["message"])
		}
		if _, exists := raw["data"]; !exists {
			t.Errorf("expected data field to be present")
		}
	})

	t.Run("supports primitive data types", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeSuccess(rec, http.StatusOK, 42, "computed")

		var resp SuccessResponse[int]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data != 42 {
			t.Errorf("expected data 42, got %d", resp.Data)
		}
		if resp.Message != "computed" {
			t.Errorf("expected message %q, got %q", "computed", resp.Message)
		}
	})
}

func TestWriteAppError(t *testing.T) {
	t.Run("writes structured error with code and message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		code := "INSUFFICIENT_FUNDS"
		msg := "ゴールドが足りません！"

		writeAppError(rec, http.StatusUnprocessableEntity, code, msg)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %q", ct)
		}

		var resp StructuredErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if resp.Error.Code != code {
			t.Errorf("expected code %q, got %q", code, resp.Error.Code)
		}
		if resp.Error.Message != msg {
			t.Errorf("expected message %q, got %q", msg, resp.Error.Message)
		}
	})
}

func TestWriteAppErrorf(t *testing.T) {
	t.Run("formats message string correctly", func(t *testing.T) {
		rec := httptest.NewRecorder()
		code := "INVALID_AMOUNT"

		writeAppErrorf(rec, http.StatusBadRequest, code, "指定された金額 %d は無効です", -100)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}

		var resp StructuredErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		expectedMsg := "指定された金額 -100 は無効です"
		if resp.Error.Code != code {
			t.Errorf("expected code %q, got %q", code, resp.Error.Code)
		}
		if resp.Error.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, resp.Error.Message)
		}
	})
}

func TestWriteError(t *testing.T) {
	t.Run("4xx preserves original error message and does not log", func(t *testing.T) {
		for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			rec := httptest.NewRecorder()
			writeError(rec, status, errors.New("invalid character name"))

			if rec.Code != status {
				t.Fatalf("status = %d, want %d", rec.Code, status)
			}
			var resp errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Error != "invalid character name" {
				t.Errorf("error = %q, want %q", resp.Error, "invalid character name")
			}
			if buf.Len() != 0 {
				t.Errorf("expected no server log for %d, got %s", status, buf.String())
			}
		}
	})

	t.Run("5xx returns generic message and logs original error", func(t *testing.T) {
		rawErr := errors.New(`pq: relation "players" does not exist`)
		for _, status := range []int{
			http.StatusInternalServerError,
			http.StatusNotImplemented,
			http.StatusServiceUnavailable,
		} {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			rec := httptest.NewRecorder()
			writeError(rec, status, rawErr)

			if rec.Code != status {
				t.Fatalf("status = %d, want %d", rec.Code, status)
			}
			var resp errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Error != "internal server error" {
				t.Errorf("error = %q, want %q", resp.Error, "internal server error")
			}
			if strings.Contains(rec.Body.String(), "players") {
				t.Errorf("response body leaked internal error: %s", rec.Body.String())
			}
			logged := buf.String()
			if !strings.Contains(logged, "http server error") ||
				!strings.Contains(logged, fmt.Sprintf("status=%d", status)) ||
				!strings.Contains(logged, "players") {
				t.Errorf("expected server log with status=%d and raw error, got %q", status, logged)
			}
		}
	})
}
