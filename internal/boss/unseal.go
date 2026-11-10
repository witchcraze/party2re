package boss

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/economy"
)

// UnsealDemonKing increments MaoCount (魔王カウント mao_c) for all participating non-NPC characters
// upon unsealing the demon king (legacy vs_monster.cgi:218).
func (s *Service) UnsealDemonKing(ctx context.Context, characterIDs []string) error {
	if len(characterIDs) == 0 {
		return errors.New("no characters provided")
	}

	// Normalize participant IDs and exclude NPCs before locking.
	ids := make([]string, 0, len(characterIDs))
	for _, id := range characterIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || strings.HasPrefix(trimmed, "@") {
			continue
		}
		ids = append(ids, trimmed)
	}
	if len(ids) == 0 {
		return nil
	}

	var heroNames []string

	updateFn := func(txCtx context.Context) error {
		heroNames = make([]string, 0, len(ids))
		return economy.LockInOrder(txCtx, ids, func(txCtx context.Context, charID string) error {
			char, err := s.characterRepo.FindByIDForUpdate(txCtx, charID)
			if err != nil {
				return fmt.Errorf("find character %s for update: %w", charID, err)
			}
			char.MaoCount++
			if err := s.characterRepo.Update(txCtx, char); err != nil {
				return fmt.Errorf("update character %s: %w", charID, err)
			}
			heroNames = append(heroNames, char.Name)
			return nil
		})
	}

	var err error
	if s.txProvider != nil {
		err = s.txProvider.RunInTx(ctx, updateFn)
	} else {
		err = economy.LockInOrder(ctx, ids, func(ctx context.Context, charID string) error {
			char, fErr := s.characterRepo.FindByID(ctx, charID)
			if fErr != nil {
				return fmt.Errorf("find character %s: %w", charID, fErr)
			}
			char.MaoCount++
			if uErr := s.characterRepo.Update(ctx, char); uErr != nil {
				return fmt.Errorf("update character %s: %w", charID, uErr)
			}
			heroNames = append(heroNames, char.Name)
			return nil
		})
	}
	if err != nil {
		return err
	}

	if s.newsPub != nil && len(heroNames) > 0 {
		newsMsg := fmt.Sprintf("%sによって封印されし者達の封印が解かれました！", strings.Join(heroNames, "、"))
		//lint:ignore error-swallow best-effort informational news does not invalidate the committed unseal
		if err := s.newsPub.PublishNews(ctx, "boss", newsMsg, newsMsg, "System", time.Now().UTC()); err != nil {
			slog.WarnContext(ctx, "boss unseal news failed", "error_type", reflect.TypeOf(err).String())
		}
	}

	return nil
}
