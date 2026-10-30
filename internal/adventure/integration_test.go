package adventure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/character"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/tavern"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

type failingSaveDepotRepo struct {
	depot.Repository
}

func (r *failingSaveDepotRepo) Save(ctx context.Context, dep depot.Depot) error {
	return errors.New("simulated delivery persistence error")
}

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
	char, err := characterService.Create(ctx, player.ID, "DungCrwl")
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

	adv, err := service.StartStage(ctx, char.ID, adventure.StarterAdventure)
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
	char, err := characterService.Create(ctx, player.ID, "ChrnHero")
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
	adv1, err := service.StartStage(ctx, char.ID, adventure.StarterAdventure)
	if err != nil {
		t.Fatalf("StartStage(StarterAdventure) error = %v", err)
	}

	clock.now = clock.now.Add(time.Minute)
	adv2, err := service.StartStage(ctx, char.ID, adventure.StarterAdventure)
	if err != nil {
		t.Fatalf("StartStage(StarterAdventure) error = %v", err)
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
	if len(chronicle.Stages) == 0 || chronicle.Stages[0].StageID != adventure.StarterAdventure {
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

	// 1. Order meal at tavern counter first so character is full before adventure
	_, err = tavernService.OrderMeal(ctx, char.ID, "tavern_water")
	if err != nil {
		t.Fatalf("OrderMeal failed: %v", err)
	}
	preStatus, err := tavernService.GetStatus(ctx, char.ID)
	if err != nil || !preStatus.IsFull {
		t.Fatalf("expected character to be full before adventure, got %v, err: %v", preStatus.IsFull, err)
	}

	// 2. Reserve tavern delivery for omelet rice (Price: 750, HPHeal: 500, MPHeal: 100, Tickets: 7)
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
		_ = tavernService.ResetFullness(hookCtx, characterID)
		_, err := tavernService.ClaimDelivery(hookCtx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
			return nil
		}
		return err
	})

	// 3. Start adventure (immediate execution + PostAdventureHook)
	claimedAdv, err := advService.StartStage(ctx, char.ID, adventure.StarterAdventure)
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

	// Gold should reflect adventure reward minus counter meal (20G) and delivery meal cost (750G)
	expectedGold := 5000 - 20 - 750 + claimedAdv.BattleResult.Reward.Currency
	if updatedChar.Money != expectedGold {
		t.Errorf("expected Gold %d, got %d", expectedGold, updatedChar.Money)
	}

	// Delivery reservation should remain active (standing order / recurring contract)
	delivAfter, err := tavernService.GetDelivery(ctx, char.ID)
	if err != nil {
		t.Errorf("expected delivery reservation to remain active, got %v", err)
	} else if delivAfter.ItemID != "tavern_omelet_rice" {
		t.Errorf("expected delivery item tavern_omelet_rice, got %s", delivAfter.ItemID)
	}

	// Raffle tickets: 1 from initial counter meal, 0 added from delivery meals
	tickets, err := lotteryRepo.GetRaffleTickets(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tickets != 1 {
		t.Errorf("expected 1 raffle ticket from counter meal, got %d", tickets)
	}

	// 5. Verify Post-Adventure fullness reset (is_eat = 0)
	postStatus, err := tavernService.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if postStatus.IsFull {
		t.Errorf("expected IsFull to be false after adventure, got true")
	}

	// Character can immediately dine at the tavern counter again
	_, err = tavernService.OrderMeal(ctx, char.ID, "tavern_water")
	if err != nil {
		t.Errorf("expected character to be able to dine immediately after adventure, got %v", err)
	}
}

