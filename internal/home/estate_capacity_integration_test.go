package home_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/town"
)

// houseCountRendezvous holds each builder after its count so that, without the town
// lock, both see nine houses. With the lock the second builder cannot count until the
// first commits, so the wait ends by timeout.
type houseCountRendezvous struct {
	*database.HomeRepository
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func (r *houseCountRendezvous) CountActiveTownHouses(ctx context.Context, townID string, now time.Time) (int, error) {
	n, err := r.HomeRepository.CountActiveTownHouses(ctx, townID, now)
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

type houseGuildPointLog struct {
	mu      sync.Mutex
	awarded map[string]int
}

func (g *houseGuildPointLog) AddGuildPoints(_ context.Context, characterID string, points int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.awarded[characterID] += points
	return nil
}

// Uses a dedicated DB: the limit counts every active house in the town.
func TestBuildHouseTownCapacitySerializesBuilders(t *testing.T) {
	db := homeIsolatedDB(t)
	ctx := context.Background()
	homes, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	eco, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	rendezvous := &houseCountRendezvous{HomeRepository: homes, both: make(chan struct{})}
	gp := &houseGuildPointLog{awarded: map[string]int{}}
	svc, err := home.NewService(rendezvous, chars, home.WithEconomy(eco), home.WithTransactionProvider(tx), home.WithGuildPoints(gp))
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := town.GetTown("town1")
	now := time.Now().UTC()
	// Nine active houses plus an expired one, which must not count.
	for i := range t1.MaxHouses {
		c, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("HouseFill%d", i))
		if err != nil {
			t.Fatal(err)
		}
		expires := now.Add(time.Hour)
		if i == t1.MaxHouses-1 {
			expires = now.Add(-time.Hour)
		}
		if err := homes.SaveHome(ctx, home.CharacterHome{CharacterID: c.ID, TownID: "town1", HouseStyle: "001", ExpiresAt: &expires,
			CompanionName: home.DefaultCompanionName, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	builders := make([]string, 2)
	for i := range builders {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, "HouseBuilder", t1.Price)
		if err != nil {
			t.Fatal(err)
		}
		builders[i] = c.ID
	}
	errs := testutil.RunRace(
		func() error { _, err := svc.BuildHouse(ctx, builders[0], "town1", "001"); return err },
		func() error { _, err := svc.BuildHouse(ctx, builders[1], "town1", "001"); return err },
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
		case errors.Is(err, home.ErrTownMaxHousesReached):
			if c.Money != t1.Price || gp.awarded[builders[i]] != 0 {
				t.Fatalf("rejected builder changed: gold=%d gp=%d", c.Money, gp.awarded[builders[i]])
			}
			if h, err := homes.GetHome(ctx, builders[i]); err != nil || h.IsActive(time.Now().UTC()) {
				t.Fatalf("rejected builder got a lease: %+v %v", h, err)
			}
		default:
			t.Fatalf("unexpected race outcome: %v", err)
		}
	}
	active, err := homes.CountActiveTownHouses(ctx, "town1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if won != 1 || active != t1.MaxHouses {
		t.Fatalf("town limit broken: won=%d active=%d", won, active)
	}
}
