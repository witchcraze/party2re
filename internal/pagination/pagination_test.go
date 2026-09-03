package pagination_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestCursorEncodeDecode(t *testing.T) {
	now := time.Date(2026, 9, 2, 20, 0, 0, 123456789, time.UTC)
	id := "post-12345"

	token := pagination.EncodeCursor(now, id)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	decodedTime, decodedID, err := pagination.DecodeCursor(token)
	if err != nil {
		t.Fatalf("DecodeCursor failed: %v", err)
	}
	if !decodedTime.Equal(now) {
		t.Errorf("decoded time %v != %v", decodedTime, now)
	}
	if decodedID != id {
		t.Errorf("decoded id %s != %s", decodedID, id)
	}

	// Zero time and empty id
	emptyToken := pagination.EncodeCursor(time.Time{}, "")
	if emptyToken != "" {
		t.Errorf("expected empty token, got %q", emptyToken)
	}

	emptyTime, emptyID, err := pagination.DecodeCursor("")
	if err != nil || !emptyTime.IsZero() || emptyID != "" {
		t.Errorf("DecodeCursor(\"\") error = %v, got time %v, id %s", err, emptyTime, emptyID)
	}

	// Invalid encodings & formats
	if _, _, err := pagination.DecodeCursor("%%%invalid-base64%%%"); err == nil {
		t.Error("expected error for invalid base64")
	}
	if _, _, err := pagination.DecodeCursor("YWJj"); err == nil { // "abc" (no colon)
		t.Error("expected error for missing delimiter")
	}
	if _, _, err := pagination.DecodeCursor("bm90LW51bWJlcjppZA"); err == nil { // "not-number:id"
		t.Error("expected error for non-numeric timestamp")
	}
}

func TestIDCursorEncodeDecode(t *testing.T) {
	id := "item-abc-123"
	token := pagination.EncodeIDCursor(id)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	decoded, err := pagination.DecodeIDCursor(token)
	if err != nil {
		t.Fatalf("DecodeIDCursor failed: %v", err)
	}
	if decoded != id {
		t.Errorf("decoded ID %s != %s", decoded, id)
	}

	if empty := pagination.EncodeIDCursor(""); empty != "" {
		t.Errorf("expected empty token, got %s", empty)
	}
	if dec, err := pagination.DecodeIDCursor(""); err != nil || dec != "" {
		t.Errorf("DecodeIDCursor(\"\") error = %v, dec = %s", err, dec)
	}
	if _, err := pagination.DecodeIDCursor("%%%invalid%%%"); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestParseCursor(t *testing.T) {
	p := pagination.ParseCursor("some-cursor", "50")
	if p.Cursor != "some-cursor" || p.Limit != 50 {
		t.Errorf("ParseCursor() = %+v, want Cursor: 'some-cursor', Limit: 50", p)
	}

	pDefault := pagination.ParseCursor("", "")
	if pDefault.Cursor != "" || pDefault.Limit != pagination.DefaultLimit {
		t.Errorf("ParseCursor() = %+v, want defaults", pDefault)
	}

	pClamp := pagination.ParseCursor("c", "999")
	if pClamp.Limit != pagination.MaxLimit {
		t.Errorf("ParseCursor() limit = %d, want %d", pClamp.Limit, pagination.MaxLimit)
	}
}

func TestParseCursorRequest(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		p := pagination.ParseCursorRequest(nil)
		if p.Cursor != "" || p.Limit != pagination.DefaultLimit {
			t.Errorf("ParseCursorRequest(nil) = %+v, want defaults", p)
		}
	})

	t.Run("valid query parameters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/posts?cursor=tok123&limit=30", nil)
		p := pagination.ParseCursorRequest(req)
		if p.Cursor != "tok123" || p.Limit != 30 {
			t.Errorf("ParseCursorRequest(req) = %+v, want Cursor: 'tok123', Limit: 30", p)
		}
	})

	t.Run("custom bounds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/posts?cursor=tok123&limit=150", nil)
		p := pagination.ParseCursorRequestWithDefaults(req, 25, 200)
		if p.Cursor != "tok123" || p.Limit != 150 {
			t.Errorf("ParseCursorRequestWithDefaults() = %+v, want Limit: 150", p)
		}
	})
}

