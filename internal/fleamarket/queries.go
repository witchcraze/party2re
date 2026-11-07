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

// GetActiveCharacterListings retrieves the seller's active listings, the set legacy offers for return.
func (s *Service) GetActiveCharacterListings(ctx context.Context, characterID string) ([]Listing, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetActiveListingsBySeller(ctx, characterID)
}
