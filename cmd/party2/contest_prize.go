package main

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

// contestPrizeDelivery connects contest awards to held gold and Depot delivery.
type contestPrizeDelivery struct {
	remittances depot.RemittanceRepository
	depots      depot.DepotDeliveryRepository
	notifier    depot.DeliveryNotifier
	items       depot.ItemDefinitionProvider
}

func (d contestPrizeDelivery) HoldGold(ctx context.Context, characterID string, amount int, award string) error {
	if err := d.remittances.AddRemittance(ctx, depot.Remittance{
		ID: id.New(), CharacterID: characterID, Amount: amount,
		Source: depot.RemittanceContestPrize, SenderName: award,
	}); err != nil {
		return err
	}
	return d.notifier.AddDeliveryFlag(ctx, characterID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}

func (d contestPrizeDelivery) DeliverMedals(ctx context.Context, recipient corecharacter.Character, quantity, notified int) error {
	inst, err := item.NewInstance(contest.PrizeMedalItemID, quantity)
	if err != nil {
		return err
	}
	dep, err := depot.FindOrCreate(ctx, d.depots, recipient)
	if err != nil {
		return err
	}
	dep.Deliver(inst)
	if err := d.depots.Save(ctx, dep); err != nil {
		return err
	}
	if notified == 0 {
		return nil
	}
	def, err := d.items.FindByID(contest.PrizeMedalItemID)
	if err != nil {
		return err
	}
	for range notified {
		if err := d.notifier.AddDeliveryNotice(ctx, recipient.ID, depot.NoticeItemTransfer,
			fmt.Sprintf("預かり所にフォトコンテストから%sが届いています", def.Name)); err != nil {
			return err
		}
	}
	return nil
}