func TestAdventurePostAdventureHook_TavernDelivery_ZeroFundsLifecycle(t *testing.T) {
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

	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernService, err := tavern.NewService(
		tavernCatalog,
		tavernRepo,
		charRepo,
		txProvider,
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create test character with 0G
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "TavernAdvZeroHero", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Reduce character HP and MP to verify healing conditions
	char.Stats.HP = 20
	char.Stats.MaxHP = 100
	char.Stats.MP = 5
	char.Stats.MaxMP = 50
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// 2. Reserve delivery for full course (Price: 3000) with 0G -> succeeds!
	delivFull, err := tavernService.ReserveDelivery(ctx, char.ID, "tavern_full_course")
	if err != nil {
		t.Fatalf("ReserveDelivery for full course with 0G failed: %v", err)
	}
	if delivFull.ItemID != "tavern_full_course" {
		t.Fatalf("expected tavern_full_course, got %s", delivFull.ItemID)
	}

	// 3. Overwrite / replace delivery with omelet rice (Price: 750) with 0G -> succeeds!
	deliv, err := tavernService.ReserveDelivery(ctx, char.ID, "tavern_omelet_rice")
	if err != nil {
		t.Fatalf("ReserveDelivery for omelet rice with 0G failed: %v", err)
	}
	if deliv.ItemID != "tavern_omelet_rice" {
		t.Fatalf("expected tavern_omelet_rice, got %s", deliv.ItemID)
	}

	// Ensure gold remains 0G after reservations
	charCheck, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if charCheck.Money != 0 {
		t.Fatalf("expected 0G intact after reservations, got %d", charCheck.Money)
	}

	// 4. Setup Adventure Service with PostAdventureHook wired to tavern ClaimDelivery
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
		_ = tavernService.ResetFullness(hookCtx, characterID)
		_, err := tavernService.ClaimDelivery(hookCtx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
			return nil
		}
		return err
	})

	// 5. Start adventure: character finishes with small reward (< 750G)
	claimedAdv, err := advService.StartStage(ctx, char.ID, adventure.StarterAdventure)
	if err != nil {
		t.Fatalf("StartStage failed: %v", err)
	}
	if !claimedAdv.Resolved {
		t.Fatal("expected adventure to be resolved")
	}

	// Verify post-adventure state: delivery was skipped due to insufficient funds, but reservation remains
	charAfterFirstAdv, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Money reflects only battle reward (not deducted for delivery)
	if charAfterFirstAdv.Money != claimedAdv.BattleResult.Reward.Currency {
		t.Errorf("expected Money %d, got %d", claimedAdv.BattleResult.Reward.Currency, charAfterFirstAdv.Money)
	}
	// Delivery reservation remains active in DB
	delivStillActive, err := tavernService.GetDelivery(ctx, char.ID)
	if err != nil {
		t.Fatalf("expected delivery reservation to remain active after insufficient funds skip, got %v", err)
	}
	if delivStillActive.ItemID != "tavern_omelet_rice" {
		t.Errorf("expected delivery item tavern_omelet_rice, got %s", delivStillActive.ItemID)
	}

	// 6. Character earns sufficient funds (e.g. granted 2000G)
	charAfterFirstAdv.Money += 2000
	if err := charRepo.Update(ctx, charAfterFirstAdv); err != nil {
		t.Fatal(err)
	}

	// 7. Start second adventure: character now has sufficient funds for delivery
	claimedAdv2, err := advService.StartStage(ctx, char.ID, adventure.StarterAdventure)
	if err != nil {
		t.Fatalf("second StartStage failed: %v", err)
	}
	if !claimedAdv2.Resolved {
		t.Fatal("expected second adventure to be resolved")
	}

	// Verify delivery was successfully claimed and deducted
	charFinal, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedFinalGold := charAfterFirstAdv.Money + claimedAdv2.BattleResult.Reward.Currency - 750
	if charFinal.Money != expectedFinalGold {
		t.Errorf("expected Final Gold %d, got %d", expectedFinalGold, charFinal.Money)
	}
	if charFinal.Stats.HP != 100 {
		t.Errorf("expected HP 100 healed by delivery, got %d", charFinal.Stats.HP)
	}
	if charFinal.Stats.MP != 50 {
		t.Errorf("expected MP 50 healed by delivery, got %d", charFinal.Stats.MP)
	}

	// Delivery reservation remains active for future adventures (standing order)
	delivFinal, err := tavernService.GetDelivery(ctx, char.ID)
	if err != nil {
		t.Fatalf("expected delivery reservation to remain active after successful claim, got %v", err)
	}
	if delivFinal.ItemID != "tavern_omelet_rice" {
		t.Errorf("expected delivery item tavern_omelet_rice, got %s", delivFinal.ItemID)
	}
}

