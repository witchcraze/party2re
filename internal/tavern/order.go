package tavern

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// OrderMeal executes an immediate meal purchase and consumption.
func (s *Service) OrderMeal(ctx context.Context, characterID string, itemID string) (OrderResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return OrderResult{}, ErrInvalidCharacterID
	}

	item, ok := s.catalog.GetItem(itemID)
	if !ok {
		return OrderResult{}, ErrMenuItemNotFound
	}

	var result OrderResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, charID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		status, err := s.repo.GetCharacterStatus(txCtx, charID)
		if err != nil {
			return err
		}

		if status.IsFull {
			return ErrAlreadyFull
		}

		if char.Money < item.Price {
			return ErrInsufficientFunds
		}

		// Calculate HP/MP healing
		hpHealed := 0
		if item.HPHeal > 0 {
			missingHP := char.Stats.MaxHP - char.Stats.HP
			if missingHP > 0 {
				if item.HPHeal > missingHP {
					hpHealed = missingHP
				} else {
					hpHealed = item.HPHeal
				}
				char.Stats.HP += hpHealed
			}
		}

		mpHealed := 0
		if item.MPHeal > 0 {
			missingMP := char.Stats.MaxMP - char.Stats.MP
			if missingMP > 0 {
				if item.MPHeal > missingMP {
					mpHealed = missingMP
				} else {
					mpHealed = item.MPHeal
				}
				char.Stats.MP += mpHealed
			}
		}

		if err := char.DeductMoney(item.Price); err != nil {
			return ErrInsufficientFunds
		}

		if err := s.charRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character after meal: %w", err)
		}

		ticketsAwarded, totalTickets := 0, 0
		if s.lotteryRepo != nil && item.Tickets > 0 {
			totalTickets, err = s.lotteryRepo.AddRaffleTickets(txCtx, charID, item.Tickets)
			if err != nil {
				return fmt.Errorf("failed to award raffle tickets: %w", err)
			}
			ticketsAwarded = item.Tickets
		}

		// +2 GP for the affiliated guild (bar.cgi:118).
		if s.guildPoints != nil {
			if err := s.guildPoints.AddGuildPoints(txCtx, charID, 2); err != nil {
				return fmt.Errorf("failed to award guild points: %w", err)
			}
		}

		// Update tavern status
		now := time.Now().UTC()
		status.IsFull = true
		status.LastEatenAt = &now
		status.TotalMealsEaten++
		status.TotalGoldSpent += int64(item.Price)
		status.UpdatedAt = now

		if err := s.repo.UpsertCharacterStatus(txCtx, status); err != nil {
			return fmt.Errorf("failed to update tavern character status: %w", err)
		}

		result = OrderResult{
			CharacterID:    char.ID,
			Item:           item,
			HPHealed:       hpHealed,
			MPHealed:       mpHealed,
			CurrentHP:      char.Stats.HP,
			CurrentMP:      char.Stats.MP,
			RemainingGold:  char.Money,
			TicketsAwarded: ticketsAwarded,
			TotalTickets:   totalTickets,
		}
		return nil
	})

	if err != nil {
		return OrderResult{}, err
	}
	return result, nil
}
