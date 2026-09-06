package tavern

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// DeliveryReservation represents a reserved meal to be eaten upon adventure completion.
type DeliveryReservation struct {
	CharacterID string    `json:"character_id"`
	ItemID      string    `json:"item_id"`
	ItemName    string    `json:"item_name"`
	Price       int       `json:"price"`
	HPHeal      int       `json:"hp_heal"`
	MPHeal      int       `json:"mp_heal"`
	Tickets     int       `json:"tickets"`
	CreatedAt   time.Time `json:"created_at"`
}

// ReserveDelivery sets a pending delivery order.
func (s *Service) ReserveDelivery(ctx context.Context, characterID string, itemID string) (DeliveryReservation, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return DeliveryReservation{}, ErrInvalidCharacterID
	}

	item, ok := s.catalog.GetItem(itemID)
	if !ok {
		return DeliveryReservation{}, ErrMenuItemNotFound
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return DeliveryReservation{}, ErrCharacterNotFound
		}
		return DeliveryReservation{}, err
	}

	if char.Money < item.Price {
		return DeliveryReservation{}, ErrInsufficientFunds
	}

	delivery := DeliveryReservation{
		CharacterID: charID,
		ItemID:      item.ID,
		ItemName:    item.Name,
		Price:       item.Price,
		HPHeal:      item.HPHeal,
		MPHeal:      item.MPHeal,
		Tickets:     item.Tickets,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.SaveDelivery(ctx, delivery); err != nil {
		return DeliveryReservation{}, err
	}
	return delivery, nil
}

// GetDelivery returns the current delivery reservation for a character.
func (s *Service) GetDelivery(ctx context.Context, characterID string) (DeliveryReservation, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return DeliveryReservation{}, ErrInvalidCharacterID
	}

	delivery, err := s.repo.GetDelivery(ctx, charID)
	if err != nil {
		return DeliveryReservation{}, ErrNoActiveDelivery
	}
	return delivery, nil
}

// CancelDelivery cancels the active delivery reservation.
func (s *Service) CancelDelivery(ctx context.Context, characterID string) error {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return ErrInvalidCharacterID
	}

	_, err := s.repo.GetDelivery(ctx, charID)
	if err != nil {
		return ErrNoActiveDelivery
	}

	return s.repo.DeleteDelivery(ctx, charID)
}

// ClaimDelivery consumes the active delivery meal (e.g. after adventure).
func (s *Service) ClaimDelivery(ctx context.Context, characterID string) (OrderResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return OrderResult{}, ErrInvalidCharacterID
	}

	var result OrderResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		delivery, err := s.repo.GetDelivery(txCtx, charID)
		if err != nil || delivery.ItemID == "" {
			return ErrNoActiveDelivery
		}

		item, ok := s.catalog.GetItem(delivery.ItemID)
		if !ok {
			item = MenuItem{
				ID:       delivery.ItemID,
				Name:     delivery.ItemName,
				Price:    delivery.Price,
				HPHeal:   delivery.HPHeal,
				MPHeal:   delivery.MPHeal,
				Tickets:  delivery.Tickets,
				Category: "Delivery",
			}
		}

		char, err := s.charRepo.FindByIDForUpdate(txCtx, charID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		if char.Money < delivery.Price {
			return ErrInsufficientFunds
		}

		// Calculate HP/MP healing
		hpHealed := 0
		if delivery.HPHeal > 0 {
			missingHP := char.Stats.MaxHP - char.Stats.HP
			if missingHP > 0 {
				if delivery.HPHeal > missingHP {
					hpHealed = missingHP
				} else {
					hpHealed = delivery.HPHeal
				}
				char.Stats.HP += hpHealed
			}
		}

		mpHealed := 0
		if delivery.MPHeal > 0 {
			missingMP := char.Stats.MaxMP - char.Stats.MP
			if missingMP > 0 {
				if delivery.MPHeal > missingMP {
					mpHealed = missingMP
				} else {
					mpHealed = delivery.MPHeal
				}
				char.Stats.MP += mpHealed
			}
		}

		if err := char.DeductMoney(delivery.Price); err != nil {
			return ErrInsufficientFunds
		}

		if err := s.charRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character after delivery meal: %w", err)
		}

		// Award lottery tickets
		totalTickets := 0
		if s.lotteryRepo != nil && delivery.Tickets > 0 {
			newTickets, err := s.lotteryRepo.AddRaffleTickets(txCtx, charID, delivery.Tickets)
			if err == nil {
				totalTickets = newTickets
			}
		}

		// Update tavern status
		status, err := s.repo.GetCharacterStatus(txCtx, charID)
		if err != nil {
			status = TavernCharacterStatus{CharacterID: charID}
		}
		now := time.Now().UTC()
		status.IsFull = true
		status.LastEatenAt = &now
		status.TotalMealsEaten++
		status.TotalGoldSpent += int64(delivery.Price)
		status.UpdatedAt = now

		if err := s.repo.UpsertCharacterStatus(txCtx, status); err != nil {
			return fmt.Errorf("failed to update tavern character status: %w", err)
		}

		if err := s.repo.DeleteDelivery(txCtx, charID); err != nil {
			return fmt.Errorf("failed to delete claimed delivery: %w", err)
		}

		msg := fmt.Sprintf("配達完了！冒険帰りの体に染み渡る%sが届いたわ♪", delivery.ItemName)
		if hpHealed > 0 && mpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d、MPが%d回復した！", hpHealed, mpHealed)
		} else if hpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d回復した！", hpHealed)
		} else if mpHealed > 0 {
			msg += fmt.Sprintf(" MPが%d回復した！", mpHealed)
		}
		if delivery.Tickets > 0 {
			msg += fmt.Sprintf(" 福引券を%d枚もらった！", delivery.Tickets)
		}

		result = OrderResult{
			CharacterID:    char.ID,
			Item:           item,
			HPHealed:       hpHealed,
			MPHealed:       mpHealed,
			CurrentHP:      char.Stats.HP,
			CurrentMP:      char.Stats.MP,
			RemainingGold:  char.Money,
			TicketsAwarded: delivery.Tickets,
			TotalTickets:   totalTickets,
			Message:        msg,
		}
		return nil
	})

	if err != nil {
		return OrderResult{}, err
	}
	return result, nil
}
