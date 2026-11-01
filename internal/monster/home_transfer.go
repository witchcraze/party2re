package monster

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// HomeCompanion represents an external NPC companion residing in the player's home.
type HomeCompanion struct {
	ID          string
	CharacterID string
	MonsterID   string
	CustomName  string
	CreatedAt   time.Time
}

// HomeCompanionStore accesses and mutates external NPC companions residing in the player's home.
type HomeCompanionStore interface {
	ListHomeCompanions(ctx context.Context, characterID string) ([]HomeCompanion, error)
	FindHomeCompanionForUpdate(ctx context.Context, id string) (HomeCompanion, error)
	DeleteHomeCompanion(ctx context.Context, id string) error
}

// BringToHome transfers a monster from Monster Grandpa's box into the player's home as a pet.
func (s *Service) BringToHome(ctx context.Context, characterID, instanceID string) (MonsterInstance, error) {
	var updated MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != characterID {
			return ErrForbidden
		}
		if inst.Location == LocationHome {
			updated = inst
			return nil // already at home
		}

		homeMonsters, err := s.monsters.ListByCharacterIDAndLocation(txCtx, characterID, LocationHome)
		if err != nil {
			return err
		}

		var companions []HomeCompanion
		if s.companions != nil {
			companions, err = s.companions.ListHomeCompanions(txCtx, characterID)
			if err != nil {
				return err
			}
		}

		for _, m := range homeMonsters {
			if m.CustomName == inst.CustomName {
				return ErrDuplicatePetNameAtHome
			}
		}
		for _, c := range companions {
			if c.CustomName == inst.CustomName {
				return ErrDuplicatePetNameAtHome
			}
		}

		if len(homeMonsters)+len(companions) >= MaxHomePets {
			return ErrHomeFull
		}

		inst.Location = LocationHome
		inst.UpdatedAt = time.Now().UTC()
		if err := s.monsters.Save(txCtx, inst); err != nil {
			return err
		}
		updated = inst
		return nil
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return updated, nil
}

// DepositToBox deposits a home pet back to Monster Grandpa storage box.
func (s *Service) DepositToBox(ctx context.Context, characterID, instanceID string) (MonsterInstance, error) {
	var updated MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err == nil {
			if inst.CharacterID != characterID {
				return ErrForbidden
			}
			if inst.Location == LocationBox {
				updated = inst
				return nil // already in box
			}

			boxCount, err := s.monsters.CountByLocation(txCtx, characterID, LocationBox)
			if err != nil {
				return err
			}
			if boxCount >= BoxCapacityForCharacter(char) {
				return ErrBoxFull
			}

			inst.Location = LocationBox
			inst.UpdatedAt = time.Now().UTC()
			if err := s.monsters.Save(txCtx, inst); err != nil {
				return err
			}
			updated = inst
			return nil
		}
		if !errors.Is(err, ErrMonsterNotFound) {
			return err
		}

		if s.companions == nil {
			return ErrMonsterNotFound
		}

		comp, err := s.companions.FindHomeCompanionForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if comp.CharacterID != characterID {
			return ErrForbidden
		}

		boxCount, err := s.monsters.CountByLocation(txCtx, characterID, LocationBox)
		if err != nil {
			return err
		}
		if boxCount >= BoxCapacityForCharacter(char) {
			return ErrBoxFull
		}

		now := time.Now().UTC()
		createdAt := comp.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		newInst := MonsterInstance{
			ID:          comp.ID,
			CharacterID: characterID,
			MonsterID:   comp.MonsterID,
			CustomName:  comp.CustomName,
			Location:    LocationBox,
			CreatedAt:   createdAt,
			UpdatedAt:   now,
		}
		if err := s.monsters.Save(txCtx, newInst); err != nil {
			return err
		}
		if err := s.companions.DeleteHomeCompanion(txCtx, comp.ID); err != nil {
			return err
		}
		updated = newInst
		return nil
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return updated, nil
}
