package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

type BossRepository struct {
	db *sql.DB
}

func NewBossRepository(db *sql.DB) (*BossRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BossRepository{db: db}, nil
}

// SettleMember updates the character's post-battle state and delivers any chest reward item.
func (r *BossRepository) SettleMember(
	ctx context.Context,
	character corecharacter.Character,
	rewardItem *coreitem.Instance,
) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		if err := updateCharacter(txCtx, executor, character); err != nil {
			return err
		}

		// Deliver reward item if present (Rank 3 Inventory -> Rank 5 Depot).
		if rewardItem != nil {
			if rewardItem.ID == "" {
				rewardItem.ID = id.New()
			}
			if err := deliverRewardItems(txCtx, r.db, character, []coreitem.Instance{*rewardItem}); err != nil {
				return err
			}
		}

		return nil
	})
}
