package logging

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
)

// Logger is the application logging boundary. Implementations must keep
// operation names and attributes safe for the configured output.
type Logger interface {
	Info(context.Context, string, ...slog.Attr)
	Warn(context.Context, string, ...slog.Attr)
	Error(context.Context, string, error, ...slog.Attr)
}

type correlationIDKey struct{}

// WithCorrelationID associates a request or workflow identifier with logs
// written using ctx.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}

// NewJSON creates a structured logger that writes safe JSON records to w.
// A nil writer discards output.
func NewJSON(w io.Writer) Logger {
	if w == nil {
		w = io.Discard
	}
	return &slogLogger{
		logger: slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

// Nop creates a logger that discards records.
func Nop() Logger {
	return NewJSON(io.Discard)
}

type slogLogger struct {
	logger *slog.Logger
}

func (l *slogLogger) Info(ctx context.Context, operation string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelInfo, operation, nil, attrs)
}

func (l *slogLogger) Warn(ctx context.Context, operation string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelWarn, operation, nil, attrs)
}

func (l *slogLogger) Error(ctx context.Context, operation string, err error, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelError, operation, err, attrs)
}

func (l *slogLogger) log(ctx context.Context, level slog.Level, operation string, err error, attrs []slog.Attr) {
	safe := make([]slog.Attr, 0, len(attrs)+3)
	for _, attr := range attrs {
		if sanitized, ok := sanitizeAttr(attr); ok {
			safe = append(safe, sanitized)
		}
	}

	record := []slog.Attr{slog.String("operation", operation)}
	if correlationID, ok := ctx.Value(correlationIDKey{}).(string); ok && correlationID != "" {
		record = append(record, slog.String("correlation_id", correlationID))
	}
	record = append(record, safe...)
	if err != nil {
		// Error messages can accidentally contain a DSN or a credential.
		// The concrete error type preserves diagnostics without copying it.
		record = append(record, slog.String("error", reflect.TypeOf(err).String()))
	}
	l.logger.LogAttrs(ctx, level, operation, record...)
}

var sensitiveKeys = map[string]struct{}{
	"accesstoken":      {},
	"apikey":           {},
	"authorization":    {},
	"connectionstring": {},
	"credential":       {},
	"credentials":      {},
	"cookie":           {},
	"databaseurl":      {},
	"dbdsn":            {},
	"dsn":              {},
	"error":            {},
	"password":         {},
	"passwordhash":     {},
	"passwd":           {},
	"refreshtoken":     {},
	"secret":           {},
	"session":          {},
	"sessionid":        {},
	"token":            {},
}

var sensitiveText = regexp.MustCompile(`(?i)(password|passwd|session(?:[_-]?id)?|credential(?:s)?|access[_-]?token|refresh[_-]?token|api[_-]?key|authorization|secret|dsn)\s*[:=]\s*(?:"[^"]*"|'[^']*'|\S+)`)

func sanitizeAttr(attr slog.Attr) (slog.Attr, bool) {
	if isSensitiveKey(attr.Key) {
		return slog.Attr{}, false
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		safe := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			if sanitized, ok := sanitizeAttr(child); ok {
				safe = append(safe, sanitized)
			}
		}
		attr.Value = slog.GroupValue(safe...)
		return attr, true
	}
	if attr.Value.Kind() == slog.KindString {
		attr.Value = slog.StringValue(sensitiveText.ReplaceAllString(attr.Value.String(), "$1=[REDACTED]"))
	}
	return attr, true
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, key))
	for sensitive := range sensitiveKeys {
		if normalized == sensitive || strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}
