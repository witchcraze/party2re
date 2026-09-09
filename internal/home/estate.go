package home

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/town"
)

type GuildPointsRegistrar interface {
	AddGuildPoints(ctx context.Context, characterID string, points int) error
}

func (s *Service) BuildHouse(ctx context.Context, characterID, townID, houseStyle string) (*HomeCheckResult, error) {
	t, ok := town.GetTown(townID)
	if !ok {
		return nil, ErrInvalidTownID
	}
	if !town.IsValidHouseStyle(townID, houseStyle) {
		return nil, ErrInvalidHouseStyle
	}

	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil, ErrCharacterNotFound
		}
		return nil, err
	}

	now := s.nowFunc().UTC()

	// 1-house rule globally: check existing home
	existingHome, err := s.repo.GetHome(ctx, characterID)
	if err == nil && existingHome.IsActive(now) {
		return nil, ErrAlreadyOwnsHouse
	}

	if char.Money < t.Price {
		return nil, ErrInsufficientFunds
	}

	// Max 10 houses in town
	count, err := s.repo.CountActiveTownHouses(ctx, townID, now)
	if err != nil {
		return nil, err
	}
	if count >= t.MaxHouses {
		return nil, ErrTownMaxHousesReached
	}

	// Deduct funds
	if err := char.DeductMoney(t.Price); err != nil {
		return nil, err
	}
	if err := s.charUpdater.Update(ctx, char); err != nil {
		return nil, err
	}

	// Award guild points if applicable: cycle_days * 10
	if s.guildPoints != nil {
		_ = s.guildPoints.AddGuildPoints(ctx, characterID, t.CycleDays*10)
	}

	// Calculate expiration: cycle_days * 24h
	expiresAt := now.Add(time.Duration(t.CycleDays) * 24 * time.Hour)
	existingHome.CharacterID = characterID
	existingHome.TownID = t.ID
	existingHome.HouseStyle = houseStyle
	existingHome.ExpiresAt = &expiresAt
	existingHome.UpdatedAt = now
	if existingHome.CompanionName == "" {
		existingHome.CompanionName = DefaultCompanionName
	}

	if err := s.repo.SaveHome(ctx, existingHome); err != nil {
		return nil, err
	}

	// Set Valkey TTL timer cache
	if s.timer != nil {
		_ = s.timer.SetLock(ctx, timer.CategoryHouse, characterID, time.Duration(t.CycleDays)*24*time.Hour)
	}

	jstExpires := expiresAt.In(timer.JST)
	msg := fmt.Sprintf("<b>%s の家</b>の所有期間は %d月%d日%d時 までです", char.Name, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour())

	return &HomeCheckResult{
		CharacterID: characterID,
		OwnerName:   char.Name,
		TownID:      t.ID,
		TownName:    t.Name,
		HouseStyle:  houseStyle,
		ExpiresAt:   expiresAt,
		Message:     msg,
	}, nil
}

func (s *Service) CheckHouse(ctx context.Context, targetNameOrID string) (*HomeCheckResult, error) {
	now := s.nowFunc().UTC()

	// Try find by character ID first
	h, err := s.repo.FindActiveHomeByCharacterID(ctx, targetNameOrID, now)
	var char corecharacter.Character
	if err == nil {
		char, err = s.charReader.FindByID(ctx, h.CharacterID)
		if err != nil {
			return nil, err
		}
	} else if errors.Is(err, ErrHouseNotFound) {
		// Try find by character Name
		var c corecharacter.Character
		h, c, err = s.repo.FindActiveHomeByCharacterName(ctx, targetNameOrID, now)
		if err != nil {
			return nil, err
		}
		char = c
	} else {
		return nil, err
	}

	t, _ := town.GetTown(h.TownID)
	townName := h.TownID
	if t.Name != "" {
		townName = t.Name
	}

	var expiresAt time.Time
	if h.ExpiresAt != nil {
		expiresAt = *h.ExpiresAt
	}
	jstExpires := expiresAt.In(timer.JST)
	msg := fmt.Sprintf("<b>%s の家</b>の所有期間は %d月%d日%d時 までです", char.Name, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour())

	return &HomeCheckResult{
		CharacterID: char.ID,
		OwnerName:   char.Name,
		TownID:      h.TownID,
		TownName:    townName,
		HouseStyle:  h.HouseStyle,
		ExpiresAt:   expiresAt,
		Message:     msg,
	}, nil
}

func (s *Service) ListTownHouses(ctx context.Context, townID string) ([]HomeCheckResult, error) {
	t, ok := town.GetTown(townID)
	if !ok {
		return nil, ErrInvalidTownID
	}
	now := s.nowFunc().UTC()
	homes, err := s.repo.ListActiveTownHouses(ctx, townID, now)
	if err != nil {
		return nil, err
	}

	results := make([]HomeCheckResult, 0, len(homes))
	for _, h := range homes {
		char, err := s.charReader.FindByID(ctx, h.CharacterID)
		ownerName := h.CharacterID
		if err == nil {
			ownerName = char.Name
		}
		var exp time.Time
		if h.ExpiresAt != nil {
			exp = *h.ExpiresAt
		}
		jstExpires := exp.In(timer.JST)
		msg := fmt.Sprintf("<b>%s の家</b>の所有期間は %d月%d日%d時 までです", ownerName, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour())
		results = append(results, HomeCheckResult{
			CharacterID: h.CharacterID,
			OwnerName:   ownerName,
			TownID:      t.ID,
			TownName:    t.Name,
			HouseStyle:  h.HouseStyle,
			ExpiresAt:   exp,
			Message:     msg,
		})
	}
	return results, nil
}

func (s *Service) SetCharacterColor(ctx context.Context, characterID, color string) error {
	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return ErrCharacterNotFound
		}
		return err
	}
	if err := char.SetColor(color); err != nil {
		return err
	}
	return s.charUpdater.Update(ctx, char)
}
