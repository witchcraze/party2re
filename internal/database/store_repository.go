package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/store"
)

type StoreRepository struct {
	db *sql.DB
}

func NewStoreRepository(db *sql.DB) (*StoreRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &StoreRepository{db: db}, nil
}

func (r *StoreRepository) GetStoreByCharacterID(ctx context.Context, characterID string) (store.Store, error) {
	return r.queryStore(ctx, `
		SELECT id, character_id, town_id, store_name, house_style, wallpaper, expires_at, created_at, updated_at
		FROM character_stores
		WHERE character_id = ?
	`, characterID)
}

func (r *StoreRepository) GetStoreByID(ctx context.Context, storeID string) (store.Store, error) {
	return r.queryStore(ctx, `
		SELECT id, character_id, town_id, store_name, house_style, wallpaper, expires_at, created_at, updated_at
		FROM character_stores
		WHERE id = ?
	`, storeID)
}

func (r *StoreRepository) GetStoreByName(ctx context.Context, storeName string) (store.Store, error) {
	return r.queryStore(ctx, `
		SELECT id, character_id, town_id, store_name, house_style, wallpaper, expires_at, created_at, updated_at
		FROM character_stores
		WHERE store_name = ?
	`, storeName)
}

func (r *StoreRepository) queryStore(ctx context.Context, query string, arg interface{}) (store.Store, error) {
	var s store.Store
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, arg).Scan(
		&s.ID,
		&s.CharacterID,
		&s.TownID,
		&s.StoreName,
		&s.HouseStyle,
		&s.Wallpaper,
		&s.ExpiresAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Store{}, store.ErrStoreNotFound
	}
	if err != nil {
		return store.Store{}, err
	}
	return s, nil
}

func (r *StoreRepository) CountActiveTownStores(ctx context.Context, townID string, now time.Time) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_stores
		WHERE town_id = ? AND expires_at > ?
	`, townID, now).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *StoreRepository) ListActiveStoresInTown(ctx context.Context, townID string, now time.Time) ([]store.Store, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, town_id, store_name, house_style, wallpaper, expires_at, created_at, updated_at
		FROM character_stores
		WHERE town_id = ? AND expires_at > ?
		ORDER BY created_at ASC
	`, townID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stores []store.Store
	for rows.Next() {
		var s store.Store
		if err := rows.Scan(
			&s.ID,
			&s.CharacterID,
			&s.TownID,
			&s.StoreName,
			&s.HouseStyle,
			&s.Wallpaper,
			&s.ExpiresAt,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		stores = append(stores, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if stores == nil {
		stores = []store.Store{}
	}
	return stores, nil
}

func (r *StoreRepository) SaveStore(ctx context.Context, s store.Store) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_stores (
			id, character_id, town_id, store_name, house_style, wallpaper, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			town_id = VALUES(town_id),
			store_name = VALUES(store_name),
			house_style = VALUES(house_style),
			wallpaper = VALUES(wallpaper),
			expires_at = VALUES(expires_at),
			updated_at = VALUES(updated_at)
	`, s.ID, s.CharacterID, s.TownID, s.StoreName, s.HouseStyle, s.Wallpaper, s.ExpiresAt, s.CreatedAt, s.UpdatedAt)
	return err
}

func (r *StoreRepository) DeleteStore(ctx context.Context, storeID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM character_stores WHERE id = ?
	`, storeID)
	return err
}

func (r *StoreRepository) GetSalesByStoreID(ctx context.Context, storeID string) ([]store.Sale, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, store_id, character_id, slot_number, item_definition_id, item_name, quantity, enhancement_level, sale_type, price, wish_item_name, created_at
		FROM store_sales
		WHERE store_id = ?
		ORDER BY slot_number ASC
	`, storeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sales []store.Sale
	for rows.Next() {
		var s store.Sale
		if err := rows.Scan(
			&s.ID,
			&s.StoreID,
			&s.CharacterID,
			&s.SlotNumber,
			&s.ItemDefinitionID,
			&s.ItemName,
			&s.Quantity,
			&s.EnhancementLevel,
			&s.SaleType,
			&s.Price,
			&s.WishItemName,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		sales = append(sales, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if sales == nil {
		sales = []store.Sale{}
	}
	return sales, nil
}

func (r *StoreRepository) GetSaleByIDForUpdate(ctx context.Context, saleID string) (store.Sale, error) {
	var s store.Sale
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, store_id, character_id, slot_number, item_definition_id, item_name, quantity, enhancement_level, sale_type, price, wish_item_name, created_at
		FROM store_sales
		WHERE id = ?
		FOR UPDATE
	`, saleID).Scan(
		&s.ID,
		&s.StoreID,
		&s.CharacterID,
		&s.SlotNumber,
		&s.ItemDefinitionID,
		&s.ItemName,
		&s.Quantity,
		&s.EnhancementLevel,
		&s.SaleType,
		&s.Price,
		&s.WishItemName,
		&s.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Sale{}, store.ErrListingNotFound
	}
	if err != nil {
		return store.Sale{}, err
	}
	return s, nil
}

func (r *StoreRepository) SaveSale(ctx context.Context, sale store.Sale) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO store_sales (
			id, store_id, character_id, slot_number, item_definition_id, item_name, quantity, enhancement_level, sale_type, price, wish_item_name, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			slot_number = VALUES(slot_number),
			item_definition_id = VALUES(item_definition_id),
			item_name = VALUES(item_name),
			quantity = VALUES(quantity),
			enhancement_level = VALUES(enhancement_level),
			sale_type = VALUES(sale_type),
			price = VALUES(price),
			wish_item_name = VALUES(wish_item_name)
	`, sale.ID, sale.StoreID, sale.CharacterID, sale.SlotNumber, sale.ItemDefinitionID, sale.ItemName, sale.Quantity, sale.EnhancementLevel, sale.SaleType, sale.Price, sale.WishItemName, sale.CreatedAt)
	return err
}

func (r *StoreRepository) DeleteSale(ctx context.Context, saleID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM store_sales WHERE id = ?
	`, saleID)
	return err
}

func (r *StoreRepository) GetInteriorsByStoreID(ctx context.Context, storeID string) ([]store.Interior, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, store_id, character_id, furniture_id, name, slot_index, created_at
		FROM store_interiors
		WHERE store_id = ?
		ORDER BY slot_index ASC
	`, storeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interiors []store.Interior
	for rows.Next() {
		var in store.Interior
		if err := rows.Scan(
			&in.ID,
			&in.StoreID,
			&in.CharacterID,
			&in.FurnitureID,
			&in.Name,
			&in.SlotIndex,
			&in.CreatedAt,
		); err != nil {
			return nil, err
		}
		interiors = append(interiors, in)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if interiors == nil {
		interiors = []store.Interior{}
	}
	return interiors, nil
}

func (r *StoreRepository) SaveInterior(ctx context.Context, interior store.Interior) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO store_interiors (
			id, store_id, character_id, furniture_id, name, slot_index, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			slot_index = VALUES(slot_index)
	`, interior.ID, interior.StoreID, interior.CharacterID, interior.FurnitureID, interior.Name, interior.SlotIndex, interior.CreatedAt)
	return err
}

func (r *StoreRepository) DeleteInteriorsByStoreID(ctx context.Context, storeID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM store_interiors WHERE store_id = ?
	`, storeID)
	return err
}

func (r *StoreRepository) UpdateInteriorName(ctx context.Context, interiorID string, name string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE store_interiors SET name = ? WHERE id = ?
	`, name, interiorID)
	return err
}
