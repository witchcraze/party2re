package customskill

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
)

func (s *Service) GetCustomSkill(ctx context.Context, characterID string) (*CustomSkill, error) {
	return s.repo.FindCustomSkill(ctx, characterID)
}

func (s *Service) SetCustomSkill(ctx context.Context, characterID, name, comment string, gems [3]string) (*CustomSkill, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if err := validateText(name, true); err != nil {
		return nil, err
	}
	if err := validateText(comment, false); err != nil {
		return nil, err
	}
	if s.gems == nil || s.gemBox == nil {
		return nil, ErrGemDependencies
	}
	var result *CustomSkill
	run := func(txCtx context.Context) error {
		char, err := s.charRepo.FindByID(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		expectedCap := gemstore.CalculateGemBoxCapacity(char.JobLevel)
		box, err := s.gemBox.FindByCharacterIDForUpdate(txCtx, characterID)
		if errors.Is(err, gemstore.ErrGemBoxNotFound) {
			box = gemstore.GemBox{
				CharacterID: characterID,
				Capacity:    expectedCap,
				Items:       []coreitem.Instance{},
			}
		} else if err != nil {
			return err
		}
		box.Capacity = expectedCap

		var slotTotal, cmpTotal int
		required := make(map[string]int)
		for _, gemID := range gems {
			if gemID == "" {
				continue
			}
			gem, found := s.gems.FindGemByID(gemID)
			if !found {
				return ErrGemNotFound
			}
			slotTotal += gem.SlotCost
			cmpTotal += gem.MPCost
			required[gemID]++
			if slotTotal > 3 {
				return ErrTooManyGemSlots
			}
		}

		previous, err := s.repo.FindCustomSkill(txCtx, characterID)
		if err != nil {
			return err
		}
		if previous != nil {
			for _, oldGem := range previous.Gems {
				if oldGem == "" {
					continue
				}
				instance, createErr := coreitem.NewInstance(oldGem, 1)
				if createErr != nil {
					return createErr
				}
				box.Items = append(box.Items, instance)
			}
		}

		for gemID, count := range required {
			owned := 0
			for _, item := range box.Items {
				if item.DefinitionID == gemID {
					owned++
				}
			}
			if owned < count {
				return ErrGemNotOwned
			}
		}

		for _, gemID := range gems {
			if gemID == "" {
				continue
			}
			if _, err := box.RemoveItem(gemID); err != nil {
				return ErrGemNotOwned
			}
		}

		if box.Count() > box.Capacity {
			return gemstore.ErrGemBoxFull
		}

		if err := s.gemBox.Save(txCtx, box); err != nil {
			return err
		}

		result = &CustomSkill{
			CharacterID: characterID,
			Name:        name,
			Comment:     comment,
			CMP:         cmpTotal,
			Gems:        gems,
			UpdatedAt:   time.Now().UTC(),
		}
		return s.repo.SaveCustomSkill(txCtx, *result)
	}
	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return nil, err
		}
	} else if err := run(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func validateText(value string, name bool) error {
	if utf8.RuneCountInString(value) > 60 || strings.ContainsAny(value, ";<>") {
		if name {
			return ErrInvalidSkillName
		}
		return ErrInvalidSkillComment
	}
	if name {
		if value == "" || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
			return ErrInvalidSkillName
		}
		for _, blocked := range []string{"こうげき", "ぼうぎょ", "てんしょん", "ささやき", "にげる", "すくしょ", "すすむ"} {
			if value == blocked {
				return ErrInvalidSkillName
			}
		}
	}
	return nil
}
