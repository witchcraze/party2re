package fleamarket

import (
	"context"
	"strings"
)

// ListActiveListings retrieves paginated active listings.
func (s *Service) ListActiveListings(ctx context.Context, limit, offset int) ([]Listing, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListActiveListings(ctx, limit, offset)
}

// GetListing retrieves a single listing by ID.
func (s *Service) GetListing(ctx context.Context, listingID string) (Listing, error) {
	if strings.TrimSpace(listingID) == "" {
		return Listing{}, ErrInvalidInput
	}
	return s.repo.GetListingByID(ctx, listingID)
}

// GetCharacterListings retrieves all listings created by a specific character.
func (s *Service) GetCharacterListings(ctx context.Context, characterID string) ([]Listing, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetListingsBySeller(ctx, characterID)
}
