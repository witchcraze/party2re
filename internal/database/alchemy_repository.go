package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
)

type AlchemyRepository struct {
	db *sql.DB
}

func NewAlchemyRepository(db *sql.DB) (*AlchemyRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &AlchemyRepository{db: db}, nil
}

func (r *AlchemyRepository) GetSynthesisState(ctx context.Context, characterID string) (alchemy.Synthesis, error) {
	executor := ExecutorFromContext(ctx, r.db)
	return r.scanSynthesisState(ctx, executor, "SELECT recipe_id, state, started_at, matures_at, total_crafts, comp_alc FROM character_alchemy WHERE character_id = ?", characterID)
}

func (r *AlchemyRepository) GetSynthesisStateForUpdate(ctx context.Context, characterID string) (alchemy.Synthesis, error) {
	executor := ExecutorFromContext(ctx, r.db)
	return r.scanSynthesisState(ctx, executor, "SELECT recipe_id, state, started_at, matures_at, total_crafts, comp_alc FROM character_alchemy WHERE character_id = ? FOR UPDATE", characterID)
}

func (r *AlchemyRepository) scanSynthesisState(ctx context.Context, executor sqlContextExecutor, query string, characterID string) (alchemy.Synthesis, error) {
	var (
		recipeID    sql.NullString
		state       string
		startedAt   sql.NullTime
		maturesAt   sql.NullTime
		totalCrafts int
		compAlc     bool
	)

	err := executor.QueryRowContext(ctx, query, characterID).Scan(
		&recipeID,
		&state,
		&startedAt,
		&maturesAt,
		&totalCrafts,
		&compAlc,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return alchemy.Synthesis{
			CharacterID: characterID,
			State:       alchemy.StateNone,
		}, nil
	}
	if err != nil {
		return alchemy.Synthesis{}, fmt.Errorf("scan synthesis state: %w", err)
	}

	recID := ""
	if recipeID.Valid {
		recID = recipeID.String
	}
	var startPtr, maturePtr *time.Time
	if startedAt.Valid {
		t := startedAt.Time.UTC()
		startPtr = &t
	}
	if maturesAt.Valid {
		t := maturesAt.Time.UTC()
		maturePtr = &t
	}

	return alchemy.Synthesis{
		CharacterID: characterID,
		RecipeID:    recID,
		State:       alchemy.SynthesisState(state),
		StartedAt:   startPtr,
		MaturesAt:   maturePtr,
		TotalCrafts: totalCrafts,
		CompAlc:     compAlc,
	}, nil
}

func (r *AlchemyRepository) SaveSynthesisState(ctx context.Context, s alchemy.Synthesis) error {
	executor := ExecutorFromContext(ctx, r.db)
	var recID any
	if s.RecipeID != "" {
		recID = s.RecipeID
	}
	var startedAt, maturesAt any
	if s.StartedAt != nil {
		startedAt = s.StartedAt.UTC()
	}
	if s.MaturesAt != nil {
		maturesAt = s.MaturesAt.UTC()
	}

	query := `
		INSERT INTO character_alchemy (character_id, recipe_id, state, started_at, matures_at, total_crafts, comp_alc)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			recipe_id = VALUES(recipe_id),
			state = VALUES(state),
			started_at = VALUES(started_at),
			matures_at = VALUES(matures_at),
			total_crafts = VALUES(total_crafts),
			comp_alc = VALUES(comp_alc)
	`
	_, err := executor.ExecContext(ctx, query,
		s.CharacterID,
		recID,
		string(s.State),
		startedAt,
		maturesAt,
		s.TotalCrafts,
		s.CompAlc,
	)
	if err != nil {
		return fmt.Errorf("save synthesis state: %w", err)
	}
	return nil
}

func (r *AlchemyRepository) CompleteOngoingSynthesis(ctx context.Context, characterID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	query := "UPDATE character_alchemy SET state = 'completed' WHERE character_id = ? AND state = 'ongoing'"
	_, err := executor.ExecContext(ctx, query, characterID)
	if err != nil {
		return fmt.Errorf("complete ongoing synthesis: %w", err)
	}
	return nil
}

func (r *AlchemyRepository) GetDiscoveredRecipes(ctx context.Context, characterID string) ([]alchemy.DiscoveredRecipe, error) {
	executor := ExecutorFromContext(ctx, r.db)
	query := "SELECT recipe_id, is_crafted, discovered_at, crafted_at FROM character_alchemy_recipes WHERE character_id = ? ORDER BY discovered_at ASC"
	rows, err := executor.QueryContext(ctx, query, characterID)
	if err != nil {
		return nil, fmt.Errorf("query discovered recipes: %w", err)
	}
	defer rows.Close()

	var list []alchemy.DiscoveredRecipe
	for rows.Next() {
		var (
			recipeID     string
			isCrafted    bool
			discoveredAt time.Time
			craftedAt    sql.NullTime
		)
		if err := rows.Scan(&recipeID, &isCrafted, &discoveredAt, &craftedAt); err != nil {
			return nil, fmt.Errorf("scan discovered recipe: %w", err)
		}
		var craftedPtr *time.Time
		if craftedAt.Valid {
			t := craftedAt.Time.UTC()
			craftedPtr = &t
		}
		list = append(list, alchemy.DiscoveredRecipe{
			RecipeID:     recipeID,
			IsCrafted:    isCrafted,
			DiscoveredAt: discoveredAt.UTC(),
			CraftedAt:    craftedPtr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *AlchemyRepository) SaveDiscoveredRecipe(ctx context.Context, characterID, recipeID string, isCrafted bool) error {
	executor := ExecutorFromContext(ctx, r.db)
	query := `
		INSERT INTO character_alchemy_recipes (character_id, recipe_id, is_crafted, discovered_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE is_crafted = VALUES(is_crafted)
	`
	_, err := executor.ExecContext(ctx, query, characterID, recipeID, isCrafted)
	if err != nil {
		return fmt.Errorf("save discovered recipe: %w", err)
	}
	return nil
}

func (r *AlchemyRepository) MarkRecipeCrafted(ctx context.Context, characterID, recipeID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	query := `
		INSERT INTO character_alchemy_recipes (character_id, recipe_id, is_crafted, discovered_at, crafted_at)
		VALUES (?, ?, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE is_crafted = TRUE, crafted_at = CURRENT_TIMESTAMP
	`
	_, err := executor.ExecContext(ctx, query, characterID, recipeID)
	if err != nil {
		return fmt.Errorf("mark recipe crafted: %w", err)
	}
	return nil
}

func (r *AlchemyRepository) CountCraftedRecipes(ctx context.Context, characterID string) (int, error) {
	executor := ExecutorFromContext(ctx, r.db)
	query := "SELECT COUNT(*) FROM character_alchemy_recipes WHERE character_id = ? AND is_crafted = TRUE"
	var count int
	if err := executor.QueryRowContext(ctx, query, characterID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count crafted recipes: %w", err)
	}
	return count, nil
}

func (r *AlchemyRepository) SetCompAlcTitle(ctx context.Context, characterID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	query := "UPDATE character_alchemy SET comp_alc = TRUE WHERE character_id = ?"
	_, err := executor.ExecContext(ctx, query, characterID)
	if err != nil {
		return fmt.Errorf("set comp_alc title: %w", err)
	}
	return nil
}
