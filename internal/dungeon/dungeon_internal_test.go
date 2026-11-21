package dungeon

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestEncodeJSONMap(t *testing.T) {
	if got, err := encodeJSONMap[int](nil); err != nil || got != "{}" {
		t.Fatalf("nil int map = (%q, %v), want ({}, nil)", got, err)
	}
	if got, err := encodeJSONMap(map[string]int{}); err != nil || got != "{}" {
		t.Fatalf("empty int map = (%q, %v), want ({}, nil)", got, err)
	}
	if got, err := encodeJSONMap(map[string]int{"hero": -5}); err != nil || got != `{"hero":-5}` {
		t.Fatalf("non-empty int map = (%q, %v)", got, err)
	}
	if got, err := encodeJSONMap[[]corebattle.ConsumedItem](nil); err != nil || got != "{}" {
		t.Fatalf("nil consumed map = (%q, %v), want ({}, nil)", got, err)
	}
	if got, err := encodeJSONMap(map[string][]corebattle.ConsumedItem{"hero": {{ID: "item-1", Quantity: 2}}}); err != nil || got != `{"hero":[{"id":"item-1","quantity":2}]}` {
		t.Fatalf("non-empty consumed map = (%q, %v)", got, err)
	}
}
