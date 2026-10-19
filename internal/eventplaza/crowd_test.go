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

func TestCrowdCandidateSlots(t *testing.T) {
	t.Run("guest slots 65 map to 64 identities with duplicate 60 and 64", func(t *testing.T) {
		if crowdGuestName(0) != "Celebration guest 01" {
			t.Fatalf("slot 0 = %q, want Celebration guest 01", crowdGuestName(0))
		}
		if crowdGuestName(64) != "Celebration guest 61" {
			t.Fatalf("slot 64 = %q, want Celebration guest 61", crowdGuestName(64))
		}
		if crowdGuestName(60) != crowdGuestName(64) {
			t.Fatalf("slots 60 and 64 differ: %q vs %q", crowdGuestName(60), crowdGuestName(64))
		}
		seen := make(map[string]int)
		for slot := 0; slot < 65; slot++ {
			name := crowdGuestName(slot)
			if prev, ok := seen[name]; ok {
				if !(prev == 60 && slot == 64) {
					t.Fatalf("unexpected duplicate between slot %d and %d: %q", prev, slot, name)
				}
			} else {
				seen[name] = slot
			}
		}
		if len(seen) != 64 {
			t.Fatalf("unique guest identities = %d, want 64", len(seen))
		}
		for slot := 59; slot <= 63; slot++ {
			if slot != 60 && crowdGuestName(slot) == crowdGuestName(60) {
				t.Fatalf("control slot %d unexpectedly matches slot 60", slot)
			}
		}
	})

	t.Run("prop slots 32 map to 31 identities with duplicate 20 and 24", func(t *testing.T) {
		if crowdPropName(0) != "Celebration decoration 01" {
			t.Fatalf("slot 0 = %q, want Celebration decoration 01", crowdPropName(0))
		}
		if crowdPropName(31) != "Celebration decoration 31" {
			t.Fatalf("slot 31 = %q, want Celebration decoration 31", crowdPropName(31))
		}
		if crowdPropName(20) != crowdPropName(24) {
			t.Fatalf("slots 20 and 24 differ: %q vs %q", crowdPropName(20), crowdPropName(24))
		}
		seen := make(map[string]int)
		for slot := 0; slot < 32; slot++ {
			name := crowdPropName(slot)
			if prev, ok := seen[name]; ok {
				if !(prev == 20 && slot == 24) {
					t.Fatalf("unexpected duplicate between slot %d and %d: %q", prev, slot, name)
				}
			} else {
				seen[name] = slot
			}
		}
		if len(seen) != 31 {
			t.Fatalf("unique prop identities = %d, want 31", len(seen))
		}
		for _, slot := range []int{19, 21, 22, 23, 25} {
			if crowdPropName(slot) == crowdPropName(20) {
				t.Fatalf("control slot %d unexpectedly matches slot 20", slot)
			}
		}
	})
}

type postReviewCrowdRNG struct {
	guestSlots []int
	propSlots  []int
	guestCount int
	propCount  int
}

func (r *postReviewCrowdRNG) Intn(n int) int {
	switch n {
	case 4:
		return r.guestCount
	case 2:
		return r.propCount
	case 3600:
		return 0
	case 65:
		v := r.guestSlots[0]
		r.guestSlots = r.guestSlots[1:]
		return v
	case 32:
		v := r.propSlots[0]
		r.propSlots = r.propSlots[1:]
		return v
	default:
		return 0
	}
}

