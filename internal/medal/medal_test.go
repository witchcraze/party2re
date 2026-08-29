package medal_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

type mockInventoryRepo struct {
	mu  sync.Mutex
	inv coreinventory.Inventory
	err error
}

func (m *mockInventoryRepo) FindByCharacterID(ctx context.Context, id string) (coreinventory.Inventory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inv, m.err
}

func (m *mockInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, id)
}

func (m *mockInventoryRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inv = value
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

	t.Run("successful claim", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 5}
		inv, _ := coreinventory.New("char-1")

		charRepo := &mockCharacterRepo{char: char}
		invRepo := &mockInventoryRepo{inv: inv}

		svc, err := medal.NewService(charRepo, invRepo, nil, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		updatedChar, updatedInv, err := svc.Claim(context.Background(), "char-1", "armor-32")
		if err != nil {
			t.Fatal(err)
		}

		if updatedChar.SmallMedals != 2 {
			t.Errorf("expected 2 medals, got %d", updatedChar.SmallMedals)
		}

		if len(updatedInv.Items) != 1 {
			t.Errorf("expected 1 item, got %d", len(updatedInv.Items))
		}
	})

	t.Run("insufficient medals", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 2}
		inv, _ := coreinventory.New("char-1")

		charRepo := &mockCharacterRepo{char: char}
		invRepo := &mockInventoryRepo{inv: inv}

		svc, err := medal.NewService(charRepo, invRepo, nil, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.Claim(context.Background(), "char-1", "armor-32")
		if err != medal.ErrInsufficientMedals {
			t.Errorf("expected ErrInsufficientMedals, got %v", err)
		}
	})

	t.Run("reward not found", func(t *testing.T) {
		char := corecharacter.Character{ID: "char-1", SmallMedals: 10}
		inv, _ := coreinventory.New("char-1")

		charRepo := &mockCharacterRepo{char: char}
		invRepo := &mockInventoryRepo{inv: inv}

		svc, err := medal.NewService(charRepo, invRepo, nil, rewardsFile)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.Claim(context.Background(), "char-1", "non-existent-item")
		if err != medal.ErrRewardNotFound {
			t.Errorf("expected ErrRewardNotFound, got %v", err)
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
	inv, _ := coreinventory.New("char-1")

	charRepo := &mockCharacterRepo{char: char}
	invRepo := &mockInventoryRepo{inv: inv}

	svc, err := medal.NewServiceWithRewards(
		charRepo,
		invRepo,
		nil,
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
