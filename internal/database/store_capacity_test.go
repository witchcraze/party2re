package database_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

// townCountRendezvous holds each builder after its count so that, without the town
// lock, both see nine stores. With the lock the second builder cannot count until the
// first commits, so the wait ends by timeout.
type townCountRendezvous struct {
	*database.StoreRepository
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func (r *townCountRendezvous) CountActiveTownStores(ctx context.Context, townID string, now time.Time) (int, error) {
	n, err := r.StoreRepository.CountActiveTownStores(ctx, townID, now)
	r.mu.Lock()
	r.arrived++
	if r.arrived == 2 {
		close(r.both)
	}
	r.mu.Unlock()
	select {
	case <-r.both:
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
	}
	return n, err
}

type guildPointLog struct {
	mu      sync.Mutex
	awarded map[string]int
}

func (g *guildPointLog) AddGuildPoints(_ context.Context, characterID string, points int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.awarded[characterID] += points
	return nil
}

// Requires a dedicated DB: the limit counts every active store in the town.
func TestStoreTownCapacitySerializesBuilders(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "character_stores", "store_town_capacity_lock", "store_name_lock")
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "DELETE FROM store_town_capacity_lock"); err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewStoreRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	rendezvous := &townCountRendezvous{StoreRepository: repo, both: make(chan struct{})}
	gp := &guildPointLog{awarded: map[string]int{}}
	svc := store.NewService(rendezvous, chars, depots, items, database.NewTransactionProvider(db), store.WithGuildPoints(gp))
	now := time.Now().UTC()
	// Nine active stores plus an expired one, which must not count.
	for i := range store.MaxTownStores {
		expires := now.Add(time.Hour)
		if i == store.MaxTownStores-1 {
			expires = now.Add(-time.Hour)
		}
		if err := repo.SaveStore(ctx, store.Store{ID: fmt.Sprintf("filler-%d", i), CharacterID: fmt.Sprintf("filler-char-%d", i), TownID: "town1",
			StoreName: fmt.Sprint(i), HouseStyle: "001", Wallpaper: "none", ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	builders := make([]string, 2)
	for i := range builders {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, "TownBuilder", store.StorePrice)
		if err != nil {
			t.Fatal(err)
		}
		builders[i] = c.ID
	}
	errs := testutil.RunRace(
		func() error { _, err := svc.BuildStore(ctx, builders[0], "town1", "001"); return err },
		func() error { _, err := svc.BuildStore(ctx, builders[1], "town1", "001"); return err },
	)
	won := 0
	for i, err := range errs {
		c, findErr := chars.FindByID(ctx, builders[i])
		if findErr != nil {
			t.Fatal(findErr)
		}
		switch {
		case err == nil:
			won++
			if c.Money != 0 {
				t.Fatalf("winner gold = %d", c.Money)
			}
		case errors.Is(err, store.ErrTownMaxStoresReached):
			if c.Money != store.StorePrice || gp.awarded[builders[i]] != 0 {
				t.Fatalf("rejected builder changed: gold=%d gp=%d", c.Money, gp.awarded[builders[i]])
			}
			if _, err := repo.GetStoreByCharacterID(ctx, builders[i]); !errors.Is(err, store.ErrStoreNotFound) {
				t.Fatalf("rejected builder has a store: %v", err)
			}
		default:
			t.Fatalf("unexpected race outcome: %v", err)
		}
	}
	active, err := repo.CountActiveTownStores(ctx, "town1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if won != 1 || active != store.MaxTownStores {
		t.Fatalf("town limit broken: won=%d active=%d", won, active)
	}
}
