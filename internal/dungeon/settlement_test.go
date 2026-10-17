package dungeon_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type errorInjectingDungeonRepo struct {
	*mockDungeonRepo
	errGetRecord          error
	errFinalizeExpedition error
}

func (e *errorInjectingDungeonRepo) GetRecord(ctx context.Context, characterID string) (dungeon.CharacterDungeonRecord, error) {
	if e.errGetRecord != nil {
		return dungeon.CharacterDungeonRecord{}, e.errGetRecord
	}
	return e.mockDungeonRepo.GetRecord(ctx, characterID)
}

func (e *errorInjectingDungeonRepo) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
) error {
	if e.errFinalizeExpedition != nil {
		return e.errFinalizeExpedition
	}
	return e.mockDungeonRepo.FinalizeExpedition(ctx, history, record, character, rewardItems)
}

func customSettlementDungeons() []dungeon.Dungeon {
	return []dungeon.Dungeon{
		{
			ID:               "test-dg-01",
			Name:             "テストダンジョン",
			MinLevel:         1,
			Tier:             1,
			MaxTurnsPerFloor: 25,
			ClearExpBonus:    100,
			ClearGoldBonus:   200,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      2,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S00",
						"D00",
					},
				},
				{
					FloorNumber: 2,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"SBD",
					},
					Boss: &dungeon.DungeonMonster{
						ID:         "floor-boss",
						Name:       "階層ボス",
						HP:         40,
						Attack:     10,
						Defense:    5,
						ExpReward:  150,
						GoldReward: 300,
						DropItemID: "boss-orb",
					},
				},
			},
		},
		{
			ID:               "test-dg-bossless",
			Name:             "ボス不在迷宮",
			MinLevel:         1,
			Tier:             2,
			MaxTurnsPerFloor: 10,
			ClearExpBonus:    50,
			ClearGoldBonus:   50,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"SBD",
					},
					Boss:     nil,
					Monsters: nil,
				},
			},
		},
	}
}

func setupSettlementTest(t *testing.T) (*errorInjectingDungeonRepo, *mockCharRepo, dungeon.ActiveExpeditionStore, *dungeon.Service) {
	t.Helper()
	repo := &errorInjectingDungeonRepo{mockDungeonRepo: newMockDungeonRepo()}
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": createTestChar("hero", 10, 100, 50, 30),
			"weak": createTestChar("weak", 1, 10, 5, 5),
		},
	}
	activeStore := dungeon.NewMemoryExpeditionRepository()
	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithActiveExpeditionStore(activeStore),
		dungeon.WithCustomDungeons(customSettlementDungeons()),
	)
	if err != nil {
		t.Fatalf("failed to create dungeon service: %v", err)
	}
	return repo, charRepo, activeStore, service
}

func TestEscape_InputAndStatusValidations(t *testing.T) {
	ctx := context.Background()
	_, _, _, service := setupSettlementTest(t)

	// 1. Empty character ID
	_, err := service.Escape(ctx, "")
	if !errors.Is(err, dungeon.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}

	// 2. No active expedition
	_, err = service.Escape(ctx, "hero")
	if !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Errorf("expected ErrNoActiveExpedition, got %v", err)
	}

	// 3. Active expedition not in StatusExploring
	_, _, activeStore2, service2 := setupSettlementTest(t)
	now := time.Now().UTC()
	_ = activeStore2.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		CharacterID: "hero",
		DungeonID:   "test-dg-01",
		Status:      dungeon.StatusCleared,
		StartedAt:   now,
		UpdatedAt:   now,
	})
	_, err = service2.Escape(ctx, "hero")
	if !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Errorf("expected ErrNoActiveExpedition when status is cleared, got %v", err)
	}

	// 4. Character not found in charRepo
	_ = activeStore2.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		CharacterID: "unknown-char",
		DungeonID:   "test-dg-01",
		Status:      dungeon.StatusExploring,
		StartedAt:   now,
		UpdatedAt:   now,
	})
	_, err = service2.Escape(ctx, "unknown-char")
	if !errors.Is(err, dungeon.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound for missing char, got %v", err)
	}
}

