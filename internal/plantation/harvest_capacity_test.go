package plantation_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/plantation"
)

type notFoundDepotRepo struct {
	savedDepot depot.Depot
	saveCalled bool
}

func (m *notFoundDepotRepo) FindByCharacterID(_ context.Context, _ string) (depot.Depot, error) {
	if m.saveCalled {
		return m.savedDepot, nil
	}
	return depot.Depot{}, depot.ErrNotFound
}

func (m *notFoundDepotRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (depot.Depot, error) {
	if m.saveCalled {
		return m.savedDepot, nil
	}
	return depot.Depot{}, depot.ErrNotFound
}

func (m *notFoundDepotRepo) Save(_ context.Context, d depot.Depot) error {
	m.savedDepot = d
	m.saveCalled = true
	return nil
}

func TestHarvest_RefreshesDepotCapacityWithJobLevelAndOverDepot(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c_farmer_lv10"
	charRepo.chars[charID] = corecharacter.Character{
		ID:        charID,
		Name:      "Farmer",
		JobLevel:  10, // base 55
		OverDepot: 1,  // +50
	}

	// Depot with ExDepot = 2 (+10) and stale capacity 5
	existingDepot, _ := depot.NewDepot(charID)
	existingDepot.ExDepot = 2
	existingDepot.Capacity = 5
	_ = depotRepo.Save(context.Background(), existingDepot)

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	fertID := "magic_powder" // 0% wither
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "red",
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	rng := &mockRNG{values: []int{50, 0, 50, 0}} // no wither, base yield
	svc, err := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.Harvest(context.Background(), charID)
	if err != nil {
		t.Fatalf("Harvest failed: %v", err)
	}
	if res.Withered {
		t.Fatal("expected harvest not to wither")
	}

	dep, err := depotRepo.FindByCharacterID(context.Background(), charID)
	if err != nil {
		t.Fatalf("depot not found: %v", err)
	}

	// Expected capacity: JobLevel 10 (55) + ExDepot 2 (10) + OverDepot 1 (50) = 115
	expectedCap := depot.CalculateCapacity(10, 2, 1)
	if dep.Capacity != expectedCap {
		t.Errorf("expected refreshed depot capacity %d, got %d", expectedCap, dep.Capacity)
	}
}

func TestHarvest_InitializesNewDepotWhenNotFound(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := &notFoundDepotRepo{}
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c_farmer_fresh"
	charRepo.chars[charID] = corecharacter.Character{
		ID:        charID,
		Name:      "FreshFarmer",
		JobLevel:  8, // base 45
		OverDepot: 1, // +50
	}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	fertID := "magic_powder"
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "red",
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	rng := &mockRNG{values: []int{50, 0, 50, 0}}
	svc, err := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.Harvest(context.Background(), charID)
	if err != nil {
		t.Fatalf("Harvest failed: %v", err)
	}
	if res.Withered {
		t.Fatal("expected harvest not to wither")
	}

	// Expected capacity: JobLevel 8 (45) + OverDepot 1 (50) = 95
	expectedCap := depot.CalculateCapacity(8, 0, 1)
	if depotRepo.savedDepot.Capacity != expectedCap {
		t.Errorf("expected newly initialized depot capacity %d, got %d", expectedCap, depotRepo.savedDepot.Capacity)
	}
}

func TestHarvest_DepotFullWithRefreshedCapacity(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c_farmer_full"
	charRepo.chars[charID] = corecharacter.Character{
		ID:        charID,
		Name:      "FullFarmer",
		JobLevel:  1, // base 10
		OverDepot: 0,
	}

	// Fill depot with 10 items (capacity for JobLevel 1 is 10)
	dep, _ := depot.NewDepotWithCapacity(charID, 1, 0, 0)
	for i := 1; i <= 10; i++ {
		inst, _ := coreitem.NewInstance(fmt.Sprintf("weapon-%02d", i), 1)
		if err := dep.AddItem(inst); err != nil {
			t.Fatalf("failed to add item %d: %v", i, err)
		}
	}
	_ = depotRepo.Save(context.Background(), dep)

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	fertID := "magic_powder"
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "red",
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	rng := &mockRNG{values: []int{50, 0, 50, 0}}
	svc, err := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.Harvest(context.Background(), charID)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}
}
