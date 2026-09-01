package pagination

import (
	"net/http"
	"strconv"
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
