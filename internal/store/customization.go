package store

import (
	"context"
	"strings"
)

// ChangeStoreName updates the store's signboard name (requires 5,000 G balance, 0 G deducted).
// Transaction: RunInTx
// Lock Order: store_name_lock(0) -> characters(2)
func (s *Service) ChangeStoreName(ctx context.Context, characterID, cleanName string) error {
	if err := ValidateStoreName(cleanName); err != nil {
		return err
	}

	now := s.nowFunc().UTC()

	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockStoreNamesForUpdate(txCtx); err != nil {
			return err
		}
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		if char.Money < NameChangePrice {
			return ErrInsufficientFunds
		}

		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		// Legacy rejects any active store with the name, including the actor's own.
		taken, err := s.repo.ActiveStoreNameExists(txCtx, cleanName, now)
		if err != nil {
			return err
		}
		if taken {
			return ErrStoreNameTaken
		}

		st.StoreName = cleanName
		st.UpdatedAt = now
		return s.repo.SaveStore(txCtx, st)
	})
}

// ChangeWallpaper changes the store's interior wallpaper style.
func (s *Service) ChangeWallpaper(ctx context.Context, characterID, wallpaper string) error {
	cleanWallpaper := strings.TrimSpace(wallpaper)
	cleanWallpaper = strings.TrimSuffix(cleanWallpaper, ".gif")

	cost, ok := WallpaperPrices[cleanWallpaper]
	if !ok {
		return ErrInvalidWallpaper
	}

	now := s.nowFunc().UTC()

	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		if char.Money < cost {
			return ErrInsufficientFunds
		}

		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		if cost > 0 {
			if err := char.DeductMoney(cost); err != nil {
				return ErrInsufficientFunds
			}
			if err := s.charRepo.Update(txCtx, char); err != nil {
				return err
			}
		}

		st.Wallpaper = cleanWallpaper
		st.UpdatedAt = now
		return s.repo.SaveStore(txCtx, st)
	})
}

// AddInterior adds a piece of furniture to the store (requires 1,000 G balance, 0 G deducted, up to 5 items).
func (s *Service) AddInterior(ctx context.Context, characterID, furnitureID string) (*Interior, error) {
	cleanFurniture := strings.TrimSpace(furnitureID)
	cleanFurniture = strings.TrimSuffix(cleanFurniture, ".gif")

	if !ValidFurnitures[cleanFurniture] {
		return nil, ErrInvalidInterior
	}

	now := s.nowFunc().UTC()
	var createdInterior *Interior

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		if char.Money < InteriorPrice {
			return ErrInsufficientFunds
		}

		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		interiors, err := s.repo.GetInteriorsByStoreID(txCtx, st.ID)
		if err != nil {
			return err
		}
		if len(interiors) >= MaxInteriorCount {
			return ErrMaxInteriorsReached
		}

		interiorObj := Interior{
			ID:          s.idGen(),
			StoreID:     st.ID,
			CharacterID: characterID,
			FurnitureID: cleanFurniture,
			Name:        cleanFurniture,
			SlotIndex:   len(interiors),
			CreatedAt:   now,
		}
		if err := s.repo.SaveInterior(txCtx, interiorObj); err != nil {
			return err
		}

		createdInterior = &interiorObj
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdInterior, nil
}

// RenameInterior gives a custom name to a placed interior furniture item.
func (s *Service) RenameInterior(ctx context.Context, characterID, interiorID, cleanName string) error {
	if err := ValidateInteriorName(cleanName); err != nil {
		return err
	}

	now := s.nowFunc().UTC()

	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}
		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		interiors, err := s.repo.GetInteriorsByStoreID(txCtx, st.ID)
		if err != nil {
			return err
		}

		found := false
		for _, in := range interiors {
			if in.ID == interiorID {
				found = true
				break
			}
		}
		if !found {
			return ErrInteriorNotFound
		}

		return s.repo.UpdateInteriorName(txCtx, interiorID, cleanName)
	})
}

// CleanInteriors clears all furniture from the store.
func (s *Service) CleanInteriors(ctx context.Context, characterID string) error {
	now := s.nowFunc().UTC()

	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}
		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		return s.repo.DeleteInteriorsByStoreID(txCtx, st.ID)
	})
}
