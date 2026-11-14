package blackmarket_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingPointsBlackMarketRepo struct {
	*database.BlackMarketRepository
}

func (r failingPointsBlackMarketRepo) SaveCharacterPoints(context.Context, blackmarket.CharacterPoints) error {
	return errors.New("simulated points save failure")
}

type blackMarketDBFixture struct {
	db      *sql.DB
	chars   *database.CharacterRepository
	inv     *database.InventoryRepository
	equip   *database.EquipmentRepository
	depots  *database.DepotRepository
	points  *database.BlackMarketRepository
	catalog *blackmarket.Catalog
	itemCat *item.Catalog
}

func newBlackMarketDBFixture(t *testing.T) *blackMarketDBFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN not set")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	points, err := database.NewBlackMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	itemCat, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return &blackMarketDBFixture{
		db:      db,
		chars:   chars,
		inv:     inv,
		equip:   equip,
		depots:  depots,
		points:  points,
		catalog: catalog,
		itemCat: itemCat,
	}
}

func (f *blackMarketDBFixture) createCharacter(t *testing.T, ctx context.Context, name string) corecharacter.Character {
	t.Helper()
	char, err := database.CreateTestCharacterWithFunds(ctx, f.db, name, 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := f.chars.Delete(cleanupCtx, char.ID); err != nil {
			t.Error(err)
		}
	})

	char.JobLevel = blackmarket.MinAccessJobLevel
	if err := f.chars.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	return char
}

func (f *blackMarketDBFixture) service(t *testing.T, failPoints bool) *blackmarket.Service {
	t.Helper()
	var ptsRepo blackmarket.BlackMarketRepository = f.points
	if failPoints {
		ptsRepo = failingPointsBlackMarketRepo{BlackMarketRepository: f.points}
	}
	svc, err := blackmarket.NewService(
		f.chars,
		f.inv,
		ptsRepo,
		f.catalog,
		blackmarket.WithDepotRepository(f.depots),
		blackmarket.WithEquipmentRepository(f.equip),
		blackmarket.WithTransactionProvider(database.NewTransactionProvider(f.db)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestBlackMarketSacrificeAndTradePersistence_Integration(t *testing.T) {
	f := newBlackMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	char := f.createCharacter(t, ctx, "BMMain")
	commonItem, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	rareDepotItem, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, f.db, char.ID, []item.Instance{commonItem}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, char.ID, 0, []item.Instance{rareDepotItem}); err != nil {
		t.Fatal(err)
	}

	svc := f.service(t, false)

	// Ineligible sacrifice & unaffordable trade leave DB unchanged.
	if _, err := svc.SacrificeItem(ctx, char.ID, commonItem.ID); !errors.Is(err, blackmarket.ErrNotSacrificeEligible) {
		t.Fatalf("expected ErrNotSacrificeEligible, got %v", err)
	}
	if _, err := svc.TradePrize(ctx, char.ID, "bm_prize_087"); !errors.Is(err, blackmarket.ErrInsufficientRarePoints) {
		t.Fatalf("expected ErrInsufficientRarePoints, got %v", err)
	}

	pts, err := f.points.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pts.RarePoints != 0 || pts.URarePoints != 0 {
		t.Fatalf("expected 0 points after rejected ops, got %+v", pts)
	}

	sacRes, err := svc.SacrificeItem(ctx, char.ID, rareDepotItem.ID)
	if err != nil {
		t.Fatalf("SacrificeItem failed: %v", err)
	}
	if sacRes.TotalRarePoints != 1 || sacRes.RarePointsGained != 1 {
		t.Fatalf("unexpected sacrifice result: %+v", sacRes)
	}

	depAfterSac, err := f.depots.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depAfterSac.Items) != 0 {
		t.Fatalf("expected sacrificed depot item consumed, got %+v", depAfterSac.Items)
	}

	tradeRes, err := svc.TradePrize(ctx, char.ID, "bm_prize_087")
	if err != nil {
		t.Fatalf("TradePrize failed: %v", err)
	}
	if tradeRes.DepotInstanceID == "" || tradeRes.RemainingRare != 0 || tradeRes.Cost != 1 {
		t.Fatalf("unexpected trade result: %+v", tradeRes)
	}

	depAfterTrade, err := f.depots.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depAfterTrade.Items) != 1 || depAfterTrade.Items[0].ID != tradeRes.DepotInstanceID || depAfterTrade.Items[0].DefinitionID != tradeRes.ItemDefinitionID {
		t.Fatalf("depot after trade = %+v (tradeRes=%+v)", depAfterTrade.Items, tradeRes)
	}
	ptsAfterTrade, err := f.points.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ptsAfterTrade.RarePoints != 0 {
		t.Fatalf("expected 0 rare points after trade, got %+v", ptsAfterTrade)
	}
}

