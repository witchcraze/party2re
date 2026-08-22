package character

import (
	"errors"
	"testing"
)

func TestNewCreatesLevelOneCharacter(t *testing.T) {
	got, err := New("  Alice  ")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.ID == "" || got.Name != "Alice" || got.Level != 1 || got.Experience != 0 {
		t.Fatalf("New() = %#v", got)
	}
}

func TestNewRejectsInvalidNames(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace", input: "   "},
		{name: "too long", input: "123456789012345678901234567890123"},
		{name: "control character", input: "Alice\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.input); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("New(%q) error = %v, want %v", test.input, err, ErrInvalidName)
			}
		})
	}
}
