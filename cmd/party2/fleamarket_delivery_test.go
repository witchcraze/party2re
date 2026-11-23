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
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/testutil"
)

type fleaDeliveryFixture struct {
	depotEquipmentFixture
	soc     *socServices
	listing fleamarket.Listing
}

func newFleaDeliveryFixture(t *testing.T) fleaDeliveryFixture {
	t.Helper()
	f := newDepotEquipmentFixture(t, item.SlotMainHand, false)
	ctx := context.Background()
	soc, err := newSocServices(f.db, f.core, f.econ, testutil.OpenValkey(t), logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	f.sender.Money = corecharacter.MaxMoney - 100
	if err := f.core.charRepo.Update(ctx, f.sender); err != nil {
		t.Fatal(err)
	}
	if _, err := f.move(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, f.recipient.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	listing, err := f.econ.fleamarket.CreateListing(ctx, f.sender.ID, f.instance.ID, 250, time.Now().UTC().Truncate(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return fleaDeliveryFixture{f, soc, listing}
}

type fleaDeliveryState struct {
	Money   []int
	Depots  []depot.Depot
	Held    [][]depot.Remittance
	Notices [][]home.DeliveryNotice
	Listing fleamarket.Listing
}

func (f fleaDeliveryFixture) snapshot(t *testing.T) fleaDeliveryState {
	t.Helper()
	ctx := context.Background()
	var state fleaDeliveryState
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
	state.Listing, err = f.econ.fleamarketRepo.GetListingByID(ctx, f.listing.ID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (f fleaDeliveryFixture) assertSettled(t *testing.T) {
	t.Helper()
	s := f.snapshot(t)
	if s.Listing.Status != fleamarket.StatusSold || len(s.Held[1]) != 0 || len(s.Notices[1]) != 0 {
		t.Fatalf("listing or buyer receipts unexpected: %+v", s)
	}
	if s.Money[0] != corecharacter.MaxMoney-100 || s.Money[1] != f.recipient.Money-250 {
		t.Fatalf("wallets = %+v", s.Money)
	}
	if len(s.Depots[0].Items) != 0 || len(s.Depots[1].Items) != 1 ||
		s.Depots[1].Items[0].DefinitionID != f.instance.DefinitionID ||
		s.Depots[1].Items[0].EnhancementLevel != f.instance.EnhancementLevel {
		t.Fatalf("depots = %+v", s.Depots)
	}
	if len(s.Held[0]) != 1 || len(s.Notices[0]) != 1 {
		t.Fatalf("held=%+v notices=%+v", s.Held[0], s.Notices[0])
	}
	r := s.Held[0][0]
	if r.Amount != 250 || r.Source != depot.RemittanceFleaMarketPayment || r.SenderName != f.recipient.Name ||
		s.Notices[0][0].NoticeType != depot.NoticeMoneyTransfer || s.Notices[0][0].Message != "お金が預かり所に届いています" {
		t.Fatalf("remittance=%+v notice=%+v", r, s.Notices[0][0])
	}
}

func TestFleaMarketDeliveryRollbackAndRetry_Integration(t *testing.T) {
	for _, stage := range []string{"remittance", "money flag"} {
		t.Run(stage, func(t *testing.T) {
			f := newFleaDeliveryFixture(t)
			good := fleaMarketDelivery{remittances: f.econ.depotRepo, notifier: f.soc.home}
			bad := good
			if stage == "remittance" {
				bad.remittances = failStoreRemittance{good.remittances}
			} else {
				bad.notifier = failAfterNotice{good.notifier}
			}
			f.econ.fleamarket.SetPaymentHolder(bad)
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Second)
			before := f.snapshot(t)
			if _, err := f.econ.fleamarket.PurchaseListing(ctx, f.recipient.ID, f.listing.ID, now); !errors.Is(err, errNoticeAfterWrite) {
				t.Fatalf("injected failure = %v", err)
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("rollback changed SQL state: before=%+v after=%+v", before, after)
			}
			f.econ.fleamarket.SetPaymentHolder(good)
			res, err := f.econ.fleamarket.PurchaseListing(ctx, f.recipient.ID, f.listing.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if res.BuyerGold != f.recipient.Money-250 || res.Listing.Status != fleamarket.StatusSold {
				t.Fatalf("unexpected purchase result: %+v", res)
			}
			f.assertSettled(t)
			committed := f.snapshot(t)
			if _, err := f.econ.fleamarket.PurchaseListing(ctx, f.recipient.ID, f.listing.ID, now); !errors.Is(err, fleamarket.ErrListingNotActive) {
				t.Fatalf("repeat purchase = %v", err)
			}
			if after := f.snapshot(t); !reflect.DeepEqual(committed, after) {
				t.Fatal("repeat purchase changed committed state")
			}
		})
	}
}

func TestFleaMarketPaymentCollection_Integration(t *testing.T) {
	f := newFleaDeliveryFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := f.econ.fleamarket.PurchaseListing(ctx, f.recipient.ID, f.listing.ID, now); err != nil {
		t.Fatal(err)
	}
	f.assertSettled(t)

	// First withdrawal collects 100G up to the wallet cap and leaves 150G held.
	_, collected, err := f.econ.depot.WithdrawItem(ctx, f.sender.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(collected) != 1 || collected[0].Amount != 100 || collected[0].Source != depot.RemittanceFleaMarketPayment || collected[0].SenderName != f.recipient.Name {
		t.Fatalf("first collected = %+v", collected)
	}
	s := f.snapshot(t)
	if s.Money[0] != corecharacter.MaxMoney || len(s.Held[0]) != 1 || s.Held[0][0].Amount != 150 {
		t.Fatalf("state after capped collection = %+v", s)
	}
	if _, _, err := f.econ.depot.WithdrawItem(ctx, f.sender.ID, ""); !errors.Is(err, depot.ErrInvalidItemInstanceID) {
		t.Fatalf("repeat collection at cap = %v", err)
	}
	if after := f.snapshot(t); !reflect.DeepEqual(s, after) {
		t.Fatal("repeat collection at cap changed assets")
	}

	// Spend 200G and collect the remaining 150G without losing gold or double-collecting.
	char, err := f.core.charRepo.FindByID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := char.DeductMoney(200); err != nil {
		t.Fatal(err)
	}
	if err := f.core.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	_, collected, err = f.econ.depot.WithdrawItem(ctx, f.sender.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(collected) != 1 || collected[0].Amount != 150 || collected[0].Source != depot.RemittanceFleaMarketPayment || collected[0].SenderName != f.recipient.Name {
		t.Fatalf("second collected = %+v", collected)
	}
	final := f.snapshot(t)
	if final.Money[0] != corecharacter.MaxMoney-50 || len(final.Held[0]) != 0 {
		t.Fatalf("final state = %+v", final)
	}
	if _, _, err := f.econ.depot.WithdrawItem(ctx, f.sender.ID, ""); !errors.Is(err, depot.ErrInvalidItemInstanceID) {
		t.Fatalf("repeat collection after empty = %v", err)
	}
}
