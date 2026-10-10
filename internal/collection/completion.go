package collection

import (
	"context"
	"fmt"
	"time"
)

// completeMilestone executes the atomic completion induction boundary for a collection category.
// The Hall of Fame induction (LegendInductor) is a mandatory persistent milestone and must succeed
// before marking completion. If legend induction fails, completion is not finalized and the error
// is returned to the caller so the failure is never masked as success.
// News publishing is an informational broadcast (best-effort) and only fires when newly completed.
func (s *Service) completeMilestone(ctx context.Context, characterID, kind, legendCategory, formatMsg string) error {
	s.milestoneMu.Lock()
	defer s.milestoneMu.Unlock()

	completed, err := s.repo.IsCompleted(ctx, characterID, kind)
	if err != nil {
		return fmt.Errorf("check %s completion: %w", kind, err)
	}
	if completed {
		return nil
	}
	if s.legend != nil {
		if err := s.legend.RecordLegend(ctx, legendCategory, characterID); err != nil {
			return fmt.Errorf("record legend for %s (%s): %w", kind, legendCategory, err)
		}
	}
	newlyCompleted, err := s.repo.MarkCompleted(ctx, characterID, kind)
	if err != nil {
		return fmt.Errorf("mark %s completed: %w", kind, err)
	}
	if newlyCompleted && s.newsPub != nil {
		charName := s.resolveCharacterName(ctx, characterID)
		msg := fmt.Sprintf(formatMsg, charName)
		_ = s.newsPub.PublishNews(ctx, "collection", msg, msg, "System", time.Now().UTC())
	}
	return nil
}

func (s *Service) checkMonsterBookCompletion(ctx context.Context, characterID string) error {
	if s.totalMonsters <= 0 {
		return nil
	}
	count, err := s.repo.GetMonsterBookCount(ctx, characterID)
	if err != nil {
		return err
	}
	if count < s.totalMonsters {
		return nil
	}
	return s.completeMilestone(ctx, characterID, "monster_book", "comp_mon", "%sがモンスターブックをコンプリートしました！")
}

func (s *Service) checkWeaponCollectionCompletion(ctx context.Context, characterID string) error {
	if s.totalWeapons <= 0 {
		return nil
	}
	_, progress, err := s.GetWeaponCollection(ctx, characterID)
	if err != nil {
		return err
	}
	if !progress.IsCompleted {
		return nil
	}
	return s.completeMilestone(ctx, characterID, "weapon", "comp_wea", "%sが武器図鑑をコンプリートしました！")
}

func (s *Service) checkArmorCollectionCompletion(ctx context.Context, characterID string) error {
	if s.totalArmors <= 0 {
		return nil
	}
	_, progress, err := s.GetArmorCollection(ctx, characterID)
	if err != nil {
		return err
	}
	if !progress.IsCompleted {
		return nil
	}
	return s.completeMilestone(ctx, characterID, "armor", "comp_arm", "%sが防具図鑑をコンプリートしました！")
}

func (s *Service) checkItemCollectionCompletion(ctx context.Context, characterID string) error {
	if s.totalItems <= 0 {
		return nil
	}
	_, progress, err := s.GetItemCollection(ctx, characterID, "item")
	if err != nil {
		return err
	}
	if !progress.IsCompleted {
		return nil
	}
	return s.completeMilestone(ctx, characterID, "item", "comp_ite", "%sがアイテム図鑑をコンプリートしました！")
}

// RecoverMissingLegends inspects all 4 collection milestone completions for a character and re-inducts
// any missing Hall of Fame records idempotently. This serves as the recovery procedure for existing completed
// records that lost Hall of Fame induction due to prior persistence failures.
func (s *Service) RecoverMissingLegends(ctx context.Context, characterID string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if s.legend == nil {
		return nil
	}
	milestones := []struct {
		kind     string
		category string
	}{
		{"monster_book", "comp_mon"},
		{"weapon", "comp_wea"},
		{"armor", "comp_arm"},
		{"item", "comp_ite"},
	}
	for _, m := range milestones {
		completed, err := s.repo.IsCompleted(ctx, characterID, m.kind)
		if err != nil {
			return fmt.Errorf("check %s completion: %w", m.kind, err)
		}
		if completed {
			if err := s.legend.RecordLegend(ctx, m.category, characterID); err != nil {
				return fmt.Errorf("recover legend for %s (%s): %w", m.kind, m.category, err)
			}
		}
	}
	return nil
}
