package database

import "testing"

// Factory players share one hash, which must still authenticate like a real player's.
func TestTestPlayerTemplateReusesValidHash(t *testing.T) {
	a, err := testPlayerTemplate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := testPlayerTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if a.PasswordHash != b.PasswordHash {
		t.Fatal("password hash recomputed")
	}
	if !a.Authenticate(testPlayerPassword) {
		t.Fatal("factory password rejected")
	}
	if a.Authenticate("wrong-password") {
		t.Fatal("wrong password accepted")
	}
}