func TestBlackMarketRollsBackAssetsWhenPointsFail_Integration(t *testing.T) {
	f := newBlackMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	char := f.createCharacter(t, ctx, "BMRbk")
	char.WeaponSeal = 2
	char.WeaponCustomName = "RareBlade"
	char.ArmorCustomName = "KeepArmor"
	if err := f.chars.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	rareEq, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.CreateTestInventoryWithItems(ctx, f.db, char.ID, []item.Instance{rareEq})
	if err != nil {
		t.Fatal(err)
	}
	eq, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	def, err := f.itemCat.FindByID("weapon-29")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eq.Equip(&inv, def, rareEq.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, eq); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, char.ID, 0, nil); err != nil {
		t.Fatal(err)
	}

	failingSvc := f.service(t, true)
	if _, err := failingSvc.SacrificeItem(ctx, char.ID, rareEq.ID); err == nil {
		t.Fatal("expected SacrificeItem failure when SaveCharacterPoints fails")
	}

	persistedChar, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedEq, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqID, equipped := persistedEq.Equipped(item.SlotMainHand)
	persistedInv, err := f.inv.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, inInv := persistedInv.Find(rareEq.ID)
	pts, err := f.points.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedChar.WeaponSeal != 2 || persistedChar.WeaponCustomName != "RareBlade" || persistedChar.ArmorCustomName != "KeepArmor" ||
		!equipped || eqID != rareEq.ID || !inInv || pts.RarePoints != 0 {
		t.Fatalf("sacrifice rollback leaked state: char=%+v equipped=%v(%s) inInv=%v pts=%+v", persistedChar, equipped, eqID, inInv, pts)
	}

	// Retry sacrifice succeeds cleanly.
	healthySvc := f.service(t, false)
	if _, err := healthySvc.SacrificeItem(ctx, char.ID, rareEq.ID); err != nil {
		t.Fatalf("retry SacrificeItem failed: %v", err)
	}

	// Now verify TradePrize rollback on SaveCharacterPoints failure.
	if _, err := failingSvc.TradePrize(ctx, char.ID, "bm_prize_087"); err == nil {
		t.Fatal("expected TradePrize failure when SaveCharacterPoints fails")
	}
	depAfterFailTrade, err := f.depots.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	ptsAfterFailTrade, err := f.points.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depAfterFailTrade.Items) != 0 || ptsAfterFailTrade.RarePoints != 1 {
		t.Fatalf("trade rollback leaked state: depot=%+v pts=%+v", depAfterFailTrade.Items, ptsAfterFailTrade)
	}

	if _, err := healthySvc.TradePrize(ctx, char.ID, "bm_prize_087"); err != nil {
		t.Fatalf("retry TradePrize failed: %v", err)
	}
}

