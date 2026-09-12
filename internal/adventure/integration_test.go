package adventure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/tavern"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func TestAdventure_ImmediateDungeonCrawlIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	characterService, err := character.NewService(characters)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := characterService.Create(ctx, player.ID, "Dungeon Crawler")
	if err != nil {
		t.Fatal(err)
	}
	char.Stats.HP = 200
	char.Stats.MaxHP = 200
	char.Stats.Attack = 100
	char.Stats.Defense = 50
	char.Stats.Agility = 50
	if err := characters.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	clock := &fixedClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := adventure.NewServiceWithClock(adventures, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	adv, err := service.StartStage(ctx, char.ID, "stage-01")
	if err != nil {
		t.Fatalf("StartStage failed: %v", err)
	}

	if !adv.Resolved {
		t.Fatalf("expected adventure to be resolved immediately, got resolved=false")
	}
	if adv.FloorsCleared == 0 {
		t.Fatalf("expected floors cleared > 0, got %d", adv.FloorsCleared)
	}
	if adv.PartySize != 1 {
		t.Fatalf("expected party size 1, got %d", adv.PartySize)
	}

	// Verify DB record matches migration 073 schema
	saved, err := adventures.FindByID(ctx, adv.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if saved.ID != adv.ID || saved.FloorsCleared != adv.FloorsCleared || saved.PartySize != 1 {
		t.Fatalf("saved adventure mismatch: %+v", saved)
	}
}

func TestAdventureHistoryAndChronicleIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	characterService, err := character.NewService(characters)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := characterService.Create(ctx, player.ID, "Chronicle Hero")
	if err != nil {
		t.Fatal(err)
	}
	char.Stats.HP = 200
	char.Stats.MaxHP = 200
	char.Stats.Attack = 100
	char.Stats.Defense = 50
	char.Stats.Agility = 50
	if err := characters.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	clock := &fixedClock{now: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := adventure.NewServiceWithClock(adventures, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	// Start 2 adventures (immediately resolved)
	adv1, err := service.StartStage(ctx, char.ID, "stage-01")
	if err != nil {
		t.Fatalf("StartStage(stage-01) error = %v", err)
	}

	clock.now = clock.now.Add(time.Minute)
	adv2, err := service.StartStage(ctx, char.ID, "stage-01")
	if err != nil {
		t.Fatalf("StartStage(stage-01) error = %v", err)
	}

	// Query paginated history
	history, err := service.ListHistory(ctx, char.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListHistory error = %v", err)
	}
	if history.Total != 2 || len(history.Items) != 2 {
		t.Fatalf("expected total 2 and 2 items, got total=%d, len=%d", history.Total, len(history.Items))
	}
	if history.Items[0].ID != adv2.ID || history.Items[1].ID != adv1.ID {
		t.Fatalf("unexpected ordering: first=%s, second=%s", history.Items[0].ID, history.Items[1].ID)
	}

	// Query chronicle
	chronicle, err := service.GetChronicle(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetChronicle error = %v", err)
	}
	if chronicle.TotalAdventures != 2 {
		t.Fatalf("expected 2 adventures, got %d", chronicle.TotalAdventures)
	}
	if len(chronicle.Stages) == 0 || chronicle.Stages[0].StageID != "stage-01" {
		t.Fatalf("unexpected stage stats: %+v", chronicle.Stages)
	}
	if len(chronicle.Milestones) == 0 {
		t.Fatal("expected milestones to be populated")
	}
}

func TestAdventure_TavernDelivery_PostAdventureIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	tavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	lotteryRepo, err := database.NewLotteryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernService, err := tavern.NewService(
		tavernCatalog,
		tavernRepo,
		charRepo,
		txProvider,
		tavern.WithLotteryRepository(lotteryRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create test character with 5000G
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "TavernAdvHero", 5000)
	if err != nil {
		t.Fatal(err)
	}

	// Reduce character HP and MP to verify healing
	char.Stats.HP = 20
	char.Stats.MaxHP = 100
	char.Stats.MP = 5
	char.Stats.MaxMP = 50
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// 1. Reserve tavern delivery for omelet rice (Price: 750, HPHeal: 500, MPHeal: 100, Tickets: 7)
	deliv, err := tavernService.ReserveDelivery(ctx, char.ID, "tavern_omelet_rice")
	if err != nil {
		t.Fatalf("ReserveDelivery failed: %v", err)
	}
	if deliv.ItemID != "tavern_omelet_rice" {
		t.Fatalf("expected tavern_omelet_rice, got %s", deliv.ItemID)
	}

	// 2. Setup Adventure Service with PostAdventureHook wired to tavern ClaimDelivery
	clock := &fixedClock{now: time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	advService, err := adventure.NewServiceWithCatalogs(
		adventures,
		charRepo,
		invRepo,
		stages,
		monsters,
		corebattle.Engine{},
		nil,
		nil,
		clock,
	)
	if err != nil {
		t.Fatal(err)
	}

	advService.SetPostAdventureHook(func(hookCtx context.Context, characterID string) error {
		_, err := tavernService.ClaimDelivery(hookCtx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
			return nil
		}
		return err
	})

	// 3. Start adventure (immediate execution + PostAdventureHook)
	claimedAdv, err := advService.StartStage(ctx, char.ID, "stage-01")
	if err != nil {
		t.Fatalf("StartStage failed: %v", err)
	}
	if !claimedAdv.Resolved {
		t.Fatal("expected adventure to be resolved")
	}

	// 4. Verify Post-Adventure delivery effects
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	// HP and MP should be restored to MaxHP (100) and MaxMP (50)
	if updatedChar.Stats.HP != 100 {
		t.Errorf("expected HP 100, got %d", updatedChar.Stats.HP)
	}
	if updatedChar.Stats.MP != 50 {
		t.Errorf("expected MP 50, got %d", updatedChar.Stats.MP)
	}

	// Gold should reflect adventure reward minus meal cost (750G)
	expectedGold := 5000 - 750 + claimedAdv.BattleResult.Reward.Currency
	if updatedChar.Money != expectedGold {
		t.Errorf("expected Gold %d, got %d", expectedGold, updatedChar.Money)
	}

	// Delivery reservation should be consumed
	_, err = tavernService.GetDelivery(ctx, char.ID)
	if !errors.Is(err, tavern.ErrNoActiveDelivery) {
		t.Errorf("expected ErrNoActiveDelivery after post-adventure delivery, got %v", err)
	}

	// Raffle tickets should have been awarded
	tickets, err := lotteryRepo.GetRaffleTickets(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tickets != 7 {
		t.Errorf("expected 7 raffle tickets, got %d", tickets)
	}
}