func TestNewCursorPage(t *testing.T) {
	t.Run("normal construction", func(t *testing.T) {
		items := []string{"p1", "p2"}
		page := pagination.NewCursorPage(items, "next", "prev", 20, true)
		if len(page.Items) != 2 || page.NextCursor != "next" || page.PrevCursor != "prev" || page.Limit != 20 || !page.HasMore {
			t.Errorf("NewCursorPage() = %+v", page)
		}
	})

	t.Run("nil items slice fallback", func(t *testing.T) {
		var items []string
		page := pagination.NewCursorPage(items, "", "", -5, false)
		if page.Items == nil || len(page.Items) != 0 || page.Limit != pagination.DefaultLimit {
			t.Errorf("NewCursorPage() with nil slice = %+v", page)
		}
	})
}

func TestSliceCursorPage(t *testing.T) {
	type item struct {
		ID   string
		Name string
	}
	all := []item{
		{"1", "One"},
		{"2", "Two"},
		{"3", "Three"},
		{"4", "Four"},
		{"5", "Five"},
	}
	getCursor := func(it item) string { return it.ID }

	// First page (no cursor)
	page1 := pagination.SliceCursorPage(all, 2, "", getCursor)
	if len(page1.Items) != 2 || page1.Items[0].ID != "1" || page1.Items[1].ID != "2" {
		t.Fatalf("page1 items mismatch: %+v", page1.Items)
	}
	if !page1.HasMore || page1.NextCursor != "2" || page1.PrevCursor != "" {
		t.Errorf("page1 metadata mismatch: %+v", page1)
	}

	// Second page (cursor = "2")
	page2 := pagination.SliceCursorPage(all, 2, "2", getCursor)
	if len(page2.Items) != 2 || page2.Items[0].ID != "3" || page2.Items[1].ID != "4" {
		t.Fatalf("page2 items mismatch: %+v", page2.Items)
	}
	if !page2.HasMore || page2.NextCursor != "4" || page2.PrevCursor != "2" {
		t.Errorf("page2 metadata mismatch: %+v", page2)
	}

	// Third page (cursor = "4", last page)
	page3 := pagination.SliceCursorPage(all, 2, "4", getCursor)
	if len(page3.Items) != 1 || page3.Items[0].ID != "5" {
		t.Fatalf("page3 items mismatch: %+v", page3.Items)
	}
	if page3.HasMore || page3.NextCursor != "" || page3.PrevCursor != "4" {
		t.Errorf("page3 metadata mismatch: %+v", page3)
	}

	// Beyond last page (cursor = "5")
	page4 := pagination.SliceCursorPage(all, 2, "5", getCursor)
	if len(page4.Items) != 0 || page4.HasMore {
		t.Errorf("page4 beyond end should be empty: %+v", page4)
	}

	// Non-existent cursor
	pageBad := pagination.SliceCursorPage(all, 2, "non-existent", getCursor)
	if len(pageBad.Items) != 0 || pageBad.HasMore {
		t.Errorf("pageBad should be empty: %+v", pageBad)
	}

	// Empty list
	pageEmpty := pagination.SliceCursorPage([]item{}, 2, "", getCursor)
	if len(pageEmpty.Items) != 0 || pageEmpty.HasMore {
		t.Errorf("pageEmpty should be empty: %+v", pageEmpty)
	}
}

func TestDecodeCursorParts(t *testing.T) {
	now := time.Date(2026, 9, 2, 20, 0, 0, 123456789, time.UTC)
	id := "item-123"

	// Compound cursor
	compoundToken := pagination.EncodeCursor(now, id)
	tDec, idDec := pagination.DecodeCursorParts(compoundToken)
	if !tDec.Equal(now) || idDec != id {
		t.Errorf("DecodeCursorParts(compound) = (%v, %s), want (%v, %s)", tDec, idDec, now, id)
	}

	// Single ID cursor fallback
	idToken := pagination.EncodeIDCursor(id)
	tDecID, idDecID := pagination.DecodeCursorParts(idToken)
	if !tDecID.IsZero() || idDecID != id {
		t.Errorf("DecodeCursorParts(idToken) = (%v, %s), want (zero, %s)", tDecID, idDecID, id)
	}

	// Empty string
	tEmpty, idEmpty := pagination.DecodeCursorParts("")
	if !tEmpty.IsZero() || idEmpty != "" {
		t.Errorf("DecodeCursorParts(\"\") = (%v, %s), want (zero, \"\")", tEmpty, idEmpty)
	}

	// Whitespace only
	tWs, idWs := pagination.DecodeCursorParts("   ")
	if !tWs.IsZero() || idWs != "" {
		t.Errorf("DecodeCursorParts(whitespace) = (%v, %s), want (zero, \"\")", tWs, idWs)
	}
}

