package main

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
)

type referralRecipientFinder interface {
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
}

type referralDeliveryAdapter struct {
	characters referralRecipientFinder
	depots     depot.Repository
	home       *home.Service
	items      item.DefinitionProvider
}

// DeliverReferral joins the creation transaction and locks the passive recipient character before depot storage.
// Lock order: recipient character(2), then character_depots/depot_items(5), then delivery notice insert.
func (a referralDeliveryAdapter) DeliverReferral(ctx context.Context, referrerID string, joining corecharacter.Character) error {
	if a.characters != nil {
		if _, err := a.characters.FindByIDForUpdate(ctx, referrerID); errors.Is(err, corecharacter.ErrNotFound) {
			return nil // Missing recipient is an approved no-op.
		} else if err != nil {
			return err
		}
	}
	dep, err := a.depots.FindByCharacterIDForUpdate(ctx, referrerID)
	if errors.Is(err, depot.ErrNotFound) {
		return nil // Legacy send_item requires an existing recipient depot.
	}
	if err != nil {
		return err
	}
	definition, err := a.items.FindByID("item-023")
	if err != nil {
		return err
	}
	reward, err := definition.NewInstance(1)
	if err != nil {
		return err
	}
	dep.AppendDelivery(reward)
	if err := a.depots.Save(ctx, dep); err != nil {
		return err
	}
	return a.home.AddDeliveryNotice(ctx, referrerID, "referral_registration",
		fmt.Sprintf("預かり所に%s(紹介加入)から%sが届いています (character_id: %s)", joining.Name, definition.Name, joining.ID))
}
