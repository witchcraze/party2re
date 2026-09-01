package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/id"
)

type AuctionRepository struct {
	db *sql.DB
}

func NewAuctionRepository(db *sql.DB) (*AuctionRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &AuctionRepository{db: db}, nil
}

func (r *AuctionRepository) CreateListing(ctx context.Context, listing auction.AuctionListing) (auction.AuctionListing, error) {
	if listing.ID == "" {
		listing.ID = id.New()
	}
	if listing.CreatedAt.IsZero() {
		listing.CreatedAt = time.Now().UTC()
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO auction_listings (
			id, seller_character_id, item_id, item_name, item_category,
			enhancement_level, start_bid, current_bid, buyout_price,
			highest_bidder_id, status, created_at, expires_at, settled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, NULL, 'ACTIVE', ?, ?, NULL)
	`, listing.ID, listing.SellerCharacterID, listing.ItemID, listing.ItemName, listing.ItemCategory,
		listing.EnhancementLevel, listing.StartBid, listing.BuyoutPrice, listing.CreatedAt, listing.ExpiresAt)
	if err != nil {
		return auction.AuctionListing{}, err
	}
	return listing, nil
}

func (r *AuctionRepository) GetListing(ctx context.Context, listingID string) (auction.AuctionListing, error) {
	var l auction.AuctionListing
	var highestBidder sql.NullString
	var settledAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, seller_character_id, item_id, item_name, item_category,
		       enhancement_level, start_bid, current_bid, buyout_price,
		       highest_bidder_id, status, created_at, expires_at, settled_at
		FROM auction_listings
		WHERE id = ?
	`, listingID).Scan(
		&l.ID, &l.SellerCharacterID, &l.ItemID, &l.ItemName, &l.ItemCategory,
		&l.EnhancementLevel, &l.StartBid, &l.CurrentBid, &l.BuyoutPrice,
		&highestBidder, &l.Status, &l.CreatedAt, &l.ExpiresAt, &settledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return auction.AuctionListing{}, auction.ErrListingNotFound
	}
	if err != nil {
		return auction.AuctionListing{}, err
	}
	if highestBidder.Valid {
		l.HighestBidderID = &highestBidder.String
	}
	if settledAt.Valid {
		l.SettledAt = &settledAt.Time
	}
	return l, nil
}

func (r *AuctionRepository) ListActive(ctx context.Context, limit, offset int) ([]auction.AuctionListing, int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM auction_listings
		WHERE status = 'ACTIVE' AND expires_at > UTC_TIMESTAMP()
	`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, seller_character_id, item_id, item_name, item_category,
		       enhancement_level, start_bid, current_bid, buyout_price,
		       highest_bidder_id, status, created_at, expires_at, settled_at
		FROM auction_listings
		WHERE status = 'ACTIVE' AND expires_at > UTC_TIMESTAMP()
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var listings []auction.AuctionListing
	for rows.Next() {
		var l auction.AuctionListing
		var highestBidder sql.NullString
		var settledAt sql.NullTime
		if err := rows.Scan(
			&l.ID, &l.SellerCharacterID, &l.ItemID, &l.ItemName, &l.ItemCategory,
			&l.EnhancementLevel, &l.StartBid, &l.CurrentBid, &l.BuyoutPrice,
			&highestBidder, &l.Status, &l.CreatedAt, &l.ExpiresAt, &settledAt,
		); err != nil {
			return nil, 0, err
		}
		if highestBidder.Valid {
			l.HighestBidderID = &highestBidder.String
		}
		if settledAt.Valid {
			l.SettledAt = &settledAt.Time
		}
		listings = append(listings, l)
	}
	return listings, total, rows.Err()
}

func (r *AuctionRepository) PlaceBid(ctx context.Context, listingID, bidderID string, bidAmount int) (auction.AuctionListing, error) {
	var l auction.AuctionListing

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock listing
		var highestBidder sql.NullString
		var settledAt sql.NullTime
		err := executor.QueryRowContext(txCtx, `
			SELECT id, seller_character_id, item_id, item_name, item_category,
			       enhancement_level, start_bid, current_bid, buyout_price,
			       highest_bidder_id, status, created_at, expires_at, settled_at
			FROM auction_listings
			WHERE id = ? FOR UPDATE
		`, listingID).Scan(
			&l.ID, &l.SellerCharacterID, &l.ItemID, &l.ItemName, &l.ItemCategory,
			&l.EnhancementLevel, &l.StartBid, &l.CurrentBid, &l.BuyoutPrice,
			&highestBidder, &l.Status, &l.CreatedAt, &l.ExpiresAt, &settledAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return auction.ErrListingNotFound
		}
		if err != nil {
			return err
		}
		if l.Status != auction.StatusActive {
			return auction.ErrListingNotActive
		}
		if time.Now().UTC().After(l.ExpiresAt) {
			return auction.ErrListingExpired
		}
		if l.SellerCharacterID == bidderID {
			return auction.ErrSellerCannotBid
		}
		if bidAmount < l.StartBid || (l.CurrentBid > 0 && bidAmount <= l.CurrentBid) {
			return auction.ErrInvalidBidAmount
		}

		// 2. Deduct gold from new bidder
		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, bidAmount, bidderID, bidAmount)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return auction.ErrInsufficientGold
		}

		// 3. Refund previous highest bidder if one exists
		if highestBidder.Valid && l.CurrentBid > 0 {
			_, err = executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money + ?
				WHERE id = ?
			`, l.CurrentBid, highestBidder.String)
			if err != nil {
				return err
			}
		}

		// 4. Update listing with new highest bid
		_, err = executor.ExecContext(txCtx, `
			UPDATE auction_listings
			SET current_bid = ?, highest_bidder_id = ?
			WHERE id = ?
		`, bidAmount, bidderID, listingID)
		if err != nil {
			return err
		}

		l.CurrentBid = bidAmount
		l.HighestBidderID = &bidderID
		if settledAt.Valid {
			l.SettledAt = &settledAt.Time
		}

		return nil
	})
	if err != nil {
		return auction.AuctionListing{}, err
	}
	return l, nil
}

