package depot

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type recordedNotice struct {
	characterID, noticeType, message string
	flag                             bool
}

type recordingNotifier struct {
	notices []recordedNotice
	err     error
}

func (n *recordingNotifier) AddDeliveryNotice(_ context.Context, characterID, noticeType, message string) error {
	n.notices = append(n.notices, recordedNotice{characterID, noticeType, message, false})
	return n.err
}

func (n *recordingNotifier) AddDeliveryFlag(_ context.Context, characterID, noticeType, message string) error {
	n.notices = append(n.notices, recordedNotice{characterID, noticeType, message, true})
	return n.err
}

// send_item names the sender and item; send_money only raises a flag (system.cgi:818–855).
func TestSendNotifiesOnlyTheRecipient(t *testing.T) {
	ctx := context.Background()
	depots, chars, invs := newMemoryDepotRepo(), newMemoryCharRepo(), newMemoryInvRepo()
	sender, _ := corecharacter.New("Sender")
	sender.Money = 1000
	recipient, _ := corecharacter.New("Recip")
	chars.characters[sender.ID], chars.characters[recipient.ID] = sender, recipient
	inv, _ := coreinventory.New(sender.ID)
	sword := item.Instance{ID: "sword", DefinitionID: "wea-01", Quantity: 1}
	_ = inv.Add(sword)
	invs.inventories[sender.ID] = inv
	notifier := &recordingNotifier{}
	svc, _ := NewService(depots, chars, invs, WithTransactionProvider(&memoryTxProvider{}),
		WithRemittanceRepository(depots), WithItemDefinitionProvider(newMemoryItemCatalog()))
	svc.SetDeliveryNotifier(notifier)

	if _, err := svc.SendMoney(ctx, sender.ID, recipient.ID, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendItem(ctx, sender.ID, recipient.ID, sword.ID); err != nil {
		t.Fatal(err)
	}
	want := []recordedNotice{
		{recipient.ID, NoticeMoneyTransfer, "お金が預かり所に届いています", true},
		{recipient.ID, NoticeItemTransfer, "預かり所にSenderからIron Swordが届いています", false},
	}
	if !reflect.DeepEqual(notifier.notices, want) {
		t.Fatalf("notices = %+v", notifier.notices)
	}

	// Rejected sends notify nobody.
	notifier.notices = nil
	if _, err := svc.SendMoney(ctx, sender.ID, recipient.ID, 5000); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("overdraft: %v", err)
	}
	if _, err := svc.SendItem(ctx, sender.ID, recipient.ID, sword.ID); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("missing item: %v", err)
	}
	if len(notifier.notices) != 0 {
		t.Fatalf("rejected sends notified: %+v", notifier.notices)
	}

	// A notice failure fails the send so its transaction rolls back.
	notifier.err = errTestBoom
	if _, err := svc.SendMoney(ctx, sender.ID, recipient.ID, 1); !errors.Is(err, errTestBoom) {
		t.Fatalf("notice failure: %v", err)
	}
}
