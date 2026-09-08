package custom_skill

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func (s *Service) ConfigureGemSynthesis(gems GemProvider, inventory InventoryProvider, txProvider TransactionProvider) {
	s.gems = gems
	s.inventory = inventory
	s.txProvider = txProvider
}

func (s *Service) GetCustomSkill(ctx context.Context, characterID string) (*CustomSkill, error) {
	repo, ok := s.repo.(SynthesisRepository)
	if !ok {
		return nil, ErrGemDependencies
	}
	return repo.FindCustomSkill(ctx, characterID)
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
	if s.gems == nil || s.inventory == nil {
		return nil, ErrGemDependencies
	}
	repo, ok := s.repo.(SynthesisRepository)
	if !ok {
		return nil, ErrGemDependencies
	}
	var result *CustomSkill
	run := func(txCtx context.Context) error {
		inventory, err := s.inventory.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
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
			if inventory.Quantity(gemID) < required[gemID] {
				return ErrGemNotOwned
			}
		}
		char, err := s.charRepo.FindByID(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if cmpTotal > char.Stats.MaxMP {
			return ErrCMPTooHigh
		}
		previous, err := repo.FindCustomSkill(txCtx, characterID)
		if err != nil {
			return err
		}
		if previous != nil {
			for _, oldGem := range previous.Gems {
				if oldGem == "" {
					continue
				}
				instance, found := firstItem(inventory, oldGem)
				if found {
					instance.Quantity++
					if err := inventory.Update(instance); err != nil {
						return err
					}
				} else {
					instance, err := coreitem.NewInstance(oldGem, 1)
					if err != nil {
						return err
					}
					if err := inventory.Add(instance); err != nil {
						return err
					}
				}
			}
		}
		for _, gemID := range gems {
			if gemID == "" {
				continue
			}
			instance, found := firstItem(inventory, gemID)
			if !found {
				return ErrGemNotOwned
			}
			if err := inventory.Consume(instance.ID, 1); err != nil {
				return err
			}
		}
		result = &CustomSkill{CharacterID: characterID, Name: name, Comment: comment,
			CMP: cmpTotal, Gems: gems, UpdatedAt: time.Now().UTC()}
		if err := s.inventory.Save(txCtx, inventory); err != nil {
			return err
		}
		return repo.SaveCustomSkill(txCtx, *result)
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

func firstItem(inventory coreinventory.Inventory, definitionID string) (coreitem.Instance, bool) {
	for _, instance := range inventory.Items {
		if instance.DefinitionID == definitionID && instance.Quantity > 0 {
			return instance, true
		}
	}
	return coreitem.Instance{}, false
}
