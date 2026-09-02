package fleamarket

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	MaxListingsPerCharacter = 5
	MinListingPrice         = 1
	MaxListingPrice         = 999999
	MaxGoldCap              = 2000000000
)

type ListingStatus string

const (
	StatusActive    ListingStatus = "active"
	StatusSold      ListingStatus = "sold"
	StatusCancelled ListingStatus = "cancelled"
)

var (
	ErrNilDependency       = errors.New("fleamarket dependency is nil")
	ErrCharacterNotFound   = errors.New("character not found")
	ErrListingNotFound     = errors.New("fleamarket listing not found")
	ErrListingNotActive    = errors.New("fleamarket listing is not active")
	ErrCannotBuyOwnListing = errors.New("cannot purchase your own flea market listing")
	ErrUnauthorizedSeller  = errors.New("only the seller can cancel this listing")
	ErrInsufficientGold    = errors.New("insufficient gold to purchase listing")
	ErrMaxListingsReached  = errors.New("maximum active listings limit reached")
	ErrInvalidPrice        = errors.New("listing price must be between 1 and 999999 gold")
	ErrItemNotInInventory  = errors.New("item not found in inventory or insufficient quantity")
	ErrForbidden           = errors.New("access forbidden: character does not own this resource")
	ErrInvalidInput        = errors.New("invalid flea market input parameters")
)

