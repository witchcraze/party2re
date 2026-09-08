package god

import (
	"context"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
)

func (s *Service) executeUnderworldWish(
	ctx context.Context,
	char *corecharacter.Character,
	wishID string,
	res *WishResult,
) error {
	switch wishID {
	case WishExpandDepot:
		if char.OverDepot >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverDepot++

		if s.depots != nil {
			dep, err := s.depots.FindByCharacterIDForUpdate(ctx, char.ID)
			if err == nil {
				dep.Capacity = depot.CalculateCapacity(0, dep.ExDepot, char.OverDepot)
				_ = s.depots.Save(ctx, dep)
			}
		}

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとアイテムを預けたい",
				Realm:       RealmUnderworld,
				Description: "預かり所の上限アップ (+50枠)",
				Available:   char.OverDepot < MaxLimitBreakTier,
				CurrentTier: char.OverDepot,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:      fmt.Sprintf("預かり所の預入上限が +50 拡張されました！ (段階: %d/5)", char.OverDepot),
			NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「もっとアイテムを預けたい」だな。<br />上限を広げてやったぞ…。さらばだ…", char.Name),
			NextLocation: "home",
		}
		return nil

	case WishExpandMonster:
		if char.OverMonster >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverMonster++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとモンスターを預けたい",
				Realm:       RealmUnderworld,
				Description: "モンスター預入上限アップ (+50枠)",
				Available:   char.OverMonster < MaxLimitBreakTier,
				CurrentTier: char.OverMonster,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:      fmt.Sprintf("モンスター預入上限が +50 拡張されました！ (段階: %d/5)", char.OverMonster),
			NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「もっとモンスターを預けたい」だな。<br />上限を広げてやったぞ…。さらばだ…", char.Name),
			NextLocation: "home",
		}
		return nil

	case WishExpandJobMemory:
		if char.OverFuture >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverFuture++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっと職業を覚えたい",
				Realm:       RealmUnderworld,
				Description: "未来のカケラの記憶上限アップ (+1枠)",
				Available:   char.OverFuture < MaxLimitBreakTier,
				CurrentTier: char.OverFuture,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:      fmt.Sprintf("職業記憶上限が +1 拡張されました！ (段階: %d/5)", char.OverFuture),
			NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「もっと職業を覚えたい」だな。<br />上限を広げてやったぞ…。さらばだ…", char.Name),
			NextLocation: "home",
		}
		return nil

	case WishExpandFleaMarket:
		if char.OverFlea >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverFlea++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとフリーマーケットで出品したい",
				Realm:       RealmUnderworld,
				Description: "フリーマーケット出品数上限アップ (+1枠)",
				Available:   char.OverFlea < MaxLimitBreakTier,
				CurrentTier: char.OverFlea,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:      fmt.Sprintf("フリーマーケット出品枠が +1 拡張されました！ (段階: %d/5, 最大 %d 出品)", char.OverFlea, 5+char.OverFlea),
			NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「もっとフリーマーケットで出品したい」だな。<br />上限を広げてやったぞ…。さらばだ…", char.Name),
			NextLocation: "home",
		}
		return nil

	case WishExpandShopStore:
		if char.OverStore >= MaxLimitBreakTier {
			return fmt.Errorf("%w: それは無理な願いだ…。私の力ではこれ以上その上限を増やすことはできんのだ…", ErrLimitBreakMaxed)
		}
		char.OverStore++

		if err := s.characters.Update(ctx, *char); err != nil {
			return err
		}

		*res = WishResult{
			Character: *char,
			Wish: Wish{
				ID:          wishID,
				Name:        "もっとお店で出品したい",
				Realm:       RealmUnderworld,
				Description: "お店の出品数上限アップ (+1枠)",
				Available:   char.OverStore < MaxLimitBreakTier,
				CurrentTier: char.OverStore,
				MaxTier:     MaxLimitBreakTier,
			},
			Message:      fmt.Sprintf("店舗出品枠が +1 拡張されました！ (段階: %d/5)", char.OverStore),
			NPCSpeech:    fmt.Sprintf("ふむ。%sの願いは「もっとお店で出品したい」だな。<br />上限を広げてやったぞ…。さらばだ…", char.Name),
			NextLocation: "home",
		}
		return nil

	default:
		return ErrWishNotFound
	}
}
