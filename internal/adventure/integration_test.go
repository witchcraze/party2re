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
	char, err := characterService.Create(ctx, player.ID, "DungeonCrawler")
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
	char, err := characterService.Create(ctx, player.ID, "ChronicleHero")
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

	char, err := characterService.Create(ctx, player.ID, "SettlerHero")
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

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithBattleEngine(corebattle.Engine{}),
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

	// 1. First run: Empty inventory -> Floor 11 treasure item saved to MariaDB inventory_items
	crawlRes, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      adventure.StarterAdventure,
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

	// Verify character HP & MP in MariaDB
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if updatedChar.Stats.HP <= 0 || updatedChar.Stats.HP > updatedChar.Stats.MaxHP {
		t.Errorf("unexpected HP in DB: %d", updatedChar.Stats.HP)
	}

	// 2. Second run: Inventory full -> Overflow delivered to depot_items in MariaDB
	crawlRes2, err := advService.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      adventure.StarterAdventure,
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

	// Verify depot in MariaDB
	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID for depot failed: %v", err)
	}
	if len(dep.Items) == 0 {
		t.Fatal("expected items in DB depot, got 0")
	}

	// 3. Defeat verification: A weak character defeated in crawl survives with 1 HP in MariaDB
	weakling, err := characterService.Create(ctx, player.ID, "DefeatedHero")
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

	hero1 := makeHero("AtomicSolo")
	hero2 := makeHero("AtomicParty")

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

	// 1. Solo history save error rolls back rewards, inventory drops, and chronicle
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
	if afterFail1.Money != hero1.Money || afterFail1.Experience != hero1.Experience || afterFail1.Level != hero1.Level {
		t.Fatalf("expected hero1 rewards rolled back, before=(money:%d exp:%d lvl:%d) after=(money:%d exp:%d lvl:%d)",
			hero1.Money, hero1.Experience, hero1.Level, afterFail1.Money, afterFail1.Experience, afterFail1.Level)
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

	// 2. Multi-participant partial history save error (2nd participant fails) rolls back both participants' rewards and 1st participant's history
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
		if reloaded.Money != h.Money || reloaded.Experience != h.Experience {
			t.Fatalf("expected %s rewards rolled back, got money=%d exp=%d", h.Name, reloaded.Money, reloaded.Experience)
		}
		hist, err := advService.ListHistory(ctx, h.ID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if hist.Total != 0 || len(hist.Items) != 0 {
			t.Fatalf("expected %s history rolled back, got total=%d len=%d", h.Name, hist.Total, len(hist.Items))
		}
	}

	// 3. Success commits rewards and history together in the same transaction; post-commit hooks run outside tx and hook errors do not revert committed state
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
	chronAfterCommit, err := advService.GetChronicle(ctx, hero1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chronAfterCommit.TotalAdventures != 1 || chronAfterCommit.TotalVictories != 1 || !adv.IsCleared {
		t.Fatalf("expected committed chronicle and cleared adventure, got chron=%+v adv=%+v", chronAfterCommit, adv)
	}
}
