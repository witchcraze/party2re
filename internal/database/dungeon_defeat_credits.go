package database

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// PostBattleSettler is the shared post-battle owner that applies claimed Dungeon credits.
type PostBattleSettler interface {
	ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error)
}

// SetPostBattleSettler configures the settler for in-run defeat credits.
func (r *DungeonRepository) SetPostBattleSettler(s PostBattleSettler) { r.postBattle = s }

// SettleDefeatCredits locks each credited character (Rank 2), records a
// receipt per run and credit ID, and applies only unreceipted credits through
// the shared post-battle owner in the same transaction. A run already claimed
// by FinalizeExpedition settled every credit before freezing, so its stale
// credits are ignored.
// Transaction: RunInTx, joined by the shared post-battle settlement.
// Lock Order: characters(2, ascending ID) -> dungeon_defeat_credits receipts -> inventory_items(3) -> character_monsters(8).
func (r *DungeonRepository) SettleDefeatCredits(ctx context.Context, runID string, credits []dungeon.DefeatCredit) error {
	if r.postBattle == nil {
		return errors.New("dungeon post-battle settler is not configured")
	}
	sorted := slices.SortedFunc(slices.Values(credits), func(a, b dungeon.DefeatCredit) int {
		return strings.Compare(a.CharacterID, b.CharacterID)
	})
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		charRepo := &CharacterRepository{db: r.db}
		for _, c := range sorted {
			if _, err := charRepo.FindByIDForUpdate(txCtx, c.CharacterID); err != nil {
				if errors.Is(err, corecharacter.ErrNotFound) {
					return dungeon.ErrCharacterNotFound
				}
				return err
			}
		}
		var finalized int
		if err := executor.QueryRowContext(txCtx, `SELECT COUNT(*) FROM dungeon_expedition_history WHERE id = ?`, runID).Scan(&finalized); err != nil {
			return err
		}
		if finalized > 0 {
			return nil
		}
		for _, c := range sorted {
			res, err := executor.ExecContext(txCtx,
				`INSERT IGNORE INTO dungeon_defeat_credits (run_id, credit_id, character_id) VALUES (?, ?, ?)`,
				runID, c.ID, c.CharacterID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 0 {
				continue
			}
			if _, err := r.postBattle.ApplyPostBattleResult(txCtx, battle.ApplyPostBattleRequest{
				CharacterIDs:          []string{c.CharacterID},
				PreserveEarnedRewards: true,
				RecipientRewards:      map[string]corebattle.Reward{c.CharacterID: c.Reward},
				RecipientCharacterID:  c.CharacterID,
				ExplicitDefeats:       c.Defeats,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
