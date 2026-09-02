package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultLimit is the standard number of items returned per page.
	DefaultLimit = 20

	// MaxLimit is the maximum allowable page size across all endpoints.
	MaxLimit = 100

	// DefaultOffset is the starting offset when none is specified.
	DefaultOffset = 0
)

// Params encapsulates standardized pagination parameters.
type Params struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Page is the standardized generic paginated response container.
type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Normalize ensures limit and offset stay within allowed system bounds.
func Normalize(limit, offset int) (int, int) {
	return NormalizeWithDefaults(limit, offset, DefaultLimit, MaxLimit)
}

// NormalizeWithDefaults ensures limit and offset stay within specified bounds.
func NormalizeWithDefaults(limit, offset, defaultLimit, maxLimit int) (int, int) {
	if defaultLimit <= 0 {
		defaultLimit = DefaultLimit
	}
	if maxLimit <= 0 {
		maxLimit = MaxLimit
	}
	if defaultLimit > maxLimit {
		defaultLimit = maxLimit
	}

	if limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}

	if offset < 0 {
		offset = DefaultOffset
	}

	return limit, offset
}

// NewParams creates a validated Params struct with boundary normalization applied.
func NewParams(limit, offset int) Params {
	return NewParamsWithDefaults(limit, offset, DefaultLimit, MaxLimit)
}

// NewParamsWithDefaults creates a Params struct normalized with custom defaults.
func NewParamsWithDefaults(limit, offset, defaultLimit, maxLimit int) Params {
	l, o := NormalizeWithDefaults(limit, offset, defaultLimit, maxLimit)
	return Params{
		Limit:  l,
		Offset: o,
	}
}

// Parse extracts and validates pagination parameters from string representations.
func Parse(limitStr, offsetStr string) Params {
	return ParseWithDefaults(limitStr, offsetStr, DefaultLimit, MaxLimit)
}

// ParseWithDefaults extracts and validates pagination parameters with custom defaults.
func ParseWithDefaults(limitStr, offsetStr string, defaultLimit, maxLimit int) Params {
	limit := defaultLimit
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	offset := DefaultOffset
	if offsetStr != "" {
		if val, err := strconv.Atoi(offsetStr); err == nil {
			offset = val
		}
	}

	return NewParamsWithDefaults(limit, offset, defaultLimit, maxLimit)
}

// ParseRequest extracts and validates pagination parameters from an HTTP request query string.
func ParseRequest(r *http.Request) Params {
	return ParseRequestWithDefaults(r, DefaultLimit, MaxLimit)
}

// ParseRequestWithDefaults extracts and validates pagination parameters from an HTTP request query string with custom defaults.
func ParseRequestWithDefaults(r *http.Request, defaultLimit, maxLimit int) Params {
	if r == nil || r.URL == nil {
		return NewParamsWithDefaults(defaultLimit, DefaultOffset, defaultLimit, maxLimit)
	}
	q := r.URL.Query()
	return ParseWithDefaults(q.Get("limit"), q.Get("offset"), defaultLimit, maxLimit)
}

// NewPage constructs a Page container ensuring non-nil items slice and valid metadata.
func NewPage[T any](items []T, total, limit, offset int) Page[T] {
	return NewPageWithDefaults(items, total, limit, offset, DefaultLimit, MaxLimit)
}

