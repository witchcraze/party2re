package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

type storeDeliveryFixture struct {
	depotEquipmentFixture
	soc    *socServices
	sale   store.Sale
	herbID string
}

func newStoreDeliveryFixture(t *testing.T, barter bool) storeDeliveryFixture {
	t.Helper()
	f := newDepotEquipmentFixture(t, item.SlotMainHand, false)
	ctx := context.Background()
	soc, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	f.econ.initStore(f.core, soc.guildRepo, soc.timer, nil, storeDelivery{f.econ.depotRepo, soc.home, f.core.itemCatalog})
	if err := f.sender.AddMoney(corecharacter.MaxMoney); err != nil {
		t.Fatal(err)
	}
	if err := f.core.charRepo.Update(ctx, f.sender); err != nil {
		t.Fatal(err)
	}
	if _, err := f.move(ctx, false); err != nil {
		t.Fatal(err)
	}
	herb := item.Instance{ID: id.New(), DefinitionID: "item-001", Quantity: 2}
	if _, err := database.CreateTestDepot(ctx, f.db, f.recipient.ID, 0, []item.Instance{herb}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	st := store.Store{ID: id.New(), CharacterID: f.sender.ID, TownID: id.New(), StoreName: id.New(),
		HouseStyle: "001", Wallpaper: "none", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := f.econ.storeRepo.SaveStore(ctx, st); err != nil {
		t.Fatal(err)
	}
	var sale *store.Sale
	if barter {
		def, findErr := f.core.itemCatalog.FindByID(herb.DefinitionID)
		if findErr != nil {
			t.Fatal(findErr)
		}
		sale, err = f.econ.store.ListBarterItem(ctx, f.sender.ID, f.instance.ID, def.Name)
	} else {
		sale, err = f.econ.store.ListGoldItem(ctx, f.sender.ID, f.instance.ID, 300)
	}
	if err != nil {
		t.Fatal(err)
	}
	return storeDeliveryFixture{f, soc, *sale, herb.ID}
}

func (f storeDeliveryFixture) claim(ctx context.Context) error {
	if f.sale.SaleType == store.SaleTypeBarter {
		return f.econ.store.TradeItem(ctx, f.recipient.ID, f.sale.ID, f.herbID)
	}
	return f.econ.store.BuyItem(ctx, f.recipient.ID, f.sale.ID)
}

type storeDeliveryState struct {
	Money   []int
	Depots  []depot.Depot
	Held    [][]depot.Remittance
	Notices [][]home.DeliveryNotice
	Sales   []store.Sale
}

func (f storeDeliveryFixture) snapshot(t *testing.T) storeDeliveryState {
	t.Helper()
	ctx := context.Background()
	var state storeDeliveryState
	for _, actor := range []string{f.sender.ID, f.recipient.ID} {
		char, err := f.core.charRepo.FindByID(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.econ.depotRepo.FindByCharacterID(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		held, err := f.econ.depotRepo.ListRemittances(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		notices, err := f.soc.home.ListDeliveryNotices(ctx, actor, false)
		if err != nil {
			t.Fatal(err)
		}
		state.Money = append(state.Money, char.Money)
		state.Depots = append(state.Depots, dep)
		state.Held = append(state.Held, held)
		state.Notices = append(state.Notices, notices)
	}
	var err error
	state.Sales, err = f.econ.storeRepo.GetSalesByStoreID(ctx, f.sale.StoreID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (f storeDeliveryFixture) assertSettled(t *testing.T) {
	t.Helper()
	s := f.snapshot(t)
	if len(s.Sales) != 0 || len(s.Held[1]) != 0 {
		t.Fatalf("listing or buyer receipts remain: %+v", s)
	}
	if s.Money[0] != corecharacter.MaxMoney || len(s.Depots[1].Items) != 2 {
		t.Fatalf("seller wallet or buyer items = %+v", s)
	}
	var weapon, herbs int
	for _, dep := range s.Depots {
		for _, inst := range dep.Items {
			switch inst.DefinitionID {
			case f.instance.DefinitionID:
				weapon += inst.Quantity
				if dep.CharacterID != f.recipient.ID || inst.EnhancementLevel != f.instance.EnhancementLevel {
					t.Fatalf("delivered weapon = %+v", inst)
				}
			case "item-001":
				herbs += inst.Quantity
			}
		}
	}
	if weapon != 1 || herbs != 2 {
		t.Fatalf("asset quantities: weapon=%d herbs=%d", weapon, herbs)
	}
	if f.sale.SaleType == store.SaleTypeGold {
		if s.Money[1] != f.recipient.Money-300 || len(s.Held[0]) != 1 || len(s.Notices[0]) != 1 || len(s.Notices[1]) != 0 || len(s.Depots[0].Items) != 0 {
			t.Fatalf("purchase assets/notices = %+v", s)
		}
		r := s.Held[0][0]
		if r.Amount != 300 || r.Source != depot.RemittanceStorePayment || r.SenderName != f.recipient.Name ||
			s.Money[0]+s.Money[1]+r.Amount != corecharacter.MaxMoney+f.recipient.Money ||
			s.Notices[0][0].NoticeType != depot.NoticeMoneyTransfer || s.Notices[0][0].Message != "お金が預かり所に届いています" {
			t.Fatalf("purchase receipt/notice = %+v", s)
		}
		return
	}
	if s.Money[1] != f.recipient.Money || len(s.Held[0]) != 0 || len(s.Depots[0].Items) != 1 ||
		s.Depots[0].Items[0].DefinitionID != "item-001" || s.Depots[0].Items[0].Quantity != 1 {
		t.Fatalf("barter assets = %+v", s)
	}
	for i, want := range []struct{ sender, itemID string }{{f.recipient.Name, "item-001"}, {f.sender.Name, f.instance.DefinitionID}} {
		def, err := f.core.itemCatalog.FindByID(want.itemID)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Notices[i]) != 1 || s.Notices[i][0].NoticeType != depot.NoticeItemTransfer ||
			s.Notices[i][0].Message != "預かり所に"+want.sender+"から"+def.Name+"が届いています" {
			t.Fatalf("barter notices = %+v", s.Notices)
		}
	}
}

type failStoreRemittance struct{ depot.RemittanceRepository }

func (f failStoreRemittance) AddRemittance(ctx context.Context, value depot.Remittance) error {
	if err := f.RemittanceRepository.AddRemittance(ctx, value); err != nil {
		return err
	}
	return errNoticeAfterWrite
}

type failStoreItemNotice struct {
	depot.DeliveryNotifier
	failAt, calls int
}

func (f *failStoreItemNotice) AddDeliveryNotice(ctx context.Context, actor, kind, message string) error {
	if err := f.DeliveryNotifier.AddDeliveryNotice(ctx, actor, kind, message); err != nil {
		return err
	}
	f.calls++
	if f.calls == f.failAt {
		return errNoticeAfterWrite
	}
	return nil
}

func TestStoreDeliveryRollbackAndRetry_Integration(t *testing.T) {
	for _, stage := range []string{"remittance", "money flag", "first barter notice", "second barter notice"} {
		t.Run(stage, func(t *testing.T) {
			barter := stage == "first barter notice" || stage == "second barter notice"
			f := newStoreDeliveryFixture(t, barter)
			good := storeDelivery{f.econ.depotRepo, f.soc.home, f.core.itemCatalog}
			bad := good
			switch stage {
			case "remittance":
				bad.remittances = failStoreRemittance{good.remittances}
			case "money flag":
				bad.notifier = failAfterNotice{good.notifier}
			case "first barter notice":
				bad.notifier = &failStoreItemNotice{DeliveryNotifier: good.notifier, failAt: 1}
			case "second barter notice":
				bad.notifier = &failStoreItemNotice{DeliveryNotifier: good.notifier, failAt: 2}
			}
			f.econ.initStore(f.core, f.soc.guildRepo, f.soc.timer, nil, bad)
			before := f.snapshot(t)
			if err := f.claim(context.Background()); !errors.Is(err, errNoticeAfterWrite) {
				t.Fatalf("injected failure = %v", err)
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("rollback changed SQL state: before=%+v after=%+v", before, after)
			}
			f.econ.initStore(f.core, f.soc.guildRepo, f.soc.timer, nil, good)
			if err := f.claim(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.assertSettled(t)
			committed := f.snapshot(t)
			if err := f.claim(context.Background()); !errors.Is(err, store.ErrListingNotFound) {
				t.Fatalf("repeat claim = %v", err)
			}
			if after := f.snapshot(t); !reflect.DeepEqual(committed, after) {
				t.Fatal("repeat claim changed committed state")
			}
		})
	}
}

func TestStoreDeliveryCompetingClaims_Integration(t *testing.T) {
	for _, barter := range []bool{false, true} {
		name := "purchase"
		if barter {
			name = "barter"
		}
		t.Run(name, func(t *testing.T) {
			f := newStoreDeliveryFixture(t, barter)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
				err := f.claim(ctx)
				if errors.Is(err, store.ErrListingNotFound) {
					return err
				}
				if err != nil {
					t.Errorf("claim failed: %v", err)
				}
				return err
			})
			if result.Successes != 1 || result.Deadlocks != 0 {
				t.Fatalf("competing claims: %+v", result)
			}
			f.assertSettled(t)
		})
	}
}

func TestStorePaymentCollection_Integration(t *testing.T) {
	f := newStoreDeliveryFixture(t, false)
	ctx := context.Background()
	if err := f.claim(ctx); err != nil {
		t.Fatal(err)
	}
	f.assertSettled(t)
	for _, amount := range []int{100, 200} {
		char, err := f.core.charRepo.FindByID(ctx, f.sender.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := char.DeductMoney(amount); err != nil {
			t.Fatal(err)
		}
		if err := f.core.charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		_, collected, err := f.econ.depot.WithdrawItem(ctx, char.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(collected) != 1 || collected[0].Amount != amount || collected[0].Source != depot.RemittanceStorePayment || collected[0].SenderName != f.recipient.Name {
			t.Fatalf("collected = %+v", collected)
		}
		s := f.snapshot(t)
		if s.Money[0] != corecharacter.MaxMoney || (amount == 100 && (len(s.Held[0]) != 1 || s.Held[0][0].Amount != 200)) || (amount == 200 && len(s.Held[0]) != 0) {
			t.Fatalf("collection state = %+v", s)
		}
		if _, _, err := f.econ.depot.WithdrawItem(ctx, char.ID, ""); !errors.Is(err, depot.ErrInvalidItemInstanceID) {
			t.Fatalf("repeat collection at cap = %v", err)
		}
		if after := f.snapshot(t); !reflect.DeepEqual(s, after) {
			t.Fatal("repeat collection changed assets")
		}
	}
}

func TestStoreWithdrawalSendsNoNotice_Integration(t *testing.T) {
	f := newStoreDeliveryFixture(t, false)
	// Withdrawal works without the payment/notice port.
	f.econ.initStore(f.core, f.soc.guildRepo, f.soc.timer, nil, nil)
	before := f.snapshot(t)
	if err := f.econ.store.WithdrawListing(context.Background(), f.sender.ID, f.sale.ID); err != nil {
		t.Fatal(err)
	}
	after := f.snapshot(t)
	if len(after.Sales) != 0 || len(after.Depots[0].Items) != 1 || after.Depots[0].Items[0].DefinitionID != f.instance.DefinitionID ||
		after.Depots[0].Items[0].Quantity != 1 || after.Depots[0].Items[0].EnhancementLevel != f.instance.EnhancementLevel ||
		!reflect.DeepEqual(before.Notices, after.Notices) || !reflect.DeepEqual(before.Held, after.Held) || !reflect.DeepEqual(before.Money, after.Money) ||
		!reflect.DeepEqual(before.Depots[1], after.Depots[1]) {
		t.Fatalf("withdrawal state = %+v", after)
	}
}
