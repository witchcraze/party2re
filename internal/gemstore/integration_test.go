package gemstore_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingGemBoxRepo struct {
	*database.GemBoxRepository
	fail bool
}

func (r *failingGemBoxRepo) Save(ctx context.Context, box gemstore.GemBox) error {
	if r.fail {
		return errors.New("simulated gem-box save failure")
	}
	return r.GemBoxRepository.Save(ctx, box)
}

type gemStoreDBFixture struct {
	db          *sql.DB
	chars       *database.CharacterRepository
	boxes       *failingGemBoxRepo
	depots      *database.DepotRepository
	inventories *database.InventoryRepository
	equip       *database.EquipmentRepository
	catalog     *gemstore.Catalog
	itemCatalog *item.Catalog
}

func newGemStoreDBFixture(t *testing.T) *gemStoreDBFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
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
	boxes, err := database.NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	itemCatalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	return &gemStoreDBFixture{
		db:          db,
		chars:       chars,
		boxes:       &failingGemBoxRepo{GemBoxRepository: boxes},
		depots:      depots,
		inventories: inventories,
		equip:       equip,
		catalog:     catalog,
		itemCatalog: itemCatalog,
	}
}

func (f *gemStoreDBFixture) createCharacter(t *testing.T, ctx context.Context, name string, money int) corecharacter.Character {
	t.Helper()
	actor, err := database.CreateTestCharacterWithFunds(ctx, f.db, name, money)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := f.chars.Delete(cleanupCtx, actor.ID); err != nil {
			t.Error(err)
		}
	})
	return actor
}

func (f *gemStoreDBFixture) seedGemBox(t *testing.T, ctx context.Context, characterID string, capacity int, defs ...string) []item.Instance {
	t.Helper()
	items := make([]item.Instance, len(defs))
	for i, def := range defs {
		inst, err := item.NewInstance(def, 1)
		if err != nil {
			t.Fatal(err)
		}
		items[i] = inst
	}
	if err := f.boxes.GemBoxRepository.Save(ctx, gemstore.GemBox{
		CharacterID: characterID,
		Capacity:    capacity,
		Items:       items,
	}); err != nil {
		t.Fatal(err)
	}
	return items
}

func (f *gemStoreDBFixture) service(t *testing.T) *gemstore.Service {
	t.Helper()
	svc, err := gemstore.NewService(
		f.catalog,
		f.chars,
		gemstore.WithGemBoxRepository(f.boxes),
		gemstore.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		gemstore.WithItemDefinitionProvider(f.itemCatalog),
		gemstore.WithDepotRepository(f.depots),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestGemStorePersistenceAndRollback_Integration(t *testing.T) {
	for _, op := range []string{"sell", "sort"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%t", op, fail), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()

				f := newGemStoreDBFixture(t)
				actor := f.createCharacter(t, ctx, "GemPersist", 0)
				items := f.seedGemBox(t, ctx, actor.ID, 5, "gem_atk_2", "gem_atk_1")
				svc := f.service(t)

				before, err := f.boxes.FindByCharacterID(ctx, actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				f.boxes.fail = fail

				var sellResult gemstore.SellResult
				if op == "sort" {
					_, err = svc.SortGemBox(ctx, actor.ID)
				} else {
					sellResult, err = svc.SellGem(ctx, actor.ID, items[0].ID)
				}

				after, findBoxErr := f.boxes.FindByCharacterID(ctx, actor.ID)
				if findBoxErr != nil {
					t.Fatal(findBoxErr)
				}
				char, findCharErr := f.chars.FindByID(ctx, actor.ID)
				if findCharErr != nil {
					t.Fatal(findCharErr)
				}

				if fail {
					if err == nil || !reflect.DeepEqual(before, after) || char.Money != actor.Money {
						t.Fatalf("partial mutation on save failure: err=%v box=%+v char=%+v", err, after, char)
					}
					return
				}
				if err != nil {
					t.Fatalf("%s failed: %v", op, err)
				}

				if op == "sort" {
					if len(after.Items) != 2 || after.Items[0].ID != items[1].ID || after.Items[1].ID != items[0].ID || char.Money != 0 {
						t.Fatalf("sort lost order/assets: box=%+v char=%+v", after, char)
					}
					return
				}

				if len(after.Items) != 1 || after.Items[0].ID != items[1].ID || char.Money != 50 || sellResult.Payout != 50 || sellResult.Gem.ID != "gem_atk_2" {
					t.Fatalf("sale lost assets: box=%+v char=%+v result=%+v", after, char, sellResult)
				}
				if _, dupErr := svc.SellGem(ctx, actor.ID, items[0].ID); !errors.Is(dupErr, gemstore.ErrItemNotOwned) {
					t.Fatalf("duplicate sale err = %v, want ErrItemNotOwned", dupErr)
				}
			})
		}
	}
}

