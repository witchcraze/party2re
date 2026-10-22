package challenge_test

import (
	"github.com/witchcraze/party2re/internal/challenge"
	"testing"
)

func TestExitBonus(t *testing.T) {
	for _, tc := range []struct {
		item      string
		round     int
		draw      float64
		exp, gold int
	}{
		{"item-105", 0, .5, 0, 0}, {"item-105", 3, .5, 60, 0}, {"item-106", 0, .5, 0, 0},
		{"item-106", 3, .5, 0, 115}, {"item-106", 2, .999, 0, 153}, {"", 3, .5, 0, 0},
	} {
		exp, gold, err := challenge.ExitBonus(tc.item, tc.round, tc.draw)
		if err != nil || exp != tc.exp || gold != tc.gold {
			t.Fatalf("%+v got=%d/%d %v", tc, exp, gold, err)
		}
	}
}
