package character

import "testing"

func TestIsSafePresenceIcon(t *testing.T) {
	for icon, want := range map[string]bool{
		"":                                   true,
		"https://example.com/a.png":          true,
		"data:image/png;base64,iVBORw0KGgo=": true,
		"data:image/webp;base64,UklGRg==":    true,
		"data:image/svg+xml;base64,PHN2Zz4=": false,
		"data:text/html;base64,PGI+":         false,
		"javascript:alert(1)":                false,
		" https://example.com/a.png":         false,
		"https://example.com/a.png\n":        false,
		"ftp://example.com/a.png":            false,
	} {
		if got := IsSafePresenceIcon(icon); got != want {
			t.Errorf("%q: got %v, want %v", icon, got, want)
		}
	}
}
