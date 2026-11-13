package altar

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

// GetStatus queries the current altar status, Ramia presence, and character progression.
func (s *Service) GetStatus(ctx context.Context, characterID string) (AltarStatus, error) {
	char, err := s.chars.FindByID(ctx, characterID)
	if err != nil {
		return AltarStatus{}, err
	}

	now := s.nowFunc()
	latestAwakening, err := s.altars.GetLatestAwakening(ctx)
	if err != nil {
		return AltarStatus{}, err
	}

	ramiaPresent := latestAwakening != nil && latestAwakening.IsActive(now)
	var expiresAt *timeTimePtr
	var awakenedBy string
	if ramiaPresent {
		t := latestAwakening.ExpiresAt
		expiresAt = &t
		awakenedBy = latestAwakening.CharacterName
	}

	availableItems := []WishItemInfo{}
	if char.IsRamiaAwakened() {
		for _, itemID := range AllowedWishItems {
			def, defErr := s.itemDefs.FindByID(itemID)
			name := itemID
			if defErr == nil {
				name = def.Name
			}
			availableItems = append(availableItems, WishItemInfo{
				ID:   itemID,
				Name: name,
			})
		}
	}

	return AltarStatus{
		CharacterID:    char.ID,
		Orbs:           char.Orb,
		OrbCount:       char.OrbCount(),
		HasAllOrbs:     char.HasAllOrbs(),
		RamiaAwakened:  char.IsRamiaAwakened(),
		RamiaPresent:   ramiaPresent,
		RamiaExpiresAt: expiresAt,
		AwakenedBy:     awakenedBy,
		AvailableItems: availableItems,
	}, nil
}

// Pray implements legacy @いのる. Requires all 6 orbs.
func (s *Service) Pray(ctx context.Context, characterID string) (PrayResult, error) {
	var result PrayResult
	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Character lock
		char, err := s.chars.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Legacy offers ＠いのる only before Ramia awakens.
		if char.IsRamiaAwakened() {
			return ErrRamiaAwakened
		}
		if !char.HasAllOrbs() {
			return ErrInsufficientOrbs
		}

		now := s.nowFunc()
		awakening := RamiaAwakening{
			ID:            id.New(),
			CharacterID:   char.ID,
			CharacterName: char.Name,
			AwakenedAt:    now,
			ExpiresAt:     now.Add(RamiaStayDuration),
		}

		char.SetRamiaAwakened()
		if err := s.chars.Update(txCtx, char); err != nil {
			return err
		}

		// Rank 8: Secondary feature record
		if err := s.altars.SaveAwakening(txCtx, awakening); err != nil {
			return err
		}

		result = PrayResult{
			CharacterID:   char.ID,
			Message:       MsgRamiaAwakening,
			RamiaAwakened: true,
			ExpiresAt:     awakening.ExpiresAt,
		}
		return nil
	})
	if err != nil {
		return PrayResult{}, err
	}
	return result, nil
}

// Wish implements legacy @ねがう. Grants one of the 4 otherworld travel items.
func (s *Service) Wish(ctx context.Context, characterID string, itemID string) (WishResult, error) {
	if !IsValidWishItem(itemID) {
		return WishResult{}, ErrInvalidWishItem
	}

	itemDef, err := s.itemDefs.FindByID(itemID)
	if err != nil {
		return WishResult{}, err
	}

	var result WishResult
	err = s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Character lock
		char, err := s.chars.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if !char.IsRamiaAwakened() {
			return ErrRamiaNotAwakened
		}

		// Rank 3: Inventory lock
		inv, err := s.invs.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		deliveredTo := "inventory"
		var mes string

		if !inv.IsFull(itemID) {
			inst, err := coreitem.NewInstance(itemID, 1)
			if err != nil {
				return err
			}
			if err := inv.Add(inst); err != nil {
				return err
			}
			if err := s.invs.Save(txCtx, inv); err != nil {
				return err
			}
			if s.collector != nil {
				cat := string(itemDef.Slot)
				if cat == "" {
					cat = "consumable"
				}
				if err := s.collector.RecordItemDiscovered(txCtx, char.ID, itemDef.ID, itemDef.Name, cat); err != nil {
					return err
				}
			}
			mes = fmt.Sprintf(MsgWishInventoryFmt, itemDef.Name)
		} else {
			// Rank 5: Depot lock
			deliveredTo = "depot"
			dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
			if errors.Is(err, depot.ErrNotFound) {
				dep, err = depot.NewDepotWithCapacity(char.ID, char.JobLevel, 0, char.OverDepot)
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			dep.RefreshCapacity(char.JobLevel, char.OverDepot)

			inst, err := coreitem.NewInstance(itemID, 1)
			if err != nil {
				return err
			}
			dep.Deliver(inst)
			if err := s.depots.Save(txCtx, dep); err != nil {
				return err
			}
			mes = fmt.Sprintf(MsgWishDepotFmt, itemDef.Name, char.Name)
		}

		// Clear orbs upon successful wish
		char.ClearOrbs()
		if err := s.chars.Update(txCtx, char); err != nil {
			return err
		}

		result = WishResult{
			CharacterID: char.ID,
			ItemID:      itemDef.ID,
			ItemName:    itemDef.Name,
			DeliveredTo: deliveredTo,
			Message:     mes,
		}
		return nil
	})
	if err != nil {
		return WishResult{}, err
	}
	return result, nil
}

type timeTimePtr = time.Time
