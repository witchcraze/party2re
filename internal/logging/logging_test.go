package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"log/slog"
)

func TestNewJSONWritesStructuredOperationAndCorrelationFields(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output)
	ctx := WithCorrelationID(context.Background(), "request-123")

	logger.Info(ctx, "player.register", slog.String("username", "alice"))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log output: %v", err)
	}
	if record["operation"] != "player.register" {
		t.Fatalf("operation = %#v", record["operation"])
	}
	if record["correlation_id"] != "request-123" {
		t.Fatalf("correlation_id = %#v", record["correlation_id"])
	}
	if record["username"] != "alice" {
		t.Fatalf("username = %#v", record["username"])
	}
	if record["msg"] != "player.register" {
		t.Fatalf("msg = %#v", record["msg"])
	}
}

func TestLoggerOmitsSensitiveAttributesAndRedactsSensitiveText(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output)

	logger.Info(context.Background(), "player.login",
		slog.String("username", "alice"),
		slog.String("password", "password-value"),
		slog.String("session_id", "session-value"),
		slog.String("credential", "credential-value"),
		slog.Group("auth", slog.String("token", "token-value")),
		slog.String("details", "password=password-value session=session-value"),
	)

	logOutput := output.String()
	for _, secret := range []string{"password-value", "session-value", "credential-value", "token-value"} {
		if strings.Contains(logOutput, secret) {
			t.Fatalf("log contains secret %q: %s", secret, logOutput)
		}
	}
	for _, key := range []string{`"password"`, `"session_id"`, `"credential"`, `"token"`} {
		if strings.Contains(logOutput, key) {
			t.Fatalf("log contains sensitive key %q: %s", key, logOutput)
		}
	}
	if !strings.Contains(logOutput, "REDACTED") {
		t.Fatalf("log did not redact sensitive text: %s", logOutput)
	}
}

func TestLoggerRecordsErrorTypeWithoutErrorMessage(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output)

	logger.Error(context.Background(), "player.login", errors.New("password=password-value session=session-value credential=credential-value"))

	logOutput := output.String()
	for _, secret := range []string{"password-value", "session-value", "credential-value"} {
		if strings.Contains(logOutput, secret) {
			t.Fatalf("log contains secret %q: %s", secret, logOutput)
		}
	}
	if !strings.Contains(logOutput, `"error":"*errors.errorString"`) {
		t.Fatalf("log does not contain safe error type: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"operation":"player.login"`) {
		t.Fatalf("log does not contain operation: %s", logOutput)
	}
}

func TestNopLoggerDiscardsOutput(t *testing.T) {
	logger := Nop()
	ctx := context.Background()
	logger.Info(ctx, "test.op", slog.String("key", "val"))
	logger.Warn(ctx, "test.op", slog.String("key", "val"))
	logger.Error(ctx, "test.op", errors.New("err"), slog.String("key", "val"))
}

func TestNewJSONNilWriterDiscardsOutput(t *testing.T) {
	logger := NewJSON(nil)
	ctx := context.Background()
	logger.Info(ctx, "test.op", slog.String("key", "val"))
	logger.Warn(ctx, "test.op", slog.String("key", "val"))
	logger.Error(ctx, "test.op", errors.New("err"), slog.String("key", "val"))
}

func TestLoggerWarnLevel(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output)

	logger.Warn(context.Background(), "player.warning", slog.Int("attempts", 3))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log output: %v", err)
	}
	if record["level"] != "WARN" {
		t.Fatalf("level = %#v, want WARN", record["level"])
	}
	if record["operation"] != "player.warning" {
		t.Fatalf("operation = %#v, want player.warning", record["operation"])
	}
	if record["attempts"] != float64(3) {
		t.Fatalf("attempts = %#v, want 3", record["attempts"])
	}
}

func TestLoggerPreservesSafeNonStringAndNestedGroupAttributes(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output)

	logger.Info(context.Background(), "character.action",
		slog.Int("level", 5),
		slog.Bool("active", true),
		slog.Group("metadata",
			slog.String("region", "forest"),
			slog.String("api_key", "secret-key-val"),
			slog.Int("stage_id", 10),
		),
	)

	logOutput := output.String()
	if strings.Contains(logOutput, "secret-key-val") || strings.Contains(logOutput, "api_key") {
		t.Fatalf("log contains nested secret: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"level":5`) {
		t.Fatalf("log missing int attribute: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"active":true`) {
		t.Fatalf("log missing bool attribute: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"region":"forest"`) || !strings.Contains(logOutput, `"stage_id":10`) {
		t.Fatalf("log missing safe group attributes: %s", logOutput)
	}
}