func TestEscape_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("experience application error propagates", func(t *testing.T) {
		_, charRepo, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()
		badChar := createTestChar("invalid_char", 0, 100, 50, 30)
		charRepo.chars["invalid_char"] = badChar

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID:    "invalid_char",
			DungeonID:      "test-dg-01",
			Status:         dungeon.StatusExploring,
			AccumulatedExp: 50,
			StartedAt:      now,
			UpdatedAt:      now,
		})

		_, err := service.Escape(ctx, "invalid_char")
		if err == nil || !strings.Contains(err.Error(), "applying experience") {
			t.Fatalf("expected applying experience error, got: %v", err)
		}
	})

	t.Run("currency addition error propagates", func(t *testing.T) {
		_, _, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID:     "hero",
			DungeonID:       "test-dg-01",
			Status:          dungeon.StatusExploring,
			AccumulatedGold: -1,
			StartedAt:       now,
			UpdatedAt:       now,
		})

		_, err := service.Escape(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "adding gold") {
			t.Fatalf("expected adding gold error, got: %v", err)
		}
	})

	t.Run("small medals addition error propagates", func(t *testing.T) {
		_, _, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID:       "hero",
			DungeonID:         "test-dg-01",
			Status:            dungeon.StatusExploring,
			AccumulatedMedals: -1,
			StartedAt:         now,
			UpdatedAt:         now,
		})

		_, err := service.Escape(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "adding small medals") {
			t.Fatalf("expected adding small medals error, got: %v", err)
		}
	})

	t.Run("get record repository error propagates", func(t *testing.T) {
		repo, _, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()
		repo.errGetRecord = errors.New("simulated db connection timeout on GetRecord")

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID: "hero",
			DungeonID:   "test-dg-01",
			Status:      dungeon.StatusExploring,
			StartedAt:   now,
			UpdatedAt:   now,
		})

		_, err := service.Escape(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "getting dungeon record") {
			t.Fatalf("expected getting dungeon record error, got: %v", err)
		}
	})

	t.Run("finalize expedition repository error propagates", func(t *testing.T) {
		repo, _, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()
		repo.errFinalizeExpedition = errors.New("simulated transaction abort on FinalizeExpedition")

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID: "hero",
			DungeonID:   "test-dg-01",
			Status:      dungeon.StatusExploring,
			StartedAt:   now,
			UpdatedAt:   now,
		})

		_, err := service.Escape(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "simulated transaction abort") {
			t.Fatalf("expected finalize expedition error, got: %v", err)
		}
	})

	t.Run("successful escape transfers rewards and clears active expedition", func(t *testing.T) {
		repo, charRepo, activeStore, service := setupSettlementTest(t)
		now := time.Now().UTC()

		_ = activeStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			CharacterID:       "hero",
			DungeonID:         "test-dg-01",
			Status:            dungeon.StatusExploring,
			CurrentFloor:      3,
			AccumulatedExp:    120,
			AccumulatedGold:   350,
			AccumulatedMedals: 2,
			AccumulatedItems:  []string{"item-pot-01"},
			StartedAt:         now,
			UpdatedAt:         now,
		})

		res, err := service.Escape(ctx, "hero")
		if err != nil {
			t.Fatalf("expected successful escape, got error: %v", err)
		}

		if !res.IsFinished || res.EventType != dungeon.EventEscape {
			t.Errorf("unexpected escape event result: %+v", res)
		}
		if res.ExpEarned != 120 || res.GoldFound != 350 || res.MedalsFound != 2 {
			t.Errorf("rewards mismatch: %+v", res)
		}

		// Verify character received gold and medals
		savedChar := repo.savedChars["hero"]
		if savedChar.Money != charRepo.chars["hero"].Money+350 {
			t.Errorf("expected gold %d, got %d", charRepo.chars["hero"].Money+350, savedChar.Money)
		}
		if savedChar.SmallMedals != charRepo.chars["hero"].SmallMedals+2 {
			t.Errorf("expected medals %d, got %d", charRepo.chars["hero"].SmallMedals+2, savedChar.SmallMedals)
		}

		// Verify record updated
		rec, _ := repo.GetRecord(ctx, "hero")
		if rec.TotalExpeditions != 1 || rec.TotalFloorsCleared != 3 {
			t.Errorf("record mismatch: %+v", rec)
		}

		// Verify active expedition deleted from store
		active, _ := activeStore.GetActiveExpedition(ctx, "hero")
		if active != nil {
			t.Errorf("expected active expedition purged, got %+v", active)
		}
	})
}