func TestGemStoreSynthesisWithRenamedItemRollsBack_Integration(t *testing.T) {
	for _, outcome := range []string{"success", "save failure", "missing material", "hand only"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			f := newGemStoreDBFixture(t)
			actor := f.createCharacter(t, ctx, "GemSynth", 0)
			f.seedGemBox(t, ctx, actor.ID, 5, "gem_sky_atk_2")

			dep, err := f.depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			reins, err := item.NewInstance("item-059", 1)
			if err != nil {
				t.Fatal(err)
			}
			invBefore, err := f.inventories.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}

			if outcome == "hand only" {
				if err := invBefore.Add(reins); err != nil {
					t.Fatal(err)
				}
				if err := f.inventories.Save(ctx, invBefore); err != nil {
					t.Fatal(err)
				}
			} else if outcome != "missing material" {
				if err := dep.AddItem(reins); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.depots.Save(ctx, dep); err != nil {
				t.Fatal(err)
			}

			boxBefore, err := f.boxes.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.boxes.fail = outcome == "save failure"
			svc := f.service(t)

			_, synthErr := svc.SynthesizeGem(ctx, actor.ID, "recipe_burst_sky")

			boxAfter, err := f.boxes.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			depAfter, err := f.depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			invAfter, err := f.inventories.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(invBefore, invAfter) {
				t.Fatalf("synthesis changed hand slots: before=%+v after=%+v", invBefore, invAfter)
			}

			if outcome != "success" {
				if outcome == "save failure" {
					if synthErr == nil {
						t.Fatal("expected error on save failure, got nil")
					}
				} else if !errors.Is(synthErr, gemstore.ErrInsufficientMaterials) {
					t.Fatalf("SynthesizeGem err = %v, want ErrInsufficientMaterials", synthErr)
				}
				if !reflect.DeepEqual(boxBefore, boxAfter) || !reflect.DeepEqual(dep.Items, depAfter.Items) {
					t.Fatalf("partial mutation: box=%+v depot=%+v", boxAfter, depAfter)
				}
				return
			}

			if synthErr != nil {
				t.Fatalf("SynthesizeGem failed: %v", synthErr)
			}
			if len(boxAfter.Items) != 1 || boxAfter.Items[0].DefinitionID != "gem_burst_sky" || len(depAfter.Items) != 0 {
				t.Fatalf("synthesis lost assets: box=%+v depot=%+v", boxAfter, depAfter)
			}
		})
	}
}

func TestGemStoreConcurrentSaleAndSort_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := newGemStoreDBFixture(t)
	actor := f.createCharacter(t, ctx, "GemRaceSS", 0)
	items := f.seedGemBox(t, ctx, actor.ID, 5, "gem_atk_2", "gem_atk_1")
	svc := f.service(t)

	sell := func() error {
		opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer opCancel()
		_, err := svc.SellGem(opCtx, actor.ID, items[0].ID)
		return err
	}
	sortBox := func() error {
		opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer opCancel()
		_, err := svc.SortGemBox(opCtx, actor.ID)
		return err
	}

	errs := testutil.RunRace(sell, sell, sortBox)
	for _, err := range errs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during concurrent SellGem/SortGemBox: %v", err)
		}
	}
	if errs[2] != nil {
		t.Fatalf("SortGemBox failed: %v", errs[2])
	}
	if !((errs[0] == nil && errors.Is(errs[1], gemstore.ErrItemNotOwned)) ||
		(errors.Is(errs[0], gemstore.ErrItemNotOwned) && errs[1] == nil)) {
		t.Fatalf("expected exactly one SellGem success and one ErrItemNotOwned, got %v", errs)
	}

	char, err := f.chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	box, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if char.Money != 50 || len(box.Items) != 1 || box.Items[0].ID != items[1].ID {
		t.Fatalf("assets not conserved: char=%+v box=%+v", char, box)
	}
}

