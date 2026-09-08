package medal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/medal"
)

type mockCharacterRepo struct {
	mu   sync.Mutex
	char corecharacter.Character
	err  error
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.char, m.err
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(ctx context.Context, value corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.char = value
	return m.err
}

type mockDepotRepo struct {
	mu  sync.Mutex
	dep depot.Depot
	err error
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (depot.Depot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dep.CharacterID == "" {
		return depot.Depot{}, depot.ErrNotFound
	}
	return m.dep, m.err
}

func (m *mockDepotRepo) Save(ctx context.Context, value depot.Depot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dep = value
	return m.err
}

func TestMedalService(t *testing.T) {
	tmpDir := t.TempDir()
	rewardsFile := filepath.Join(tmpDir, "medal_rewards.json")
	rewardsData := `[
		{ "cost": 3, "item_id": "armor-32" },
		{ "cost": 10, "item_id": "weapon-32" }
	]`
	if err := os.WriteFile(rewardsFile, []byte(rewardsData), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("successful claim delivers to depot", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 5}
		d, err := depot.NewDepotWithCapacity("char-1", 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}

		charRepo := &mockCharacterRepo{char: char}
		depotRepo := &mockDepotRepo{dep: d}

		svc, err := medal.NewService(charRepo, depotRepo, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		updatedChar, updatedDepot, err := svc.Claim(context.Background(), "char-1", "armor-32")
		if err != nil {
			t.Fatal(err)
		}

		if updatedChar.SmallMedals != 2 {
			t.Errorf("expected 2 medals, got %d", updatedChar.SmallMedals)
		}

		if len(updatedDepot.Items) != 1 {
			t.Fatalf("expected 1 depot item, got %d", len(updatedDepot.Items))
		}
		if updatedDepot.Items[0].DefinitionID != "armor-32" {
			t.Errorf("expected armor-32 in depot, got %s", updatedDepot.Items[0].DefinitionID)
		}
	})

	t.Run("insufficient medals", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 2}
		d, _ := depot.NewDepotWithCapacity("char-1", 0, 0, 0)

		charRepo := &mockCharacterRepo{char: char}
		depotRepo := &mockDepotRepo{dep: d}

		svc, err := medal.NewService(charRepo, depotRepo, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.Claim(context.Background(), "char-1", "armor-32")
		if !errors.Is(err, medal.ErrInsufficientMedals) {
			t.Errorf("expected ErrInsufficientMedals, got %v", err)
		}
	})

	t.Run("reward not found", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 10}
		d, _ := depot.NewDepotWithCapacity("char-1", 0, 0, 0)

		charRepo := &mockCharacterRepo{char: char}
		depotRepo := &mockDepotRepo{dep: d}

		svc, err := medal.NewService(charRepo, depotRepo, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.Claim(context.Background(), "char-1", "non-existent-item")
		if !errors.Is(err, medal.ErrRewardNotFound) {
			t.Errorf("expected ErrRewardNotFound, got %v", err)
		}
	})

	t.Run("depot full error", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 10}
		d := depot.Depot{
			CharacterID: "char-1",
			Capacity:    1,
			Items: []coreitem.Instance{
				{ID: "existing-item", DefinitionID: "item-001", Quantity: 1},
			},
		}

		charRepo := &mockCharacterRepo{char: char}
		depotRepo := &mockDepotRepo{dep: d}

		svc, err := medal.NewService(charRepo, depotRepo, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.Claim(context.Background(), "char-1", "armor-32")
		if !errors.Is(err, depot.ErrDepotFull) {
			t.Errorf("expected ErrDepotFull, got %v", err)
		}
	})
}

type dummyTxProvider struct{}

func (d dummyTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestMedalService_ConcurrentClaim(t *testing.T) {
	rewards := []medal.Reward{
		{Cost: 3, ItemID: "armor-32"},
	}

	char := corecharacter.Character{ID: "char-1", SmallMedals: 5} // only enough for 1 claim (cost 3)
	d, _ := depot.NewDepotWithCapacity("char-1", 0, 0, 0)

	charRepo := &mockCharacterRepo{char: char}
	depotRepo := &mockDepotRepo{dep: d}

	svc, err := medal.NewServiceWithRewards(
		charRepo,
		depotRepo,
		rewards,
		medal.WithTransactionProvider(dummyTxProvider{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := svc.Claim(context.Background(), "char-1", "armor-32")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	finalChar, _ := charRepo.FindByID(context.Background(), "char-1")
	if finalChar.SmallMedals < 0 {
		t.Fatalf("small medals went negative: %d", finalChar.SmallMedals)
	}
}
