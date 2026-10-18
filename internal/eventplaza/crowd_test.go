package eventplaza

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type crowdRNG struct {
	counts []int
	names  int
}

func (r *crowdRNG) Intn(n int) int {
	switch n {
	case 4, 2:
		v := r.counts[0]
		r.counts = r.counts[1:]
		return v
	case 3600:
		return (r.names % 2) * 3599
	default:
		r.names++
		return r.names % n
	}
}

func TestVictoryCrowdIndependentDraws(t *testing.T) {
	for _, counts := range [][2]int{{0, 0}, {0, 1}, {3, 0}, {3, 1}} {
		t.Run(fmt.Sprint(counts), func(t *testing.T) {
			clock := &mockClock{now: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)}
			store := newMockSortedSetStore()
			rng := &crowdRNG{counts: []int{counts[0], counts[1], counts[0], counts[1]}}
			svc, err := NewService(newMemoryRepo(), newMemoryCharacterRepo(), newMemoryInventoryRepo(),
				WithClock(clock), WithPresenceTracker(NewValkeyPresenceTracker(store.Client())), WithRNG(rng))
			if err != nil {
				t.Fatal(err)
			}
			want := 3 + counts[0] + counts[1]
			for victory := 1; victory <= 2; victory++ {
				if err := svc.RecordVictoryCrowd(context.Background()); err != nil {
					t.Fatal(err)
				}
				if len(store.members) != victory*want {
					t.Fatalf("victory %d: got %d stored rows, want %d", victory, len(store.members), victory*want)
				}
				if victory == 1 {
					guests, props := 0, 0
					markers := make(map[int64]bool)
					for member, marker := range store.members {
						if strings.Contains(member, "Celebration guest") {
							guests++
						} else {
							props++
						}
						markers[int64(marker)-clock.now.Unix()] = true
					}
					if guests != 2+counts[0] || props != 1+counts[1] || !markers[3600] || !markers[7199] {
						t.Fatalf("unexpected generated crowd: guests=%d props=%d markers=%v", guests, props, markers)
					}
					status, err := svc.GetPlazaStatus(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if status.MerchantTier != 0 || status.ActiveParticipants != want {
						t.Fatalf("single victory = %+v", status)
					}
				}
			}
			status, err := svc.GetPlazaStatus(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status.MerchantSelectionCount != 2*want {
				t.Fatalf("selection count = %d, want %d", status.MerchantSelectionCount, 2*want)
			}
			for _, marker := range store.members {
				delta := int64(marker) - clock.now.Unix()
				if delta != 0 && delta != 3600 && delta != 7199 {
					t.Fatalf("unexpected marker delta %d", delta)
				}
			}
		})
	}
}
