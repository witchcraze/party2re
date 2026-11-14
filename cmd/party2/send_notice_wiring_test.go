package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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
	assertNotice := func(wantItem, wantMoney int) {
		t.Helper()
		notices, err := soc.home.ListDeliveryNotices(ctx, f.recipient.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, n := range notices {
			got[n.NoticeType]++
		}
		if got[depot.NoticeItemTransfer] != wantItem || got[depot.NoticeMoneyTransfer] != wantMoney {
			t.Fatalf("production notices = %+v", notices)
		}
	}
	if _, err := f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID); err != nil {
		t.Fatal(err)
	}
	assertNotice(1, 0)
	if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); err != nil {
		t.Fatal(err)
	}
	assertNotice(1, 1)
	bought, err := f.econ.gemStore.BuyGem(ctx, f.sender.ID, "gem_heal_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.econ.gemStore.SendGem(ctx, f.sender.ID, f.recipient.ID, bought.ItemInstance.ID); err != nil {
		t.Fatal(err)
	}
	assertNotice(2, 1)
}

func TestWireDepotNoticeRollback_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, false)
	soc, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f.econ.depot.SetDeliveryNotifier(failAfterNotice{soc.home})
	before := f.state(t)
	if _, err := f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID); !errors.Is(err, errNoticeAfterWrite) {
		t.Fatalf("failing item send: %v", err)
	}
	if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); !errors.Is(err, errNoticeAfterWrite) {
		t.Fatalf("failing money send: %v", err)
	}
	notices, err := soc.home.ListDeliveryNotices(ctx, f.recipient.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if after := f.state(t); !reflect.DeepEqual(before, after) || len(notices) != 0 {
		t.Fatal("failed sends changed assets or left notices")
	}
	// Rebuild the production connection for retry, without duplicating its option line.
	if _, err := newSocServices(f.db, f.core, f.econ, nil, logging.Nop()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.econ.depot.SendMoney(ctx, f.sender.ID, f.recipient.ID, 100); err != nil {
		t.Fatal(err)
	}
	notices, err = soc.home.ListDeliveryNotices(ctx, f.recipient.ID, true)
	if err != nil || len(notices) != 2 {
		t.Fatalf("retry notices = %+v, %v", notices, err)
	}
}