type settlementBattleEngine struct {
	corebattle.Engine
	consumeOnce map[string][]corebattle.ConsumedItem
}

func (e *settlementBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	res, err := e.Engine.ResolvePartyBattle(req)
	if err != nil {
		return res, err
	}
	if len(e.consumeOnce) > 0 {
		res.ConsumedItems = e.consumeOnce
		e.consumeOnce = nil
	}
	return res, nil
}

func TestAdventurePostBattleSettlement_Integration(t *testing.T) {
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
	characterService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := characterService.Create(ctx, player.ID, "SetlHero")
	if err != nil {
		t.Fatal(err)
	}
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	char.Stats.MP = 100
	char.Stats.MaxMP = 100
	char.Stats.Attack = 200
	char.Stats.Defense = 100
	char.Stats.Agility = 100
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	deterministicRng := func(n int) int { return 0 }
	battleEngine := &settlementBattleEngine{}

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(battleEngine),
		battle.WithTransactionProvider(txProvider),
	)

	clock := &fixedClock{now: time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC)}
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	advService, err := adventure.NewServiceWithCatalogs(
		advRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		battleEngine,
		nil,
		nil,
		clock,
		adventure.WithParticipantBuilder(battleAdapter),
		adventure.WithPostBattleSettler(battleAdapter),
		adventure.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. First run: Empty inventory -> Floor 11 treasure item saved to MariaDB inventory_items
	crawlRes, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      adventure.StarterAdventure,
		Rng:          deterministicRng,
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}
	if !crawlRes.StageCleared {
		t.Fatalf("expected stage cleared, got outcome: %v", crawlRes.Outcome)
	}
	if len(crawlRes.TreasureBoxes) == 0 {
		t.Fatal("expected Floor 11 treasure boxes")
	}
	if crawlRes.TreasureBoxes[0].DeliveredTo != "inventory" {
		t.Errorf("expected box delivered to inventory, got %s", crawlRes.TreasureBoxes[0].DeliveredTo)
	}

	// Verify inventory in MariaDB
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) == 0 {
		t.Fatal("expected items in DB inventory, got 0")
	}

	// Verify character HP, MP & exit fatigue in MariaDB
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if updatedChar.Stats.HP <= 0 || updatedChar.Stats.HP > updatedChar.Stats.MaxHP {
		t.Errorf("unexpected HP in DB: %d", updatedChar.Stats.HP)
	}
	if updatedChar.Tired != 34 {
		t.Errorf("expected Tired=34 after 1st Floor 11 clear in DB, got %d", updatedChar.Tired)
	}

	// 2. Second run: Inventory full -> Overflow delivered to depot_items in MariaDB
	crawlRes2, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      adventure.StarterAdventure,
		Rng:          deterministicRng,
	})
	if err != nil {
		t.Fatalf("Second ExecuteCrawl failed: %v", err)
	}
	if len(crawlRes2.TreasureBoxes) == 0 {
		t.Fatal("expected treasure boxes on second crawl")
	}
	if crawlRes2.TreasureBoxes[0].DeliveredTo != "depot" {
		t.Errorf("expected box delivered to depot, got %s", crawlRes2.TreasureBoxes[0].DeliveredTo)
	}

	updatedChar2, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByID after 2nd crawl failed: %v", err)
	}
	if updatedChar2.Tired != 68 {
		t.Errorf("expected Tired=68 after 2nd Floor 11 clear in DB, got %d", updatedChar2.Tired)
	}

	// Verify depot in MariaDB
	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID for depot failed: %v", err)
	}
	if len(dep.Items) == 0 {
		t.Fatal("expected items in DB depot, got 0")
	}

	// 3. Defeat verification: A weak character defeated in crawl survives with 1 HP and exit fatigue in MariaDB
	weakling, err := characterService.Create(ctx, player.ID, "DftdHero")
	if err != nil {
		t.Fatal(err)
	}
	weakling.Stats.HP = 10
	weakling.Stats.MaxHP = 10
	weakling.Stats.Attack = 1
	weakling.Stats.Defense = 1
	if err := charRepo.Update(ctx, weakling); err != nil {
		t.Fatal(err)
	}

	crawlRes3, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{weakling.ID},
		StageID:      adventure.StarterAdventure,
		Rng:          deterministicRng,
	})
	if err != nil {
		t.Fatalf("Third ExecuteCrawl failed: %v", err)
	}
	if crawlRes3.Outcome != corebattle.OutcomeDefeat {
		t.Fatalf("expected defeat outcome, got %v", crawlRes3.Outcome)
	}

	savedWeakling, err := charRepo.FindByID(ctx, weakling.ID)
	if err != nil {
		t.Fatalf("FindByID for defeated hero failed: %v", err)
	}
	if savedWeakling.Stats.HP != 1 {
		t.Errorf("expected defeated hero HP=1 in MariaDB, got %d", savedWeakling.Stats.HP)
	}
	expectedDefeatTired := adventure.CalculateExitFatigue(crawlRes3.Round, crawlRes3.Mode)
	if savedWeakling.Tired != expectedDefeatTired {
		t.Errorf("expected defeated hero Tired=%d (Round %d) in MariaDB, got %d", expectedDefeatTired, crawlRes3.Round, savedWeakling.Tired)
	}

	// 4. Consumption frees inventory hand: Character starts with 1 item in inventory, consumes it in combat,
	// and Floor 11 treasure is delivered to inventory (freed hand) rather than depot.
	consumerChar, err := characterService.Create(ctx, player.ID, "ConsHero")
	if err != nil {
		t.Fatal(err)
	}
	consumerChar.Stats.HP = 500
	consumerChar.Stats.MaxHP = 500
	consumerChar.Stats.MP = 100
	consumerChar.Stats.MaxMP = 100
	consumerChar.Stats.Attack = 200
	consumerChar.Stats.Defense = 100
	consumerChar.Stats.Agility = 100
	if err := charRepo.Update(ctx, consumerChar); err != nil {
		t.Fatal(err)
	}

	consumedHerb, err := coreitem.NewInstance("item-001", 1)
	if err != nil {
		t.Fatal(err)
	}
	consumerInv, err := invRepo.FindByCharacterID(ctx, consumerChar.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID for consumer hero failed: %v", err)
	}
	if err := consumerInv.Add(consumedHerb); err != nil {
		t.Fatalf("consumerInv.Add failed: %v", err)
	}
	if err := invRepo.Save(ctx, consumerInv); err != nil {
		t.Fatalf("invRepo.Save failed: %v", err)
	}

	// Arm the battle engine to report consumption of the held herb on combat resolution.
	battleEngine.consumeOnce = map[string][]corebattle.ConsumedItem{
		consumerChar.ID: {{
			ID:         consumedHerb.DefinitionID,
			InstanceID: consumedHerb.ID,
			Quantity:   1,
		}},
	}

	crawlRes4, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{consumerChar.ID},
		StageID:      adventure.StarterAdventure,
		Rng:          deterministicRng,
	})
	if err != nil {
		t.Fatalf("Fourth ExecuteCrawl failed: %v", err)
	}
	if !crawlRes4.StageCleared {
		t.Fatalf("expected stage cleared for consumer hero, got outcome: %v", crawlRes4.Outcome)
	}
	if len(crawlRes4.TreasureBoxes) == 0 {
		t.Fatal("expected Floor 11 treasure boxes on fourth crawl")
	}
	if crawlRes4.TreasureBoxes[0].DeliveredTo != "inventory" {
		t.Errorf("expected box delivered to inventory after consumption freed hand, got %s", crawlRes4.TreasureBoxes[0].DeliveredTo)
	}

	// Verify inventory in MariaDB: consumed item is debited, newly earned treasure is stored
	consumerInvAfter, err := invRepo.FindByCharacterID(ctx, consumerChar.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after consumption crawl failed: %v", err)
	}
	if len(consumerInvAfter.Items) != 1 {
		t.Fatalf("expected exactly 1 item in consumer hero DB inventory, got %d", len(consumerInvAfter.Items))
	}
	if consumerInvAfter.Items[0].ID == consumedHerb.ID {
		t.Errorf("expected consumed item %s to be debited from DB inventory", consumedHerb.ID)
	}
	if consumerInvAfter.Items[0].DefinitionID != crawlRes4.TreasureBoxes[0].ItemID {
		t.Errorf("expected delivered treasure %s in DB inventory, got %s", crawlRes4.TreasureBoxes[0].ItemID, consumerInvAfter.Items[0].DefinitionID)
	}

	// Verify depot in MariaDB: remains empty because consumption kept inventory below capacity
	consumerDepot, err := depotRepo.FindByCharacterID(ctx, consumerChar.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID for consumer depot failed: %v", err)
	}
	if len(consumerDepot.Items) != 0 {
		t.Errorf("expected 0 items in consumer DB depot, got %d", len(consumerDepot.Items))
	}
}

