package casino_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestStandardDeck(t *testing.T) {
	deck := casino.NewStandardDeck()
	if deck.Remaining() != 52 {
		t.Fatalf("expected 52 cards, got %d", deck.Remaining())
	}

	drawnMap := make(map[string]bool)
	for i := 0; i < 52; i++ {
		card, err := deck.Draw()
		if err != nil {
			t.Fatalf("failed drawing card at index %d: %v", i, err)
		}
		cardStr := card.String()
		if drawnMap[cardStr] {
			t.Fatalf("duplicate card drawn: %s", cardStr)
		}
		drawnMap[cardStr] = true
	}

	if deck.Remaining() != 0 {
		t.Errorf("deck should be empty, got %d remaining", deck.Remaining())
	}

	_, err := deck.Draw()
	if err == nil {
		t.Error("expected error drawing from empty deck, got nil")
	}
}

func TestDeck_Shuffle(t *testing.T) {
	deck := casino.NewStandardDeck()
	if err := deck.Shuffle(); err != nil {
		t.Fatalf("Shuffle failed: %v", err)
	}
	if deck.Remaining() != 52 {
		t.Errorf("expected 52 cards after shuffle, got %d", deck.Remaining())
	}
}