func TestGemStoreBuyPersistsAndRejectsWithoutFunds_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := newGemStoreDBFixture(t)
	actor := f.createCharacter(t, ctx, "GemBuy", 1)
	f.seedGemBox(t, ctx, actor.ID, 5, "gem_atk_2", "gem_atk_1")
	svc := f.service(t)

	if _, err := svc.BuyGem(ctx, actor.ID, "gem_atk_1"); !errors.Is(err, gemstore.ErrInsufficientFunds) {
		t.Fatalf("BuyGem with 1G err = %v, want ErrInsufficientFunds", err)
	}
	charAfterReject, err := f.chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	boxAfterReject, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if charAfterReject.Money != 1 || len(boxAfterReject.Items) != 2 {
		t.Fatalf("rejected purchase mutated state: char=%+v box=%+v", charAfterReject, boxAfterReject)
	}

	funded := actor
	funded.Money = 1000
	if err := f.chars.Update(ctx, funded); err != nil {
		t.Fatal(err)
	}

	result, err := svc.BuyGem(ctx, actor.ID, "gem_atk_1")
	if err != nil {
		t.Fatalf("funded BuyGem failed: %v", err)
	}
	char, err := f.chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	box, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if char.Money != 1000-result.Cost || result.Cost <= 0 || len(box.Items) != 3 || box.Items[2].ID != result.ItemInstance.ID {
		t.Fatalf("purchase not persisted: money=%d cost=%d box=%+v", char.Money, result.Cost, box)
	}
}

func TestGemStoreConcurrentSendConservesGem_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := newGemStoreDBFixture(t)
	actor := f.createCharacter(t, ctx, "GemSender", 0)
	recipient := f.createCharacter(t, ctx, "GemRecip", 0)
	items := f.seedGemBox(t, ctx, actor.ID, 5, "gem_atk_2", "gem_atk_1")
	svc := f.service(t)

	send := func() error {
		opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer opCancel()
		_, err := svc.SendGem(opCtx, actor.ID, recipient.ID, items[0].ID)
		return err
	}

	errs := testutil.RunRace(send, send)
	for _, err := range errs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during concurrent SendGem: %v", err)
		}
	}
	if !((errs[0] == nil && errors.Is(errs[1], gemstore.ErrItemNotOwned)) ||
		(errors.Is(errs[0], gemstore.ErrItemNotOwned) && errs[1] == nil)) {
		t.Fatalf("expected exactly one SendGem success and one ErrItemNotOwned, got %v", errs)
	}

	senderBox, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	recipientBox, err := f.boxes.FindByCharacterID(ctx, recipient.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(senderBox.Items) != 1 || senderBox.Items[0].ID != items[1].ID ||
		len(recipientBox.Items) != 1 || recipientBox.Items[0].DefinitionID != "gem_atk_2" {
		t.Fatalf("gem not conserved: sender=%+v recipient=%+v", senderBox, recipientBox)
	}
}

func TestGemStoreAppraisesDepotNotEquippedHandSlot_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := newGemStoreDBFixture(t)
	actor := f.createCharacter(t, ctx, "GemAppraise", 0)
	f.seedGemBox(t, ctx, actor.ID, 5, "gem_atk_2", "gem_atk_1")

	held, err := item.NewInstance("item-251", 1)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := item.NewInstance("item-251", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, f.db, actor.ID, []item.Instance{held}); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, coreequipment.Equipment{
		CharacterID: actor.ID,
		Slots:       map[item.Slot]string{item.SlotAccessory: held.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, actor.ID, 0, []item.Instance{stored}); err != nil {
		t.Fatal(err)
	}

	assertHandKept := func() {
		t.Helper()
		owned, err := f.inventories.FindByCharacterID(ctx, actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		eq, err := f.equip.FindByCharacterID(ctx, actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(owned.Items) != 1 || owned.Items[0].ID != held.ID || eq.Slots[item.SlotAccessory] != held.ID {
			t.Fatalf("hand slot changed: inv=%+v eq=%+v", owned.Items, eq.Slots)
		}
	}

	svc := f.service(t)

	// Legacy kantei never reads the hand slots.
	if _, err := svc.AppraiseItem(ctx, actor.ID, held.ID); !errors.Is(err, gemstore.ErrItemNotOwned) {
		t.Fatalf("hand-slot appraisal err = %v, want ErrItemNotOwned", err)
	}
	assertHandKept()

	before, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.AppraiseItem(ctx, actor.ID, "光る宝珠")
	if err != nil {
		t.Fatalf("depot appraisal by name failed: %v", err)
	}
	if !res.IsGem || res.IdentifiedGem == nil {
		t.Fatalf("expected gem appraisal result, got %+v", res)
	}
	assertHandKept()

	dep, err := f.depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 0 || len(after.Items) != len(before.Items)+1 {
		t.Fatalf("depot orb not appraised: depot=%+v box=%+v", dep.Items, after.Items)
	}
}
