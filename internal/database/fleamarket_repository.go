package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/id"
)

type FleaMarketRepository struct {
	db *sql.DB
}

func NewFleaMarketRepository(db *sql.DB) (*FleaMarketRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &FleaMarketRepository{db: db}, nil
}

func (r *FleaMarketRepository) CreateListing(ctx context.Context, listing fleamarket.Listing) error {
	if listing.ID == "" {
		listing.ID = id.New()
	}
	if listing.CreatedAt.IsZero() {
		listing.CreatedAt = time.Now().UTC()
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO fleamarket_listings (
			id, seller_character_id, seller_name, item_id, item_name, item_category, enhancement_level,
			price, status, buyer_character_id, buyer_name, created_at, sold_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, listing.ID, listing.SellerCharacterID, listing.SellerName, listing.ItemID, listing.ItemName, listing.ItemCategory, listing.EnhancementLevel,
		listing.Price, string(listing.Status), listing.BuyerCharacterID, listing.BuyerName, listing.CreatedAt, listing.SoldAt)
	return err
}

func (r *FleaMarketRepository) scanListing(row scanner) (fleamarket.Listing, error) {
	var l fleamarket.Listing
	var statusStr string
	var buyerID, buyerName sql.NullString
	var soldAt sql.NullTime

	err := row.Scan(
		&l.ID,
		&l.SellerCharacterID,
		&l.SellerName,
		&l.ItemID,
		&l.ItemName,
		&l.ItemCategory,
		&l.EnhancementLevel,
		&l.Price,
		&statusStr,
		&buyerID,
		&buyerName,
		&l.CreatedAt,
		&soldAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return fleamarket.Listing{}, fleamarket.ErrListingNotFound
	}
	if err != nil {
		return fleamarket.Listing{}, err
	}

	l.Status = fleamarket.ListingStatus(statusStr)
	if buyerID.Valid {
		l.BuyerCharacterID = &buyerID.String
	}
	if buyerName.Valid {
		l.BuyerName = &buyerName.String
	}
	if soldAt.Valid {
		l.SoldAt = &soldAt.Time
	}

	return l, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *FleaMarketRepository) GetListingByID(ctx context.Context, listingID string) (fleamarket.Listing, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, seller_character_id, seller_name, item_id, item_name, item_category, enhancement_level,
		       price, status, buyer_character_id, buyer_name, created_at, sold_at
		FROM fleamarket_listings
		WHERE id = ?
	`, listingID)
	return r.scanListing(row)
}

func (r *FleaMarketRepository) GetListingByIDForUpdate(ctx context.Context, listingID string) (fleamarket.Listing, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, seller_character_id, seller_name, item_id, item_name, item_category, enhancement_level,
		       price, status, buyer_character_id, buyer_name, created_at, sold_at
		FROM fleamarket_listings
		WHERE id = ?
		FOR UPDATE
	`, listingID)
	return r.scanListing(row)
}

// ListActiveListings orders by legacy kind, item number, price and age (free.cgi
// list_sort); id keeps pages stable.
func (r *FleaMarketRepository) ListActiveListings(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM fleamarket_listings
		WHERE status = 'active'
	`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, seller_character_id, seller_name, item_id, item_name, item_category, enhancement_level,
		       price, status, buyer_character_id, buyer_name, created_at, sold_at
		FROM fleamarket_listings
		WHERE status = 'active'
		ORDER BY FIELD(SUBSTRING_INDEX(item_id, '-', 1), 'weapon', 'armor', 'item'),
		         CAST(SUBSTRING_INDEX(item_id, '-', -1) AS UNSIGNED), price, created_at, id
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	listings := []fleamarket.Listing{}
	for rows.Next() {
		l, err := r.scanListing(rows)
		if err != nil {
			return nil, 0, err
		}
		listings = append(listings, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return listings, total, nil
}

func (r *FleaMarketRepository) GetActiveListingsBySeller(ctx context.Context, sellerCharacterID string) ([]fleamarket.Listing, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, seller_character_id, seller_name, item_id, item_name, item_category, enhancement_level,
		       price, status, buyer_character_id, buyer_name, created_at, sold_at
		FROM fleamarket_listings
		WHERE seller_character_id = ? AND status = 'active'
		ORDER BY created_at DESC, id
	`, sellerCharacterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	listings := []fleamarket.Listing{}
	for rows.Next() {
		l, err := r.scanListing(rows)
		if err != nil {
			return nil, err
		}
		listings = append(listings, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return listings, nil
}

func (r *FleaMarketRepository) CountActiveListingsBySeller(ctx context.Context, sellerCharacterID string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM fleamarket_listings
		WHERE seller_character_id = ? AND status = 'active'
	`, sellerCharacterID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// LockListingCapacityForUpdate holds the market-wide row for the transaction.
// READ COMMITTED has no gap locks, so locking listing rows cannot serialize inserts.
func (r *FleaMarketRepository) LockListingCapacityForUpdate(ctx context.Context) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO fleamarket_capacity_lock (id) VALUES (1)
		ON DUPLICATE KEY UPDATE id = id
	`)
	return err
}

func (r *FleaMarketRepository) CountTotalActiveListings(ctx context.Context) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM fleamarket_listings
		WHERE status = 'active'
	`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *FleaMarketRepository) UpdateListing(ctx context.Context, listing fleamarket.Listing) error {
	result, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE fleamarket_listings
		SET status = ?, buyer_character_id = ?, buyer_name = ?, sold_at = ?
		WHERE id = ? AND status = 'active'
	`, string(listing.Status), listing.BuyerCharacterID, listing.BuyerName, listing.SoldAt, listing.ID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var count int
		err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
			SELECT COUNT(*) FROM fleamarket_listings WHERE id = ?
		`, listing.ID).Scan(&count)
		if err == nil && count == 0 {
			return fleamarket.ErrListingNotFound
		}
		return fleamarket.ErrListingNotActive
	}
	return nil
}
