package character

import "testing"

// The public presence icon follows the stored avatar contract exactly.
func TestIsSafePresenceIcon(t *testing.T) {
	for icon, want := range map[string]bool{
		"":                                    true,
		"https://i.imgur.com/abc12.gif":       true,
		"https://i.imgur.com/abcd123456.gif":  true,
		"https://i.imgur.com/abcd.gif":        false,
		"https://i.imgur.com/abcd1234567.gif": false,
		"https://i.imgur.com/abc-12.gif":      false,
		"https://i.imgur.com/abc12.png":       false,
		"http://i.imgur.com/abc12.gif":        false,
		"https://example.com/a.png":           false,
		"data:image/png;base64,iVBORw0KGgo=":  false,
		"data:image/svg+xml;base64,PHN2Zz4=":  false,
		"javascript:alert(1)":                 false,
		" https://i.imgur.com/abc12.gif":      false,
		"https://i.imgur.com/abc12.gif\n":     false,
	} {
		if got := IsSafePresenceIcon(icon); got != want {
			t.Errorf("%q: got %v, want %v", icon, got, want)
		}
	}
}
