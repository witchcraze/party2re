package home

import "testing"

// The legacy item-259 list has 51 entries (_data.cgi), slot 0 being item 059;
// 2..4 draws pick slots 1..50.
func TestSealedBoxPool_DrawBounds(t *testing.T) {
	if n := len(sealedBoxPool.slots); n != 51 || sealedBoxPool.slots[0] != 59 {
		t.Fatalf("pool: len=%d slot0=%d", n, sealedBoxPool.slots[0])
	}
	for _, tc := range []struct{ countRoll, want int }{{0, 2}, {2, 4}} {
		rng := &scriptedRNG{vals: []int{tc.countRoll, 0, 49, 0, 0}}
		got := sealedBoxPool.draw(rng.Intn)
		if len(got) != tc.want || rng.bounds[0] != 3 || rng.bounds[1] != 50 || got[0] != 150 || got[1] != 216 {
			t.Fatalf("roll %d: got %v bounds %v", tc.countRoll, got, rng.bounds)
		}
	}
}
