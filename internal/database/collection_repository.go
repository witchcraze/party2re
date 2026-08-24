package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/collection"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) (*CollectionRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CollectionRepository{db: db}, nil
}

func (r *CollectionRepository) RecordMonsterDefeat(ctx context.Context, characterID, monsterID, monsterName, habitat string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO character_monster_book (
			character_id, monster_id, monster_name, habitat, defeated_count, first_defeated_at, last_defeated_at
		) VALUES (?, ?, ?, ?, 1, UTC_TIMESTAMP(), UTC_TIMESTAMP())
		ON DUPLICATE KEY UPDATE
			defeated_count = defeated_count + 1,
			monster_name = VALUES(monster_name),
			habitat = VALUES(habitat),
			last_defeated_at = UTC_TIMESTAMP()
	`, characterID, monsterID, monsterName, habitat)
	return err
}

func (r *CollectionRepository) GetMonsterBook(ctx context.Context, characterID string) ([]collection.MonsterBookEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT character_id, monster_id, monster_name, habitat, defeated_count, first_defeated_at, last_defeated_at
		FROM character_monster_book
		WHERE character_id = ?
		ORDER BY first_defeated_at ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []collection.MonsterBookEntry
	for rows.Next() {
		var e collection.MonsterBookEntry
		if err := rows.Scan(&e.CharacterID, &e.MonsterID, &e.MonsterName, &e.Habitat, &e.DefeatedCount, &e.FirstDefeatedAt, &e.LastDefeatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *CollectionRepository) GetMonsterBookCount(ctx context.Context, characterID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM character_monster_book WHERE character_id = ?
	`, characterID).Scan(&count)
	return count, err
}

func (r *CollectionRepository) RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT IGNORE INTO character_item_collection (
			character_id, item_id, item_name, category, discovered_at
		) VALUES (?, ?, ?, ?, UTC_TIMESTAMP())
	`, characterID, itemID, itemName, category)
	return err
}

func (r *CollectionRepository) GetItemCollection(ctx context.Context, characterID, category string) ([]collection.ItemCollectionEntry, error) {
	query := `
		SELECT character_id, item_id, item_name, category, discovered_at
		FROM character_item_collection
		WHERE character_id = ?
	`
	args := []any{characterID}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	query += " ORDER BY discovered_at ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []collection.ItemCollectionEntry
	for rows.Next() {
		var e collection.ItemCollectionEntry
		if err := rows.Scan(&e.CharacterID, &e.ItemID, &e.ItemName, &e.Category, &e.DiscoveredAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *CollectionRepository) GetItemCollectionCount(ctx context.Context, characterID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM character_item_collection WHERE character_id = ?
	`, characterID).Scan(&count)
	return count, err
}
