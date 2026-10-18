package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestRealAndVirtualGuestsShareDisplayNameCleanup(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Now().UTC()}
	characters := newMemoryCharacterRepo()
	char, err := corecharacter.New("Celebration guest 01")
	if err != nil {
		t.Fatal(err)
	}
	characters.characters[char.ID] = char
	store := newMockSortedSetStore()
	tracker := NewValkeyPresenceTracker(store.Client())
	svc, err := NewService(newMemoryRepo(), characters, newMemoryInventoryRepo(), WithClock(clock), WithPresenceTracker(tracker))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordPresence(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	if err := tracker.AppendCrowd(ctx, []Presence{{char.Name, clock.now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	status, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.MerchantSelectionCount != 2 || status.ActiveParticipants != 1 {
		t.Fatalf("same display name was counted twice: %+v", status)
	}
	clock.now = clock.now.Add(PresenceWindow + time.Second)
	status, err = svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveParticipants != 0 {
		t.Fatalf("later duplicate extended first visitor's marker: %+v", status)
	}
}

func TestMerchantMixedPopulationCatalogBoundaries(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	for _, count := range []int{9, 10, 19, 20, 29, 30} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store := newMockSortedSetStore()
			tracker := NewValkeyPresenceTracker(store.Client())
			if err := tracker.RecordPresence(ctx, "hero", now); err != nil {
				t.Fatal(err)
			}
			crowd := make([]Presence, 0, count-1)
			for i := 1; i < count; i++ {
				crowd = append(crowd, Presence{fmt.Sprint("guest", i), now.Add(time.Hour)})
			}
			if err := tracker.AppendCrowd(ctx, crowd); err != nil {
				t.Fatal(err)
			}
			svc := newTrackedService(t, store, &mockClock{now: now})
			items, tier, err := svc.ListAvailableBazaarItems(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wantTier, _, _ := CalculateMerchantTier(count)
			if tier != wantTier {
				t.Fatalf("catalog tier=%d want=%d", tier, wantTier)
			}
			if tier == 0 && len(items) != 0 || tier > 0 && len(items) == 0 {
				t.Fatalf("tier %d catalog = %+v", tier, items)
			}
			for _, item := range items {
				if item.TierRequired != tier {
					t.Fatalf("wrong catalog item: %+v", item)
				}
			}
		})
	}
}

func TestCatalogAndPurchaseUseExpiredStoredSnapshot(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	for _, operation := range []string{"catalog", "purchase"} {
		t.Run(operation, func(t *testing.T) {
			store := newMockSortedSetStore()
			tracker := NewValkeyPresenceTracker(store.Client())
			for i := 0; i < 10; i++ {
				if err := tracker.RecordPresence(ctx, fmt.Sprint(i), now.Add(-6*time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			characters := newMemoryCharacterRepo()
			char, err := corecharacter.New("Hero")
			if err != nil {
				t.Fatal(err)
			}
			char.Money = 50000
			characters.characters[char.ID] = char
			svc, err := NewService(newMemoryRepo(), characters, newMemoryInventoryRepo(), WithClock(&mockClock{now: now}), WithPresenceTracker(tracker))
			if err != nil {
				t.Fatal(err)
			}
			if operation == "catalog" {
				items, tier, err := svc.ListAvailableBazaarItems(ctx)
				if err != nil || tier != 1 || len(items) != 6 {
					t.Fatalf("first catalog=%+v tier=%d err=%v", items, tier, err)
				}
				items, tier, err = svc.ListAvailableBazaarItems(ctx)
				if err != nil || tier != 0 || len(items) != 0 {
					t.Fatalf("cleaned catalog=%+v tier=%d err=%v", items, tier, err)
				}
			} else {
				result, err := svc.PurchaseBazaarItem(ctx, char.ID, "item-072", 1)
				if err != nil {
					t.Fatal(err)
				}
				if result.TotalPrice != 2400 || result.RemainingGold != 47600 {
					t.Fatalf("purchase lost pricing: %+v", result)
				}
				if _, err := svc.PurchaseBazaarItem(ctx, char.ID, "item-072", 1); !errors.Is(err, ErrMerchantNotPresent) {
					t.Fatalf("cleaned purchase guard=%v", err)
				}
				if characters.characters[char.ID].Money != 47600 {
					t.Fatal("rejected purchase charged gold")
				}
			}
		})
	}
}