// NewPageWithDefaults constructs a Page container ensuring non-nil items slice and valid metadata with custom limits.
func NewPageWithDefaults[T any](items []T, total, limit, offset, defaultLimit, maxLimit int) Page[T] {
	if items == nil {
		items = make([]T, 0)
	}
	if total < 0 {
		total = 0
	}
	limit, offset = NormalizeWithDefaults(limit, offset, defaultLimit, maxLimit)

	return Page[T]{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

// SlicePage paginates an in-memory slice according to the specified limit and offset.
func SlicePage[T any](all []T, limit, offset int) Page[T] {
	total := len(all)
	limit, offset = Normalize(limit, offset)

	if offset >= total {
		return Page[T]{
			Items:  make([]T, 0),
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	items := make([]T, end-offset)
	copy(items, all[offset:end])

	return Page[T]{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

// CursorParams encapsulates standardized keyset / cursor pagination parameters.
type CursorParams struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

// CursorPage is the standardized generic cursor-paginated response container.
type CursorPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
}

// EncodeCursor encodes a timestamp and secondary unique ID into an opaque, URL-safe base64 cursor token.
func EncodeCursor(t time.Time, id string) string {
	if t.IsZero() && id == "" {
		return ""
	}
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes an opaque base64 cursor token into a timestamp and secondary ID.
func DecodeCursor(token string) (time.Time, string, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return time.Time{}, "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(trimmed)
		if err != nil {
			return time.Time{}, "", errors.New("invalid cursor encoding")
		}
	}
	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("invalid cursor format")
	}
	nano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", errors.New("invalid cursor timestamp")
	}
	return time.Unix(0, nano).UTC(), parts[1], nil
}

// EncodeIDCursor encodes a single identifier into an opaque base64 cursor token.
func EncodeIDCursor(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(trimmed))
}

// DecodeIDCursor decodes a single identifier opaque base64 cursor token.
func DecodeIDCursor(token string) (string, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(trimmed)
		if err != nil {
			return "", errors.New("invalid cursor encoding")
		}
	}
	return string(data), nil
}

// ParseCursor extracts and validates cursor pagination parameters with default limits.
func ParseCursor(cursorStr, limitStr string) CursorParams {
	return ParseCursorWithDefaults(cursorStr, limitStr, DefaultLimit, MaxLimit)
}

// ParseCursorWithDefaults extracts and validates cursor pagination parameters with custom limits.
func ParseCursorWithDefaults(cursorStr, limitStr string, defaultLimit, maxLimit int) CursorParams {
	limit := defaultLimit
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}
	limit, _ = NormalizeWithDefaults(limit, 0, defaultLimit, maxLimit)
	return CursorParams{
		Cursor: strings.TrimSpace(cursorStr),
		Limit:  limit,
	}
}

// ParseCursorRequest extracts cursor parameters from an HTTP request query string.
func ParseCursorRequest(r *http.Request) CursorParams {
	return ParseCursorRequestWithDefaults(r, DefaultLimit, MaxLimit)
}

// ParseCursorRequestWithDefaults extracts cursor parameters from an HTTP request query string with custom limits.
func ParseCursorRequestWithDefaults(r *http.Request, defaultLimit, maxLimit int) CursorParams {
	if r == nil || r.URL == nil {
		return ParseCursorWithDefaults("", "", defaultLimit, maxLimit)
	}
	q := r.URL.Query()
	return ParseCursorWithDefaults(q.Get("cursor"), q.Get("limit"), defaultLimit, maxLimit)
}

// NewCursorPage constructs a CursorPage container ensuring non-nil items slice and valid metadata.
func NewCursorPage[T any](items []T, nextCursor, prevCursor string, limit int, hasMore bool) CursorPage[T] {
	return NewCursorPageWithDefaults(items, nextCursor, prevCursor, limit, hasMore, DefaultLimit, MaxLimit)
}

// NewCursorPageWithDefaults constructs a CursorPage container with custom bounds.
func NewCursorPageWithDefaults[T any](items []T, nextCursor, prevCursor string, limit int, hasMore bool, defaultLimit, maxLimit int) CursorPage[T] {
	if items == nil {
		items = make([]T, 0)
	}
	limit, _ = NormalizeWithDefaults(limit, 0, defaultLimit, maxLimit)
	return CursorPage[T]{
		Items:      items,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
		Limit:      limit,
		HasMore:    hasMore,
	}
}

// SliceCursorPage paginates an in-memory slice using cursor-based keyset navigation.
// getCursor extracts the unique cursor string from an item.
func SliceCursorPage[T any](all []T, limit int, cursor string, getCursor func(item T) string) CursorPage[T] {
	limit, _ = Normalize(limit, 0)
	if len(all) == 0 {
		return NewCursorPage([]T{}, "", "", limit, false)
	}

	startIndex := 0
	if cursor != "" && getCursor != nil {
		found := false
		for i, item := range all {
			if getCursor(item) == cursor {
				startIndex = i + 1
				found = true
				break
			}
		}
		if !found {
			return NewCursorPage([]T{}, "", "", limit, false)
		}
	}

	if startIndex >= len(all) {
		return NewCursorPage([]T{}, "", "", limit, false)
	}

	endIndex := startIndex + limit
	hasMore := false
	if endIndex < len(all) {
		hasMore = true
	} else {
		endIndex = len(all)
	}

	items := make([]T, endIndex-startIndex)
	copy(items, all[startIndex:endIndex])

	var nextCursor string
	if hasMore && len(items) > 0 && getCursor != nil {
		nextCursor = getCursor(items[len(items)-1])
	}

	var prevCursor string
	if startIndex > 0 && getCursor != nil {
		prevCursor = getCursor(all[startIndex-1])
	}

	return NewCursorPage(items, nextCursor, prevCursor, limit, hasMore)
}
