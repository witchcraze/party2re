package main

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/store"
)

type storeDelivery struct {
	remittances depot.RemittanceRepository
	notifier    depot.DeliveryNotifier
	items       store.ItemCatalog
}

func (d storeDelivery) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	if err := d.remittances.AddRemittance(ctx, depot.Remittance{
		ID: id.New(), CharacterID: recipientID, Amount: amount,
		Source: depot.RemittanceStorePayment, SenderName: buyerName,
	}); err != nil {
		return err
	}
	return d.notifier.AddDeliveryFlag(ctx, recipientID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}

func (d storeDelivery) NotifyItem(ctx context.Context, recipientID, senderName, itemID string) error {
	def, err := d.items.FindByID(itemID)
	if err != nil {
		return err
	}
	return d.notifier.AddDeliveryNotice(ctx, recipientID, depot.NoticeItemTransfer,
		fmt.Sprintf("預かり所に%sから%sが届いています", senderName, def.Name))
}