func (r *AuctionRepository) Buyout(ctx context.Context, listingID, buyerID string) (auction.AuctionListing, error) {
	var l auction.AuctionListing

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock listing
		var highestBidder sql.NullString
		var settledAt sql.NullTime
		err := executor.QueryRowContext(txCtx, `
			SELECT id, seller_character_id, item_id, item_name, item_category,
			       enhancement_level, start_bid, current_bid, buyout_price,
			       highest_bidder_id, status, created_at, expires_at, settled_at
			FROM auction_listings
			WHERE id = ? FOR UPDATE
		`, listingID).Scan(
			&l.ID, &l.SellerCharacterID, &l.ItemID, &l.ItemName, &l.ItemCategory,
			&l.EnhancementLevel, &l.StartBid, &l.CurrentBid, &l.BuyoutPrice,
			&highestBidder, &l.Status, &l.CreatedAt, &l.ExpiresAt, &settledAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return auction.ErrListingNotFound
		}
		if err != nil {
			return err
		}
		if l.Status != auction.StatusActive {
			return auction.ErrListingNotActive
		}
		if l.BuyoutPrice <= 0 {
			return auction.ErrNoBuyoutPrice
		}
		if l.SellerCharacterID == buyerID {
			return auction.ErrSellerCannotBid
		}

		// 2. Deduct buyout price from buyer
		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, l.BuyoutPrice, buyerID, l.BuyoutPrice)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return auction.ErrInsufficientGold
		}

		// 3. Refund previous highest bidder if different from buyer
		if highestBidder.Valid && l.CurrentBid > 0 && highestBidder.String != buyerID {
			_, err = executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money + ?
				WHERE id = ?
			`, l.CurrentBid, highestBidder.String)
			if err != nil {
				return err
			}
		}

		// 4. Pay seller
		_, err = executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money + ?
			WHERE id = ?
		`, l.BuyoutPrice, l.SellerCharacterID)
		if err != nil {
			return err
		}

		// 5. Mark listing SOLD
		now := time.Now().UTC()
		_, err = executor.ExecContext(txCtx, `
			UPDATE auction_listings
			SET current_bid = ?, highest_bidder_id = ?, status = 'SOLD', settled_at = ?
			WHERE id = ?
		`, l.BuyoutPrice, buyerID, now, listingID)
		if err != nil {
			return err
		}

		l.CurrentBid = l.BuyoutPrice
		l.HighestBidderID = &buyerID
		l.Status = auction.StatusSold
		l.SettledAt = &now

		return nil
	})
	if err != nil {
		return auction.AuctionListing{}, err
	}
	return l, nil
}

func (r *AuctionRepository) SettleListing(ctx context.Context, listingID string) (auction.AuctionListing, error) {
	var l auction.AuctionListing

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		var highestBidder sql.NullString
		var settledAt sql.NullTime
		err := executor.QueryRowContext(txCtx, `
			SELECT id, seller_character_id, item_id, item_name, item_category,
			       enhancement_level, start_bid, current_bid, buyout_price,
			       highest_bidder_id, status, created_at, expires_at, settled_at
			FROM auction_listings
			WHERE id = ? FOR UPDATE
		`, listingID).Scan(
			&l.ID, &l.SellerCharacterID, &l.ItemID, &l.ItemName, &l.ItemCategory,
			&l.EnhancementLevel, &l.StartBid, &l.CurrentBid, &l.BuyoutPrice,
			&highestBidder, &l.Status, &l.CreatedAt, &l.ExpiresAt, &settledAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return auction.ErrListingNotFound
		}
		if err != nil {
			return err
		}
		if l.Status != auction.StatusActive {
			return nil // Already settled or cancelled
		}

		now := time.Now().UTC()
		if highestBidder.Valid && l.CurrentBid > 0 {
			// Sold to highest bidder -> Credit seller
			_, err = executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money + ?
				WHERE id = ?
			`, l.CurrentBid, l.SellerCharacterID)
			if err != nil {
				return err
			}

			_, err = executor.ExecContext(txCtx, `
				UPDATE auction_listings
				SET status = 'SOLD', settled_at = ?
				WHERE id = ?
			`, now, listingID)
			if err != nil {
				return err
			}
			l.Status = auction.StatusSold
			l.HighestBidderID = &highestBidder.String
		} else {
			// Unsold -> Mark Expired
			_, err = executor.ExecContext(txCtx, `
				UPDATE auction_listings
				SET status = 'EXPIRED', settled_at = ?
				WHERE id = ?
			`, now, listingID)
			if err != nil {
				return err
			}
			l.Status = auction.StatusExpired
		}
		l.SettledAt = &now

		return nil
	})
	if err != nil {
		return auction.AuctionListing{}, err
	}
	return l, nil
}

func (r *AuctionRepository) CancelListing(ctx context.Context, listingID, sellerID string) (auction.AuctionListing, error) {
	now := time.Now().UTC()
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE auction_listings
		SET status = 'CANCELLED', settled_at = ?
		WHERE id = ? AND seller_character_id = ? AND highest_bidder_id IS NULL AND status = 'ACTIVE'
	`, now, listingID, sellerID)
	if err != nil {
		return auction.AuctionListing{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return auction.AuctionListing{}, err
	}
	if affected == 0 {
		return auction.AuctionListing{}, auction.ErrCannotCancelWithBids
	}

	return r.GetListing(ctx, listingID)
}