func TestPostReviewCrowdDuplicateSlots(t *testing.T) {
	t.Run("duplicate props collapse to tier 1 on subsequent observation", func(t *testing.T) {
		clock := &mockClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
		store := newMockSortedSetStore()
		rng := &postReviewCrowdRNG{
			guestSlots: []int{0, 1},
			propSlots:  []int{20, 24},
			guestCount: 0, // 2 + 0 = 2 guests
			propCount:  1, // 1 + 1 = 2 props
		}
		tracker := NewValkeyPresenceTracker(store.Client())
		svc, err := NewService(newMemoryRepo(), newMemoryCharacterRepo(), newMemoryInventoryRepo(),
			WithClock(clock), WithPresenceTracker(tracker), WithRNG(rng))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		for i := 1; i <= 15; i++ {
			if err := tracker.RecordPresence(ctx, fmt.Sprintf("adventurer:%02d", i), clock.now); err != nil {
				t.Fatal(err)
			}
		}
		if err := svc.RecordVictoryCrowd(ctx); err != nil {
			t.Fatal(err)
		}
		if len(store.members) != 19 {
			t.Fatalf("stored rows before observation = %d, want 19", len(store.members))
		}

		status1, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status1.MerchantSelectionCount != 19 {
			t.Fatalf("first observation MerchantSelectionCount = %d, want 19", status1.MerchantSelectionCount)
		}
		if status1.MerchantTier != 1 {
			t.Fatalf("first observation MerchantTier = %d, want 1", status1.MerchantTier)
		}
		if status1.ActiveParticipants != 19 {
			t.Fatalf("first observation ActiveParticipants = %d, want 19", status1.ActiveParticipants)
		}

		status2, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status2.MerchantSelectionCount != 19 {
			t.Fatalf("second observation MerchantSelectionCount = %d, want 19", status2.MerchantSelectionCount)
		}
		if status2.MerchantTier != 1 {
			t.Fatalf("second observation MerchantTier = %d, want 1 (tier 2 indicates missing duplicate identity collapse)", status2.MerchantTier)
		}
		if status2.ActiveParticipants != 19 {
			t.Fatalf("second observation ActiveParticipants = %d, want 19", status2.ActiveParticipants)
		}
	})

	t.Run("distinct props advance to tier 2 on subsequent observation as contrast", func(t *testing.T) {
		clock := &mockClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
		store := newMockSortedSetStore()
		rng := &postReviewCrowdRNG{
			guestSlots: []int{0, 1},
			propSlots:  []int{20, 21},
			guestCount: 0, // 2 + 0 = 2 guests
			propCount:  1, // 1 + 1 = 2 props
		}
		tracker := NewValkeyPresenceTracker(store.Client())
		svc, err := NewService(newMemoryRepo(), newMemoryCharacterRepo(), newMemoryInventoryRepo(),
			WithClock(clock), WithPresenceTracker(tracker), WithRNG(rng))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		for i := 1; i <= 15; i++ {
			if err := tracker.RecordPresence(ctx, fmt.Sprintf("adventurer:%02d", i), clock.now); err != nil {
				t.Fatal(err)
			}
		}
		if err := svc.RecordVictoryCrowd(ctx); err != nil {
			t.Fatal(err)
		}

		status1, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status1.MerchantSelectionCount != 19 || status1.MerchantTier != 1 || status1.ActiveParticipants != 20 {
			t.Fatalf("first observation = %+v, want stored=19, tier=1, active=20", status1)
		}

		status2, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status2.MerchantSelectionCount != 20 || status2.MerchantTier != 2 || status2.ActiveParticipants != 20 {
			t.Fatalf("second observation = %+v, want stored=20, tier=2, active=20", status2)
		}
	})

	t.Run("duplicate guests collapse correctly", func(t *testing.T) {
		clock := &mockClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
		store := newMockSortedSetStore()
		rng := &postReviewCrowdRNG{
			guestSlots: []int{60, 64},
			propSlots:  []int{0},
			guestCount: 0, // 2 + 0 = 2 guests
			propCount:  0, // 1 + 0 = 1 prop
		}
		tracker := NewValkeyPresenceTracker(store.Client())
		svc, err := NewService(newMemoryRepo(), newMemoryCharacterRepo(), newMemoryInventoryRepo(),
			WithClock(clock), WithPresenceTracker(tracker), WithRNG(rng))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := svc.RecordVictoryCrowd(ctx); err != nil {
			t.Fatal(err)
		}
		if len(store.members) != 3 {
			t.Fatalf("stored rows = %d, want 3", len(store.members))
		}

		status, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status.MerchantSelectionCount != 3 {
			t.Fatalf("selection count = %d, want 3", status.MerchantSelectionCount)
		}
		if status.ActiveParticipants != 2 {
			t.Fatalf("active participants = %d, want 2 (duplicate guest should be collapsed)", status.ActiveParticipants)
		}
	})
}
