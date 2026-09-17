package god_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/god"
)

type errDepotRepo struct {
	depot.Depot
	saveErr   error
	findErr   error
	savedData []depot.Depot
}

func (m *errDepotRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (depot.Depot, error) {
	if m.findErr != nil {
		return depot.Depot{}, m.findErr
	}
	return m.Depot, nil
}

func (m *errDepotRepo) Save(_ context.Context, dep depot.Depot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.Depot = dep
	m.savedData = append(m.savedData, dep)
	return nil
}

func TestGod_UnderworldDepot_PreservesJobLevelCapacity(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	depotRepo := newMockDepotRepo()

	char := corecharacter.Character{
		ID:        "char_lv10",
		Name:      "Master",
		JobLevel:  10,
		OverDepot: 0,
	}
	charRepo.characters[char.ID] = char

	// Existing depot with ExDepot = 2 (10 slots bonus), stale capacity 5
	existingDepot, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatalf("failed to create depot: %v", err)
	}
	existingDepot.ExDepot = 2
	existingDepot.Capacity = 5
	_ = depotRepo.Save(ctx, existingDepot)

	svc, err := god.NewService(charRepo, god.WithDepotRepository(depotRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.GrantWish(ctx, char.ID, god.WishExpandDepot, god.RealmUnderworld)
	if err != nil {
		t.Fatalf("GrantWish failed: %v", err)
	}
	if res.Character.OverDepot != 1 {
		t.Fatalf("expected OverDepot 1, got %d", res.Character.OverDepot)
	}

	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("failed to find depot: %v", err)
	}

	// Base for JobLevel 10 is 55. ExDepot = 2 adds 10. OverDepot = 1 adds 50. Total = 115.
	expectedCap := depot.CalculateCapacity(10, 2, 1)
	if dep.Capacity != expectedCap {
		t.Errorf("expected depot capacity %d, got %d", expectedCap, dep.Capacity)
	}
	if expectedCap != 115 {
		t.Errorf("expected expectedCap 115, got %d", expectedCap)
	}
}

func TestGod_UnderworldDepot_PropagatesSaveError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()

	char := corecharacter.Character{
		ID:        "char_err",
		Name:      "Unlucky",
		JobLevel:  5,
		OverDepot: 0,
	}
	charRepo.characters[char.ID] = char

	saveErr := errors.New("database disk full")
	d, _ := depot.NewDepot(char.ID)
	failingDepotRepo := &errDepotRepo{
		Depot:   d,
		saveErr: saveErr,
	}

	svc, err := god.NewService(charRepo, god.WithDepotRepository(failingDepotRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.GrantWish(ctx, char.ID, god.WishExpandDepot, god.RealmUnderworld)
	if !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr %v, got %v", saveErr, err)
	}
}

func TestGod_UnderworldDepot_CreatesNewDepotIfNotFound(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()

	char := corecharacter.Character{
		ID:        "char_new",
		Name:      "Newbie",
		JobLevel:  8,
		OverDepot: 0,
	}
	charRepo.characters[char.ID] = char

	mockRepo := &errDepotRepo{
		findErr: depot.ErrNotFound,
	}

	svc, err := god.NewService(charRepo, god.WithDepotRepository(mockRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.GrantWish(ctx, char.ID, god.WishExpandDepot, god.RealmUnderworld)
	if err != nil {
		t.Fatalf("GrantWish failed: %v", err)
	}
	if res.Character.OverDepot != 1 {
		t.Fatalf("expected OverDepot 1, got %d", res.Character.OverDepot)
	}

	expectedCap := depot.CalculateCapacity(8, 0, 1) // 45 + 50 = 95
	if mockRepo.Capacity != expectedCap {
		t.Errorf("expected newly created depot capacity %d, got %d", expectedCap, mockRepo.Capacity)
	}
}

func TestGod_HeavenDepot_RefreshesCapacityWithJobLevel(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	depotRepo := newMockDepotRepo()

	char := corecharacter.Character{
		ID:        "char_scholar",
		Name:      "Scholar",
		JobLevel:  15, // base capacity 80
		OverDepot: 1,  // +50
	}
	charRepo.characters[char.ID] = char

	// Existing depot with stale capacity 5
	existingDepot, _ := depot.NewDepot(char.ID)
	existingDepot.Capacity = 5
	_ = depotRepo.Save(ctx, existingDepot)

	svc, err := god.NewService(
		charRepo,
		god.WithDepotRepository(depotRepo),
		god.WithRandFloat(func() float64 { return 0.1 }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. WishEroticBook refreshes capacity
	_, err = svc.GrantWish(ctx, char.ID, god.WishEroticBook, god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish WishEroticBook failed: %v", err)
	}

	dep, _ := depotRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	expectedCap := depot.CalculateCapacity(15, 0, 1) // 80 + 50 = 130
	if dep.Capacity != expectedCap {
		t.Errorf("expected refreshed depot capacity %d, got %d", expectedCap, dep.Capacity)
	}

	// 2. WishAlchemyRecipe on fresh depot
	char2 := corecharacter.Character{
		ID:        "char_alchemist",
		Name:      "Alchemist",
		JobLevel:  29, // max base capacity 150
		OverDepot: 0,
	}
	charRepo.characters[char2.ID] = char2

	freshRepo := &errDepotRepo{findErr: depot.ErrNotFound}
	svc2, err := god.NewService(
		charRepo,
		god.WithDepotRepository(freshRepo),
		god.WithRandFloat(func() float64 { return 0.1 }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc2.GrantWish(ctx, char2.ID, god.WishAlchemyRecipe, god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish WishAlchemyRecipe failed: %v", err)
	}

	expectedCap2 := depot.CalculateCapacity(29, 0, 0) // 150
	if freshRepo.Capacity != expectedCap2 {
		t.Errorf("expected depot capacity %d, got %d", expectedCap2, freshRepo.Capacity)
	}
}
