package boss

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// UnsealDemonKing increments MaoCount (魔王カウント mao_c) for all participating non-NPC characters
// upon unsealing the demon king (legacy vs_monster.cgi:218).
func (s *Service) UnsealDemonKing(ctx context.Context, characterIDs []string) error {
	if len(characterIDs) == 0 {
		return errors.New("no characters provided")
	}

	// Filter non-NPCs and deduplicate/sort in ascending lexicographical order for Rank 2 locking
	uniqueIDs := make([]string, 0, len(characterIDs))
	seen := make(map[string]bool, len(characterIDs))
	for _, id := range characterIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || strings.HasPrefix(trimmed, "@") || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		uniqueIDs = append(uniqueIDs, trimmed)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}
	slices.Sort(uniqueIDs)

	var heroNames []string

	updateFn := func(txCtx context.Context) error {
		heroNames = make([]string, 0, len(uniqueIDs))
		for _, charID := range uniqueIDs {
			char, err := s.characterRepo.FindByIDForUpdate(txCtx, charID)
			if err != nil {
				return fmt.Errorf("find character %s for update: %w", charID, err)
			}
			char.MaoCount++
			if err := s.characterRepo.Update(txCtx, char); err != nil {
				return fmt.Errorf("update character %s: %w", charID, err)
			}
			heroNames = append(heroNames, char.Name)
		}
		return nil
	}

	var err error
	if s.txProvider != nil {
		err = s.txProvider.RunInTx(ctx, updateFn)
	} else {
		for _, charID := range uniqueIDs {
			char, fErr := s.characterRepo.FindByID(ctx, charID)
			if fErr != nil {
				return fmt.Errorf("find character %s: %w", charID, fErr)
			}
			char.MaoCount++
			if uErr := s.characterRepo.Update(ctx, char); uErr != nil {
				return fmt.Errorf("update character %s: %w", charID, uErr)
			}
			heroNames = append(heroNames, char.Name)
		}
	}
	if err != nil {
		return err
	}

	if s.newsPub != nil && len(heroNames) > 0 {
		newsMsg := fmt.Sprintf("%sによって封印されし者達の封印が解かれました！", strings.Join(heroNames, "、"))
		_ = s.newsPub.PublishNews(ctx, "boss", newsMsg, newsMsg, "System", time.Now().UTC())
	}

	return nil
}
