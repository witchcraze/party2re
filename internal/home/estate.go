package home

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/town"
)

type GuildPointsRegistrar interface {
	AddGuildPoints(ctx context.Context, characterID string, points int) error
}

func (s *Service) BuildHouse(ctx context.Context, characterID, townID, houseStyle string) (*TownHouse, error) {
	t, ok := town.GetTown(townID)
	if !ok {
		return nil, ErrInvalidTownID
	}
	if !town.IsValidHouseStyle(townID, houseStyle) {
		return nil, ErrInvalidHouseStyle
	}

	if s.runner == nil {
		return nil, ErrTransactionRunnerRequired
	}

	now := s.nowFunc().UTC()

	req := economy.TransactionRequest{
		CharacterID: characterID,
		Cost: economy.ResourceCost{
			Gold: t.Price,
		},
	}

	var ownerName string
	expiresAt := now.Add(time.Duration(t.CycleDays) * 24 * time.Hour)

	err := s.inTx(ctx, func(txCtx context.Context) error {
		// Rank 0 before the runner's character lock: concurrent builders in one
		// town must not both see a free slot.
		if err := s.repo.LockTownHouseCapacityForUpdate(txCtx, townID); err != nil {
			return err
		}
		_, err := s.runner.ExecuteTransaction(txCtx, req, func(tc *economy.TxContext) error {
			ownerName = tc.Character.Name

			// 1-house rule globally: check existing home
			existingHome, err := s.repo.GetHome(tc.Context, characterID)
			if err != nil {
				return err
			}
			if existingHome.IsActive(now) {
				return ErrAlreadyOwnsHouse
			}

			// Max 10 houses in town
			count, err := s.repo.CountActiveTownHouses(tc.Context, townID, now)
			if err != nil {
				return err
			}
			if count >= t.MaxHouses {
				return ErrTownMaxHousesReached
			}

			// Award guild points if applicable: cycle_days * 10
			if s.guildPoints != nil {
				if err := s.guildPoints.AddGuildPoints(tc.Context, characterID, t.CycleDays*10); err != nil {
					return err
				}
			}

			existingHome.CharacterID = characterID
			existingHome.TownID = t.ID
			existingHome.HouseStyle = houseStyle
			existingHome.ExpiresAt = &expiresAt
			existingHome.UpdatedAt = now

			return s.repo.SaveHome(tc.Context, existingHome)
		})
		return err
	})
	if err != nil {
		if errors.Is(err, economy.ErrInsufficientGold) {
			return nil, ErrInsufficientFunds
		}
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil, ErrCharacterNotFound
		}
		return nil, err
	}

	// Set Valkey TTL timer cache
	if s.timer != nil {
		_ = s.timer.SetLock(ctx, timer.CategoryHouse, characterID, time.Duration(t.CycleDays)*24*time.Hour)
	}

	return &TownHouse{CharacterID: characterID, OwnerName: ownerName, TownID: t.ID, HouseStyle: houseStyle, ExpiresAt: expiresAt}, nil
}

// ListTownHouses lists the active houses standing in a town, soonest expiry first.
func (s *Service) ListTownHouses(ctx context.Context, townID string) ([]TownHouse, error) {
	if _, ok := town.GetTown(townID); !ok {
		return nil, ErrInvalidTownID
	}
	return s.repo.ListActiveTownHouses(ctx, townID, s.nowFunc().UTC())
}

func (s *Service) SetCharacterColor(ctx context.Context, characterID, color string) error {
	if s.runner == nil {
		return ErrTransactionRunnerRequired
	}
	// The runner locks the character row and persists the full state, so a
	// concurrent reward or spend that committed after our read is not reverted.
	_, err := s.runner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: characterID}, func(tc *economy.TxContext) error {
		return tc.Character.SetColor(color)
	})
	if errors.Is(err, economy.ErrCharacterNotFound) {
		return ErrCharacterNotFound
	}
	return err
}

// inTx runs fn in an outer transaction when a provider is configured.
// shortcut: without a provider (in-memory unit tests) fn runs untransacted; production wires one.
func (s *Service) inTx(ctx context.Context, fn func(context.Context) error) error {
	if s.txProvider == nil {
		return fn(ctx)
	}
	return s.txProvider.RunInTx(ctx, fn)
}