func TestBuildCursorPage(t *testing.T) {
	type testItem struct {
		ID        string
		CreatedAt time.Time
	}

	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	getCursor := func(item testItem) (time.Time, string) {
		return item.CreatedAt, item.ID
	}

	t.Run("has more items (len > limit)", func(t *testing.T) {
		raw := []testItem{
			{ID: "1", CreatedAt: t2},
			{ID: "2", CreatedAt: t1},
			{ID: "3", CreatedAt: t0}, // extra item for fetchLimit = limit + 1
		}

		page := pagination.BuildCursorPage(raw, 2, "initial-cursor", getCursor)
		if len(page.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(page.Items))
		}
		if !page.HasMore {
			t.Errorf("expected hasMore to be true")
		}
		if page.PrevCursor != "initial-cursor" {
			t.Errorf("expected PrevCursor 'initial-cursor', got %q", page.PrevCursor)
		}
		expectedNext := pagination.EncodeCursor(t1, "2")
		if page.NextCursor != expectedNext {
			t.Errorf("expected NextCursor %q, got %q", expectedNext, page.NextCursor)
		}
	})

	t.Run("exact or fewer items (hasMore = false)", func(t *testing.T) {
		raw := []testItem{
			{ID: "1", CreatedAt: t2},
		}

		page := pagination.BuildCursorPage(raw, 2, "some-cursor", getCursor)
		if len(page.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(page.Items))
		}
		if page.HasMore {
			t.Errorf("expected hasMore to be false")
		}
		if page.NextCursor != "" {
			t.Errorf("expected NextCursor to be empty, got %q", page.NextCursor)
		}
		if page.PrevCursor != "some-cursor" {
			t.Errorf("expected PrevCursor 'some-cursor', got %q", page.PrevCursor)
		}
	})

	t.Run("empty raw items", func(t *testing.T) {
		page := pagination.BuildCursorPage([]testItem{}, 2, "", getCursor)
		if len(page.Items) != 0 || page.HasMore || page.NextCursor != "" {
			t.Errorf("expected empty page, got %+v", page)
		}
	})
}

func TestBuildCursorPageWithMapper(t *testing.T) {
	type rawEntity struct {
		ID        string
		Value     int
		Timestamp time.Time
	}
	type dto struct {
		ID    string
		Value int
	}

	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)

	raw := []rawEntity{
		{ID: "r1", Value: 100, Timestamp: t1},
		{ID: "r2", Value: 200, Timestamp: t0},
	}

	mapper := func(r rawEntity) dto {
		return dto{ID: r.ID, Value: r.Value * 2}
	}
	getCursor := func(r rawEntity) (time.Time, string) {
		return r.Timestamp, r.ID
	}

	page := pagination.BuildCursorPageWithMapper(raw, 1, "req-token", mapper, getCursor)
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 mapped item, got %d", len(page.Items))
	}
	if page.Items[0].Value != 200 {
		t.Errorf("expected mapped value 200, got %d", page.Items[0].Value)
	}
	if !page.HasMore {
		t.Errorf("expected hasMore to be true")
	}
	expectedNext := pagination.EncodeCursor(t1, "r1")
	if page.NextCursor != expectedNext {
		t.Errorf("expected NextCursor %q, got %q", expectedNext, page.NextCursor)
	}
	if page.PrevCursor != "req-token" {
		t.Errorf("expected PrevCursor 'req-token', got %q", page.PrevCursor)
	}
}