func TestDungeonClear_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("experience error during clear propagates", func(t *testing.T) {
		_, charRepo, _, service := setupSettlementTest(t)

		_, err := service.StartExpedition(ctx, "hero", "test-dg-bossless")
		if err != nil {
			t.Fatal(err)
		}
		// Mutate hero in charRepo to invalid level so progression.ApplyExperience fails on clear
		badChar := charRepo.chars["hero"]
		badChar.Level = 0
		charRepo.chars["hero"] = badChar

		_, err = service.Move(ctx, "hero", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "applying experience") {
			t.Fatalf("expected applying experience error on clear, got: %v", err)
		}
	})

	t.Run("currency error during clear propagates", func(t *testing.T) {
		_, _, activeStore, service := setupSettlementTest(t)

		exp, err := service.StartExpedition(ctx, "hero", "test-dg-bossless")
		if err != nil {
			t.Fatal(err)
		}
		// Set negative gold so total reward is negative (< 0)
		exp.AccumulatedGold = -1000
		_ = activeStore.SaveActiveExpedition(ctx, *exp)

		_, err = service.Move(ctx, "hero", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "adding gold") {
			t.Fatalf("expected adding gold error on clear, got: %v", err)
		}
	})

	t.Run("get record error during clear propagates", func(t *testing.T) {
		repo, _, _, service := setupSettlementTest(t)

		_, err := service.StartExpedition(ctx, "hero", "test-dg-bossless")
		if err != nil {
			t.Fatal(err)
		}
		repo.errGetRecord = errors.New("simulated db error on clear GetRecord")

		_, err = service.Move(ctx, "hero", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "getting dungeon record") {
			t.Fatalf("expected getting dungeon record error on clear, got: %v", err)
		}
	})

	t.Run("finalize expedition error during clear propagates", func(t *testing.T) {
		repo, _, _, service := setupSettlementTest(t)

		_, err := service.StartExpedition(ctx, "hero", "test-dg-bossless")
		if err != nil {
			t.Fatal(err)
		}
		repo.errFinalizeExpedition = errors.New("simulated db error on clear FinalizeExpedition")

		_, err = service.Move(ctx, "hero", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "simulated db error on clear FinalizeExpedition") {
			t.Fatalf("expected finalize expedition error on clear, got: %v", err)
		}
	})
}

func TestWipeout_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("get record error during wipeout propagates", func(t *testing.T) {
		repo, _, _, service := setupSettlementTest(t)

		// Start on test-dg-01 with weak character
		_, err := service.StartExpedition(ctx, "weak", "test-dg-01")
		if err != nil {
			t.Fatal(err)
		}
		// Move down stairs to 2F
		_, _ = service.Move(ctx, "weak", dungeon.DirectionSouth)

		// Inject DB error before boss wipeout
		repo.errGetRecord = errors.New("simulated db error on wipeout GetRecord")

		// Move East to fight boss and trigger wipeout
		_, err = service.Move(ctx, "weak", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "getting dungeon record") {
			t.Fatalf("expected getting dungeon record error on wipeout, got: %v", err)
		}
	})

	t.Run("finalize expedition error during wipeout propagates", func(t *testing.T) {
		repo, _, _, service := setupSettlementTest(t)

		_, err := service.StartExpedition(ctx, "weak", "test-dg-01")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = service.Move(ctx, "weak", dungeon.DirectionSouth)

		repo.errFinalizeExpedition = errors.New("simulated db error on wipeout FinalizeExpedition")

		_, err = service.Move(ctx, "weak", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "simulated db error on wipeout FinalizeExpedition") {
			t.Fatalf("expected finalize expedition error on wipeout, got: %v", err)
		}
	})
}