type failingSaveAdventureRepo struct {
	adventure.Repository
	failOnCall int
	calls      int
	onSave     func(ctx context.Context, value adventure.Adventure)
}

func (r *failingSaveAdventureRepo) Save(ctx context.Context, value adventure.Adventure) error {
	r.calls++
	if r.onSave != nil {
		r.onSave(ctx, value)
	}
	if r.failOnCall > 0 && r.calls == r.failOnCall {
		return errors.New("injected adventure history save failure")
	}
	return r.Repository.Save(ctx, value)
}

func TestAdventurePostBattleSettlement_AtomicHistoryAndHooks_Integration(t *testing.T) {
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
	characterService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	baseAdvRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	makeHero := func(name string) corecharacter.Character {
		c, err := characterService.Create(ctx, player.ID, name)
		if err != nil {
			t.Fatal(err)
		}
		c.Level = 10
		c.Stats.HP = 500
		c.Stats.MaxHP = 500
		c.Stats.MP = 100
		c.Stats.MaxMP = 100
		c.Stats.Attack = 200
		c.Stats.Defense = 100
		c.Stats.Agility = 100
		if err := charRepo.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	hero1 := makeHero("AtmSolo")
	hero2 := makeHero("AtmParty")

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(corebattle.Engine{}),
		battle.WithTransactionProvider(txProvider),
	)

	clock := &fixedClock{now: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)}
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	advRepo := &failingSaveAdventureRepo{Repository: baseAdvRepo}
	advService, err := adventure.NewServiceWithCatalogs(
		advRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		corebattle.Engine{},
		nil,
		nil,
		clock,
		adventure.WithParticipantBuilder(battleAdapter),
		adventure.WithPostBattleSettler(battleAdapter),
		adventure.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	var victoryHooks, postHooks int
	advService.SetVictoryHook(func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error {
		victoryHooks++
		return nil
	})
	advService.SetPostAdventureHook(func(ctx context.Context, characterID string) error {
		postHooks++
		return nil
	})

	// 1. Solo history save error rolls back rewards, exit fatigue, inventory drops, and chronicle
	advRepo.calls = 0
	advRepo.failOnCall = 1
	_, err = advService.StartStage(ctx, hero1.ID, adventure.StarterAdventure)
	if err == nil {
		t.Fatal("expected StartStage to fail when adventure Save fails")
	}
	if victoryHooks != 0 || postHooks != 0 {
		t.Fatalf("hooks must not run on rolled-back crawl: victory=%d post=%d", victoryHooks, postHooks)
	}

	afterFail1, err := charRepo.FindByID(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFail1.Money != hero1.Money || afterFail1.Experience != hero1.Experience || afterFail1.Level != hero1.Level || afterFail1.Tired != hero1.Tired {
		t.Fatalf("expected hero1 rewards/tired rolled back, before=(money:%d exp:%d lvl:%d tired:%d) after=(money:%d exp:%d lvl:%d tired:%d)",
			hero1.Money, hero1.Experience, hero1.Level, hero1.Tired, afterFail1.Money, afterFail1.Experience, afterFail1.Level, afterFail1.Tired)
	}
	invAfterFail1, err := invRepo.FindByCharacterID(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invAfterFail1.Items) != 0 {
		t.Fatalf("expected hero1 inventory drops rolled back, got %d items", len(invAfterFail1.Items))
	}
	chronAfterFail1, err := advService.GetChronicle(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chronAfterFail1.TotalAdventures != 0 || chronAfterFail1.TotalVictories != 0 {
		t.Fatalf("expected hero1 chronicle empty after rollback, got %+v", chronAfterFail1)
	}

	// 2. Multi-participant partial history save error (2nd participant fails) rolls back both participants' rewards, exit fatigue, and 1st participant's history
	advRepo.calls = 0
	advRepo.failOnCall = 2
	_, err = advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{hero1.ID, hero2.ID},
		StageID:      adventure.StarterAdventure,
	})
	if err == nil {
		t.Fatal("expected ExecuteCrawl to fail when 2nd participant adventure Save fails")
	}
	if victoryHooks != 0 || postHooks != 0 {
		t.Fatalf("hooks must not run on multi-participant rollback: victory=%d post=%d", victoryHooks, postHooks)
	}

	for _, h := range []corecharacter.Character{hero1, hero2} {
		reloaded, err := charRepo.FindByID(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Money != h.Money || reloaded.Experience != h.Experience || reloaded.Tired != h.Tired {
			t.Fatalf("expected %s rewards/tired rolled back, got money=%d exp=%d tired=%d", h.Name, reloaded.Money, reloaded.Experience, reloaded.Tired)
		}
		hist, err := advService.ListHistory(ctx, h.ID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if hist.Total != 0 || len(hist.Items) != 0 {
			t.Fatalf("expected %s history rolled back, got total=%d len=%d", h.Name, hist.Total, len(hist.Items))
		}
	}

	// 3. Success commits rewards, exit fatigue, and history together in the same transaction; post-commit hooks run outside tx and hook errors do not revert committed state
	advRepo.calls = 0
	advRepo.failOnCall = 0
	var outsideMoneyDuringSave int
	var hookSawCommittedHistory bool
	advRepo.onSave = func(_ context.Context, _ adventure.Adventure) {
		// Outside context (non-tx) must not see uncommitted reward changes before Save finishes and tx commits
		outsideChar, err := charRepo.FindByID(context.Background(), hero1.ID)
		if err == nil {
			outsideMoneyDuringSave = outsideChar.Money
		}
	}
	advService.SetVictoryHook(func(hookCtx context.Context, characterID string, _ int, _ int) error {
		victoryHooks++
		if database.TxFromContext(hookCtx) != nil {
			t.Errorf("expected victoryHook context to have no active transaction")
		}
		hist, err := advService.ListHistory(context.Background(), characterID, 10, 0)
		if err == nil && hist.Total == 1 {
			hookSawCommittedHistory = true
		}
		return errors.New("victory hook failure after commit")
	})
	advService.SetPostAdventureHook(func(hookCtx context.Context, _ string) error {
		postHooks++
		if database.TxFromContext(hookCtx) != nil {
			t.Errorf("expected postAdventureHook context to have no active transaction")
		}
		return errors.New("post adventure hook failure after commit")
	})

	adv, err := advService.StartStage(ctx, hero1.ID, adventure.StarterAdventure)
	if err != nil {
		t.Fatalf("expected StartStage to succeed even when post-commit hooks return errors, got: %v", err)
	}
	if outsideMoneyDuringSave != hero1.Money {
		t.Fatalf("expected rewards to be uncommitted during adventure Save, but outside reader saw money=%d (initial=%d)",
			outsideMoneyDuringSave, hero1.Money)
	}
	if !hookSawCommittedHistory || victoryHooks != 1 || postHooks != 1 {
		t.Fatalf("expected hooks to run after commit and observe committed history: sawHistory=%v victory=%d post=%d",
			hookSawCommittedHistory, victoryHooks, postHooks)
	}

	committedHero1, err := charRepo.FindByID(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committedHero1.Money <= hero1.Money {
		t.Fatalf("expected committed money > %d, got %d", hero1.Money, committedHero1.Money)
	}
	if committedHero1.Tired != hero1.Tired+34 {
		t.Fatalf("expected committed Tired=%d, got %d", hero1.Tired+34, committedHero1.Tired)
	}
	chronAfterCommit, err := advService.GetChronicle(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chronAfterCommit.TotalAdventures != 1 || chronAfterCommit.TotalVictories != 1 || !adv.IsCleared {
		t.Fatalf("expected committed chronicle and cleared adventure, got chron=%+v adv=%+v", chronAfterCommit, adv)
	}
}

func TestAdventure_TreasurePocket_RealDBSettlementAndRollback_Integration(t *testing.T) {
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
	characterService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := characterService.Create(ctx, player.ID, "TrsHero")
	if err != nil {
		t.Fatal(err)
	}
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	char.Stats.MP = 100
	char.Stats.MaxMP = 100
	char.Stats.Attack = 200
	char.Stats.Defense = 100
	char.Stats.Agility = 100
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// Add stack of 2 ItemTreasurePocket to character's inventory in real DB
	pocketInst, err := coreitem.NewInstance(adventure.ItemTreasurePocket, 2)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		inv, _ = coreinventory.New(char.ID)
	}
	inv.Items = append(inv.Items, pocketInst)
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	battleEngine := &settlementBattleEngine{}
	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(battleEngine),
		battle.WithTransactionProvider(txProvider),
	)

	// 1. Verify BuildParticipant expands stack quantity (Quantity 2 -> 2 ItemTreasurePocket entries)
	part, err := battleAdapter.BuildParticipant(ctx, char.ID)
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}
	var pocketCount int
	for _, id := range part.ItemDefinitionIDs {
		if id == adventure.ItemTreasurePocket {
			pocketCount++
		}
	}
	if pocketCount != 2 {
		t.Fatalf("expected 2 ItemTreasurePocket in ItemDefinitionIDs for stack quantity 2, got %d", pocketCount)
	}

	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()
	clock := &fixedClock{now: time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC)}

	advService, err := adventure.NewServiceWithCatalogs(
		advRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		battleEngine,
		nil,
		nil,
		clock,
		adventure.WithParticipantBuilder(battleAdapter),
		adventure.WithPostBattleSettler(battleAdapter),
		adventure.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Verify automatic crawl does NOT consume ItemTreasurePocket
	crawlRes, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      adventure.StarterAdventure,
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}
	if len(crawlRes.ConsumedItems[char.ID]) != 0 {
		t.Fatalf("expected 0 consumed items during auto-crawl, got: %v", crawlRes.ConsumedItems[char.ID])
	}
	// Verify MariaDB inventory still has quantity 2
	invAfterAuto, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pocketInDB *coreitem.Instance
	for i := range invAfterAuto.Items {
		if invAfterAuto.Items[i].DefinitionID == adventure.ItemTreasurePocket {
			pocketInDB = &invAfterAuto.Items[i]
			break
		}
	}
	if pocketInDB == nil || pocketInDB.Quantity != 2 {
		t.Fatalf("expected ItemTreasurePocket quantity 2 in DB after auto crawl, got: %+v", pocketInDB)
	}

	// 3. Verify manual/explicit Floor 11 inspection consumes keys from real DB
	stage17, _ := stages.FindByID("stage-17")
	sess, err := adventure.NewCrawlSessionWithParticipants(stage17, []corecharacter.Character{char}, []corebattle.Participant{part}, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	sess.CurrentFloor = adventure.TreasureRoomFloor
	sess.StageCleared = true
	sess.TreasureBoxes = []adventure.TreasureBox{
		{ID: "box-1", Name: "Chest 1", ItemID: "item-001"},
		{ID: "box-2", Name: "Chest 2", ItemID: "item-002"},
		{ID: "box-3", Name: "Chest 3", ItemID: "item-003"},
	}
	// 1st box: base allowance
	if _, err := sess.ExamineTreasure(char.ID); err != nil {
		t.Fatalf("box 1: %v", err)
	}
	// 2nd box: consumes 1st key
	if _, err := sess.ExamineTreasure(char.ID); err != nil {
		t.Fatalf("box 2: %v", err)
	}
	// 3rd box: consumes 2nd key
	if _, err := sess.ExamineTreasure(char.ID); err != nil {
		t.Fatalf("box 3: %v", err)
	}
	// 4th box attempt: rejected
	sess.TreasureBoxes = append(sess.TreasureBoxes, adventure.TreasureBox{ID: "box-4", Name: "Chest 4", ItemID: "item-004"})
	if _, err := sess.ExamineTreasure(char.ID); !errors.Is(err, adventure.ErrTreasureLimitReached) {
		t.Fatalf("box 4 should be rejected with ErrTreasureLimitReached, got %v", err)
	}

	// Apply settlement to real DB
	sessRes := sess.Result()
	settleReq := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:       sessRes.Outcome,
			Turns:         1,
			RemainingHP:   sessRes.ParticipantHPs,
			RemainingMP:   sessRes.ParticipantMPs,
			ConsumedItems: sessRes.ConsumedItems,
		},
		RecipientDrops: map[string][]string{
			char.ID: {"item-001", "item-002", "item-003"},
		},
	}
	if _, err := battleAdapter.ApplyPostBattleResult(ctx, settleReq); err != nil {
		t.Fatalf("settlement failed: %v", err)
	}
	// In real DB, the stack of 2 ItemTreasurePocket must be consumed (quantity drops from 2 to 0)
	invAfterSettle, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range invAfterSettle.Items {
		if it.DefinitionID == adventure.ItemTreasurePocket {
			t.Fatalf("expected ItemTreasurePocket to be completely consumed from DB, still found: %+v", it)
		}
	}

	// 4. Test atomic rollback in real DB:
	// Verify that if reward delivery fails AFTER key consumption succeeds,
	// the entire transaction rolls back and neither key consumption nor reward delivery is committed.
	// Step 1: Re-seed inventory with a stack of 2 ItemTreasurePocket in MariaDB.
	resetPocket, err := coreitem.NewInstance(adventure.ItemTreasurePocket, 2)
	if err != nil {
		t.Fatal(err)
	}
	invReset, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	invReset.Items = []coreitem.Instance{resetPocket}
	if err := invRepo.Save(ctx, invReset); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM depot_items WHERE character_id = ?", char.ID); err != nil {
		t.Fatal(err)
	}

	// Configure a battle adapter where reward delivery persistence fails in depot.
	failingDepot := &failingSaveDepotRepo{Repository: depotRepo}
	failingBattleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(failingDepot),
		battle.WithBattleEngine(battleEngine),
		battle.WithTransactionProvider(txProvider),
	)

	// Step 2 & 3: Include key consumption (1 ItemTreasurePocket) and treasure reward delivery
	// ("item-001") in the same settlement request.
	// Because char inventory already holds 1 item (ItemTreasurePocket, remaining quantity 1)
	// and max inventory capacity is 1, the reward drop overflows to depot delivery.
	// Key consumption succeeds first, but depot delivery fails when persisting the reward.
	failingSettleReq := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			Turns:       1,
			RemainingHP: map[string]int{char.ID: 100},
			RemainingMP: map[string]int{char.ID: 50},
			ConsumedItems: map[string][]corebattle.ConsumedItem{
				char.ID: {{ID: adventure.ItemTreasurePocket, Quantity: 1}},
			},
		},
		RecipientDrops: map[string][]string{
			char.ID: {"item-001"},
		},
	}
	if _, err := failingBattleAdapter.ApplyPostBattleResult(ctx, failingSettleReq); err == nil {
		t.Fatalf("expected error from reward delivery persistence failure, got nil")
	}

	// Step 4: Verify transaction rolled back completely in MariaDB:
	// - ItemTreasurePocket quantity MUST remain at 2 (the consumed key was restored).
	// - Reward drop "item-001" MUST NOT exist in inventory.
	// - No depot items were created.
	invAfterRollback, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pocketAfterRollback *coreitem.Instance
	for i := range invAfterRollback.Items {
		if invAfterRollback.Items[i].DefinitionID == adventure.ItemTreasurePocket {
			pocketAfterRollback = &invAfterRollback.Items[i]
			break
		}
	}
	if pocketAfterRollback == nil || pocketAfterRollback.Quantity != 2 {
		t.Fatalf("expected ItemTreasurePocket quantity 2 in DB after rollback, got: %+v", pocketAfterRollback)
	}
	for _, it := range invAfterRollback.Items {
		if it.DefinitionID == "item-001" {
			t.Fatalf("reward item-001 was committed to inventory despite delivery failure")
		}
	}
	var depotItemCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM depot_items WHERE character_id = ?", char.ID).Scan(&depotItemCount); err != nil {
		t.Fatal(err)
	}
	if depotItemCount != 0 {
		t.Fatalf("expected 0 depot items after rollback, got %d", depotItemCount)
	}
}
