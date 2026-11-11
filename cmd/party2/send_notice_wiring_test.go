package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
)

var errNoticeAfterWrite = errors.New("notice failed after write")

// failAfterNotice writes the real notice, then fails the send.
type failAfterNotice struct{ depot.DeliveryNotifier }

func (f failAfterNotice) AddDeliveryNotice(ctx context.Context, characterID, noticeType, message string) error {
	if err := f.DeliveryNotifier.AddDeliveryNotice(ctx, characterID, noticeType, message); err != nil {
		return err
	}
	return errNoticeAfterWrite
}

func (f failAfterNotice) AddDeliveryFlag(ctx context.Context, characterID, noticeType, message string) error {
	if err := f.DeliveryNotifier.AddDeliveryFlag(ctx, characterID, noticeType, message); err != nil {
		return err
	}
	return errNoticeAfterWrite
}

func TestWireSendsNotifyRecipientHome_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, false)
	soc, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	notices := func(characterID string) []home.DeliveryNotice {
		t.Helper()
		got, err := soc.home.ListDeliveryNotices(ctx, characterID, true)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// A failed send rolls back its notice with the assets; the retry notifies once.
	f.econ.depot.SetDeliveryNotifier(failAfterNotice{soc.home})
	before := f.state(t)
	if _, err := f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID); !errors.Is(err, errNoticeAfterWrite) {
		t.Fatalf("failing item send: %v", err)
	}
	if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); !errors.Is(err, errNoticeAfterWrite) {
		t.Fatalf("failing money send: %v", err)
	}
	if after := f.state(t); !reflect.DeepEqual(before, after) || len(notices(f.recipient.ID)) != 0 {
		t.Fatal("failed sends changed assets or left notices")
	}
	f.econ.depot.SetDeliveryNotifier(soc.home)

	if _, err := f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); err != nil {
			t.Fatal(err)
		}
	}
	def, err := f.core.itemCatalog.FindByID(f.instance.DefinitionID)
	if err != nil {
		t.Fatal(err)
	}
	got := notices(f.recipient.ID)
	types := map[string]string{}
	for _, n := range got {
		types[n.NoticeType] = n.Message
	}
	want := map[string]string{
		depot.NoticeItemTransfer:  "預かり所に" + f.sender.Name + "から" + def.Name + "が届いています",
		depot.NoticeMoneyTransfer: "お金が預かり所に届いています",
	}
	if len(got) != 2 || !reflect.DeepEqual(types, want) {
		t.Fatalf("recipient notices = %+v", got)
	}
	if n := notices(f.sender.ID); len(n) != 0 {
		t.Fatalf("sender notices = %+v", n)
	}

	// Clearing re-arms the money flag.
	if err := soc.home.ClearDeliveryNotices(ctx, f.recipient.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); err != nil {
		t.Fatal(err)
	}
	if got := notices(f.recipient.ID); len(got) != 1 || got[0].NoticeType != depot.NoticeMoneyTransfer {
		t.Fatalf("after clear = %+v", got)
	}

	// Gem sends use the same production notifier.
	bought, err := f.econ.gemStore.BuyGem(ctx, f.sender.ID, "gem_heal_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.econ.gemStore.SendGem(ctx, f.sender.ID, f.recipient.ID, bought.ItemInstance.ID); err != nil {
		t.Fatal(err)
	}
	if got := notices(f.recipient.ID); len(got) != 2 || got[0].NoticeType != depot.NoticeItemTransfer {
		t.Fatalf("after gem send = %+v", got)
	}
}