func TestBlackMarketConcurrentAssetsAreConserved_Integration(t *testing.T) {
	f := newBlackMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	char := f.createCharacter(t, ctx, "BMRace")
	if err := f.points.SaveCharacterPoints(ctx, blackmarket.CharacterPoints{CharacterID: char.ID, RarePoints: 1}); err != nil {
		t.Fatal(err)
	}
	rareInst, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, f.db, char.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, char.ID, 0, []item.Instance{rareInst}); err != nil {
		t.Fatal(err)
	}

	svc := f.service(t, false)
	sacSuccesses := 0
	tradeSuccesses := 0

	errs := testutil.RunRace(
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.SacrificeItem(opCtx, char.ID, rareInst.ID)
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.SacrificeItem(opCtx, char.ID, rareInst.ID)
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.TradePrize(opCtx, char.ID, "bm_prize_087")
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.TradePrize(opCtx, char.ID, "bm_prize_087")
			return err
		},
	)
	for i, err := range errs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock on op %d: %v", i, err)
		}
		if err == nil {
			if i < 2 {
				sacSuccesses++
			} else {
				tradeSuccesses++
			}
		}
	}
	if sacSuccesses != 1 {
		t.Fatalf("expected exactly 1 sacrifice success, got %d (errs=%v)", sacSuccesses, errs)
	}
	if tradeSuccesses < 1 || tradeSuccesses > 2 {
		t.Fatalf("expected 1 or 2 trade successes, got %d (errs=%v)", tradeSuccesses, errs)
	}

	pts, err := f.points.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedRemaining := 1 + sacSuccesses - tradeSuccesses
	if pts.RarePoints != expectedRemaining || pts.RarePoints < 0 {
		t.Fatalf("points mismatch: got %d, want %d", pts.RarePoints, expectedRemaining)
	}
	dep, err := f.depots.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if qty := dep.Quantity("item-087"); qty != tradeSuccesses {
		t.Fatalf("depot prize quantity mismatch: got %d, want %d (items=%+v)", qty, tradeSuccesses, dep.Items)
	}
	if rem := dep.Quantity("weapon-29"); rem != 0 {
		t.Fatalf("expected sacrificed weapon removed from depot, got %+v", dep.Items)
	}
}

func TestBlackMarketEquippedCandidateAndCustomization_Integration(t *testing.T) {
	f := newBlackMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	char := f.createCharacter(t, ctx, "BMEq")
	char.WeaponSeal = 3
	char.WeaponCustomName = "OldSword"
	char.ArmorCustomName = "KeptArmor"
	if err := f.chars.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	rareEq, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	rareDepot, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.CreateTestInventoryWithItems(ctx, f.db, char.ID, []item.Instance{rareEq})
	if err != nil {
		t.Fatal(err)
	}
	eq, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	def, err := f.itemCat.FindByID("weapon-29")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eq.Equip(&inv, def, rareEq.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, eq); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, char.ID, 0, []item.Instance{rareDepot}); err != nil {
		t.Fatal(err)
	}

	svc := f.service(t, false)
	statusBefore, err := svc.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(statusBefore.SacrificeCandidates) != 2 ||
		statusBefore.SacrificeCandidates[0].ItemInstanceID != rareEq.ID ||
		!statusBefore.SacrificeCandidates[0].Equipped {
		t.Fatalf("expected equipped candidate first with Equipped=true, got %+v", statusBefore.SacrificeCandidates)
	}

	if _, err := svc.SacrificeItem(ctx, char.ID, rareEq.ID); err != nil {
		t.Fatalf("SacrificeItem equipped failed: %v", err)
	}

	persistedChar, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedChar.WeaponSeal != 0 || persistedChar.WeaponCustomName != "" || persistedChar.ArmorCustomName != "KeptArmor" {
		t.Fatalf("expected weapon customization cleared and armor kept, got %+v", persistedChar)
	}

	statusAfter, err := svc.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(statusAfter.SacrificeCandidates) != 1 || statusAfter.SacrificeCandidates[0].ItemInstanceID != rareDepot.ID || statusAfter.SacrificeCandidates[0].Equipped {
		t.Fatalf("expected only depot candidate remaining, got %+v", statusAfter.SacrificeCandidates)
	}
}
