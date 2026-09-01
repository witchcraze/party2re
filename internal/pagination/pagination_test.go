package pagination_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/witchcraze/party2re/internal/pagination"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{"default values on zero", 0, 0, 20, 0},
		{"negative values clamped", -10, -5, 20, 0},
		{"normal in-range values", 15, 30, 15, 30},
		{"upper bound limit clamped", 250, 10, 100, 10},
		{"boundary max limit exact", 100, 0, 100, 0},
		{"boundary min limit exact", 1, 0, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := pagination.Normalize(tt.limit, tt.offset)
			if gotLimit != tt.wantLimit {
				t.Errorf("Normalize() limit = %d, want %d", gotLimit, tt.wantLimit)
			}
			if gotOffset != tt.wantOffset {
				t.Errorf("Normalize() offset = %d, want %d", gotOffset, tt.wantOffset)
			}
		})
	}
}

func TestNormalizeWithDefaults(t *testing.T) {
	tests := []struct {
		name         string
		limit        int
		offset       int
		defaultLimit int
		maxLimit     int
		wantLimit    int
		wantOffset   int
	}{
		{"custom defaults", 0, -1, 50, 200, 50, 0},
		{"custom max clamp", 300, 5, 50, 200, 200, 5},
		{"invalid custom defaults fallback", 0, 0, 0, 0, 20, 0},
		{"defaultLimit > maxLimit fallback", 0, 0, 150, 50, 50, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := pagination.NormalizeWithDefaults(tt.limit, tt.offset, tt.defaultLimit, tt.maxLimit)
			if gotLimit != tt.wantLimit {
				t.Errorf("NormalizeWithDefaults() limit = %d, want %d", gotLimit, tt.wantLimit)
			}
			if gotOffset != tt.wantOffset {
				t.Errorf("NormalizeWithDefaults() offset = %d, want %d", gotOffset, tt.wantOffset)
			}
		})
	}
}

func TestNewParams(t *testing.T) {
	p := pagination.NewParams(50, 100)
	if p.Limit != 50 || p.Offset != 100 {
		t.Errorf("NewParams(50, 100) = %+v, want Limit: 50, Offset: 100", p)
	}

	pInvalid := pagination.NewParams(-1, -1)
	if pInvalid.Limit != pagination.DefaultLimit || pInvalid.Offset != pagination.DefaultOffset {
		t.Errorf("NewParams(-1, -1) = %+v, want default values", pInvalid)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		limitStr   string
		offsetStr  string
		wantLimit  int
		wantOffset int
	}{
		{"valid numbers", "30", "15", 30, 15},
		{"empty strings fallback", "", "", 20, 0},
		{"non-numeric strings fallback", "abc", "xyz", 20, 0},
		{"partial invalid limit", "invalid", "10", 20, 10},
		{"partial invalid offset", "40", "invalid", 40, 0},
		{"excessive limit clamped", "999", "50", 100, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pagination.Parse(tt.limitStr, tt.offsetStr)
			if p.Limit != tt.wantLimit || p.Offset != tt.wantOffset {
				t.Errorf("Parse(%q, %q) = %+v, want Limit: %d, Offset: %d",
					tt.limitStr, tt.offsetStr, p, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestParseRequest(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		p := pagination.ParseRequest(nil)
		if p.Limit != pagination.DefaultLimit || p.Offset != pagination.DefaultOffset {
			t.Errorf("ParseRequest(nil) = %+v, want defaults", p)
		}
	})

	t.Run("query parameters present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?limit=25&offset=50", nil)
		p := pagination.ParseRequest(req)
		if p.Limit != 25 || p.Offset != 50 {
			t.Errorf("ParseRequest(req) = %+v, want Limit: 25, Offset: 50", p)
		}
	})

	t.Run("ParseRequestWithDefaults nil and custom bounds", func(t *testing.T) {
		pNil := pagination.ParseRequestWithDefaults(nil, 50, 200)
		if pNil.Limit != 50 || pNil.Offset != 0 {
			t.Errorf("ParseRequestWithDefaults(nil, 50, 200) = %+v, want Limit: 50, Offset: 0", pNil)
		}

		req := httptest.NewRequest(http.MethodGet, "/test?limit=150&offset=10", nil)
		p := pagination.ParseRequestWithDefaults(req, 50, 200)
		if p.Limit != 150 || p.Offset != 10 {
			t.Errorf("ParseRequestWithDefaults(req) = %+v, want Limit: 150, Offset: 10", p)
		}
	})
}

func TestNewPage(t *testing.T) {
	t.Run("non-nil items", func(t *testing.T) {
		items := []string{"a", "b", "c"}
		page := pagination.NewPage(items, 10, 20, 0)
		if len(page.Items) != 3 || page.Total != 10 || page.Limit != 20 || page.Offset != 0 {
			t.Errorf("NewPage() = %+v, want items len 3, total 10", page)
		}
	})

	t.Run("nil items converted to empty non-nil slice", func(t *testing.T) {
		var items []string
		page := pagination.NewPage(items, 0, -5, -1)
		if page.Items == nil {
			t.Errorf("NewPage() Items should not be nil")
		}
		if len(page.Items) != 0 || page.Total != 0 || page.Limit != 20 || page.Offset != 0 {
			t.Errorf("NewPage() = %+v, want empty slice and clamped defaults", page)
		}
	})

	t.Run("NewPageWithDefaults custom bounds", func(t *testing.T) {
		items := []int{1, 2}
		page := pagination.NewPageWithDefaults(items, 2, 50, 0, 50, 200)
		if page.Limit != 50 || page.Total != 2 || len(page.Items) != 2 {
			t.Errorf("NewPageWithDefaults() = %+v, want Limit: 50, Total: 2", page)
		}
	})
}

func TestSlicePage(t *testing.T) {
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		name       string
		limit      int
		offset     int
		wantItems  []int
		wantTotal  int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "first page",
			limit:      4,
			offset:     0,
			wantItems:  []int{1, 2, 3, 4},
			wantTotal:  10,
			wantLimit:  4,
			wantOffset: 0,
		},
		{
			name:       "middle page",
			limit:      4,
			offset:     4,
			wantItems:  []int{5, 6, 7, 8},
			wantTotal:  10,
			wantLimit:  4,
			wantOffset: 4,
		},
		{
			name:       "partial last page",
			limit:      4,
			offset:     8,
			wantItems:  []int{9, 10},
			wantTotal:  10,
			wantLimit:  4,
			wantOffset: 8,
		},
		{
			name:       "offset beyond total",
			limit:      4,
			offset:     20,
			wantItems:  []int{},
			wantTotal:  10,
			wantLimit:  4,
			wantOffset: 20,
		},
		{
			name:       "empty input slice",
			limit:      10,
			offset:     0,
			wantItems:  []int{},
			wantTotal:  0,
			wantLimit:  10,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := pagination.SlicePage(all, tt.limit, tt.offset)
			if tt.name == "empty input slice" {
				page = pagination.SlicePage([]int{}, tt.limit, tt.offset)
			}
			if page.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", page.Total, tt.wantTotal)
			}
			if page.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", page.Limit, tt.wantLimit)
			}
			if page.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", page.Offset, tt.wantOffset)
			}
			if len(page.Items) != len(tt.wantItems) {
				t.Fatalf("Items len = %d, want %d", len(page.Items), len(tt.wantItems))
			}
			for i, v := range page.Items {
				if v != tt.wantItems[i] {
					t.Errorf("Items[%d] = %d, want %d", i, v, tt.wantItems[i])
				}
			}
		})
	}
}
