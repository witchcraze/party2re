package main

import (
	"context"

	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

type pvpPrizeRemitter struct {
	remittances depot.RemittanceRepository
	notifier    interface {
		AddDeliveryFlag(context.Context, string, string, string) error
	}
}

func (p pvpPrizeRemitter) HoldPrize(ctx context.Context, characterID, arenaName string, amount int) error {
	if err := p.remittances.AddRemittance(ctx, depot.Remittance{
		ID: id.New(), CharacterID: characterID, Amount: amount,
		Source: depot.RemittancePvPPrize, SenderName: arenaName,
	}); err != nil {
		return err
	}
	return p.notifier.AddDeliveryFlag(ctx, characterID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}