type Listing struct {
	ID                string        `json:"id"`
	SellerCharacterID string        `json:"seller_character_id"`
	SellerName        string        `json:"seller_name"`
	ItemID            string        `json:"item_id"`
	ItemName          string        `json:"item_name"`
	ItemCategory      string        `json:"item_category"`
	Price             int           `json:"price"`
	Status            ListingStatus `json:"status"`
	BuyerCharacterID  *string       `json:"buyer_character_id,omitempty"`
	BuyerName         *string       `json:"buyer_name,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	SoldAt            *time.Time    `json:"sold_at,omitempty"`
}

type PurchaseResult struct {
	Listing      Listing           `json:"listing"`
	BuyerGold    int               `json:"buyer_gold"`
	SellerGold   int               `json:"seller_gold"`
	ItemInstance coreitem.Instance `json:"item_instance"`
}

type FleaMarketRepository interface {
	CreateListing(ctx context.Context, listing Listing) error
	GetListingByID(ctx context.Context, id string) (Listing, error)
	GetListingByIDForUpdate(ctx context.Context, id string) (Listing, error)
	ListActiveListings(ctx context.Context, limit, offset int) ([]Listing, int, error)
	GetListingsBySeller(ctx context.Context, sellerCharacterID string) ([]Listing, error)
	CountActiveListingsBySeller(ctx context.Context, sellerCharacterID string) (int, error)
	UpdateListing(ctx context.Context, listing Listing) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type ItemDefinitionProvider = coreitem.DefinitionProvider

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Service struct {
	repo       FleaMarketRepository
	charRepo   CharacterRepository
	invRepo    InventoryRepository
	itemDefs   ItemDefinitionProvider
	txProvider TransactionProvider
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithItemDefinitionProvider(itemDefs ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.itemDefs = itemDefs
	}
}

func NewService(
	repo FleaMarketRepository,
	charRepo CharacterRepository,
	invRepo InventoryRepository,
	opts ...Option,
) (*Service, error) {
	if repo == nil || charRepo == nil || invRepo == nil {
		return nil, ErrNilDependency
	}

	svc := &Service{
		repo:     repo,
		charRepo: charRepo,
		invRepo:  invRepo,
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

// CreateListing lists an item from the character's inventory onto the flea market.
func (s *Service) CreateListing(
	ctx context.Context,
	sellerCharacterID string,
	itemInstanceOrDefID string,
	price int,
	now time.Time,
) (Listing, error) {
	if strings.TrimSpace(sellerCharacterID) == "" || strings.TrimSpace(itemInstanceOrDefID) == "" {
		return Listing{}, ErrInvalidInput
	}
	if price < MinListingPrice || price > MaxListingPrice {
		return Listing{}, ErrInvalidPrice
	}

	var created Listing

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock seller character
		seller, err := s.charRepo.FindByIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Check active listing count limit
		activeCount, err := s.repo.CountActiveListingsBySeller(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}
		maxListings := MaxListingsPerCharacter + seller.OverFlea
		if activeCount >= maxListings {
			return ErrMaxListingsReached
		}

		// 3. Lock seller inventory
		inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		// 4. Locate item in inventory
		inst, found := inv.Find(itemInstanceOrDefID)
		if !found {
			// Try locating by DefinitionID
			for _, item := range inv.Items {
				if item.DefinitionID == itemInstanceOrDefID && item.Quantity > 0 {
					inst = item
					found = true
					break
				}
			}
		}
		if !found || inst.Quantity < 1 {
			return ErrItemNotInInventory
		}

		// 5. Consume 1 unit from inventory
		if err := inv.Consume(inst.ID, 1); err != nil {
			return err
		}
		if err := s.invRepo.Save(txCtx, inv); err != nil {
			return err
		}

		// 6. Fetch definition for item metadata
		itemName := inst.DefinitionID
		itemCategory := "misc"
		if s.itemDefs != nil {
			if def, defErr := s.itemDefs.FindByID(inst.DefinitionID); defErr == nil {
				itemName = def.Name
				if def.Slot != "" {
					itemCategory = string(def.Slot)
				} else {
					itemCategory = "consumable"
				}
			}
		}

		// 7. Create Listing record
		created = Listing{
			ID:                id.New(),
			SellerCharacterID: seller.ID,
			SellerName:        seller.Name,
			ItemID:            inst.DefinitionID,
			ItemName:          itemName,
			ItemCategory:      itemCategory,
			Price:             price,
			Status:            StatusActive,
			CreatedAt:         now,
		}

		return s.repo.CreateListing(txCtx, created)
	})

	if err != nil {
		return Listing{}, err
	}
	return created, nil
}

// PurchaseListing allows a buyer character to purchase a listed item.
func (s *Service) PurchaseListing(
	ctx context.Context,
	buyerCharacterID string,
	listingID string,
	now time.Time,
) (PurchaseResult, error) {
	if strings.TrimSpace(buyerCharacterID) == "" || strings.TrimSpace(listingID) == "" {
		return PurchaseResult{}, ErrInvalidInput
	}

	var result PurchaseResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock listing first (CAS / P2P shared entity lock)
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return ErrListingNotFound
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}
		if listing.SellerCharacterID == buyerCharacterID {
			return ErrCannotBuyOwnListing
		}

		// 2. Deterministic lock acquisition order for characters (ascending ID order)
		firstID, secondID := listing.SellerCharacterID, buyerCharacterID
		if firstID > secondID {
			firstID, secondID = buyerCharacterID, listing.SellerCharacterID
		}

		firstChar, err := s.charRepo.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return ErrCharacterNotFound
		}
		secondChar, err := s.charRepo.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return ErrCharacterNotFound
		}

		var buyerChar, sellerChar corecharacter.Character
		if firstChar.ID == buyerCharacterID {
			buyerChar = firstChar
			sellerChar = secondChar
		} else {
			buyerChar = secondChar
			sellerChar = firstChar
		}

		// 3. Verify buyer funds
		if buyerChar.Money < listing.Price {
			return ErrInsufficientGold
		}

		// 4. Lock buyer inventory
		buyerInv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, buyerCharacterID)
		if err != nil {
			return err
		}

		// 5. Transfer Gold
		buyerChar.Money -= listing.Price
		if sellerChar.Money > MaxGoldCap-listing.Price {
			sellerChar.Money = MaxGoldCap
		} else {
			sellerChar.Money += listing.Price
		}

		// 6. Deliver Item to Buyer
		itemInst, err := coreitem.NewInstance(listing.ItemID, 1)
		if err != nil {
			return err
		}
		if err := buyerInv.Add(itemInst); err != nil {
			return err
		}

		// 7. Update listing status
		listing.Status = StatusSold
		listing.SoldAt = &now
		listing.BuyerCharacterID = &buyerCharacterID
		buyerName := buyerChar.Name
		listing.BuyerName = &buyerName

		// 8. Persist mutations
		if err := s.charRepo.Update(txCtx, buyerChar); err != nil {
			return err
		}
		if err := s.charRepo.Update(txCtx, sellerChar); err != nil {
			return err
		}
		if err := s.invRepo.Save(txCtx, buyerInv); err != nil {
			return err
		}
		if err := s.repo.UpdateListing(txCtx, listing); err != nil {
			return err
		}

		result = PurchaseResult{
			Listing:      listing,
			BuyerGold:    buyerChar.Money,
			SellerGold:   sellerChar.Money,
			ItemInstance: itemInst,
		}

		return nil
	})

	if err != nil {
		return PurchaseResult{}, err
	}
	return result, nil
}

// CancelListing allows the seller to cancel an active listing and retrieve their item.
func (s *Service) CancelListing(
	ctx context.Context,
	sellerCharacterID string,
	listingID string,
) (Listing, error) {
	if strings.TrimSpace(sellerCharacterID) == "" || strings.TrimSpace(listingID) == "" {
		return Listing{}, ErrInvalidInput
	}

	var cancelled Listing

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock listing
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return ErrListingNotFound
		}
		if listing.SellerCharacterID != sellerCharacterID {
			return ErrUnauthorizedSeller
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}

		// 2. Lock seller inventory
		inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		// 3. Return item instance to seller inventory
		itemInst, err := coreitem.NewInstance(listing.ItemID, 1)
		if err != nil {
			return err
		}
		if err := inv.Add(itemInst); err != nil {
			return err
		}
		if err := s.invRepo.Save(txCtx, inv); err != nil {
			return err
		}

		// 4. Update listing status
		listing.Status = StatusCancelled
		if err := s.repo.UpdateListing(txCtx, listing); err != nil {
			return err
		}

		cancelled = listing
		return nil
	})

	if err != nil {
		return Listing{}, err
	}
	return cancelled, nil
}

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
