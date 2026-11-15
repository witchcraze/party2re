package main

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

type fleaMarketDelivery struct {
	remittances depot.RemittanceRepository
	notifier    depot.DeliveryNotifier
}

func (d fleaMarketDelivery) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	if d.remittances == nil || d.notifier == nil {
		return errors.New("fleamarket delivery dependencies are required")
	}
	if err := d.remittances.AddRemittance(ctx, depot.Remittance{
		ID:          id.New(),
		CharacterID: recipientID,
		SenderName:  buyerName,
		Amount:      amount,
		Source:      depot.RemittanceFleaMarketPayment,
	}); err != nil {
		return err
	}
	return d.notifier.AddDeliveryFlag(ctx, recipientID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}
