package fleamarket

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	ServerMaxListings       = 120
	MaxListingsPerCharacter = 5
	MinListingPrice         = 1
	MaxListingPrice         = 999999
)

type ListingStatus string

const (
	StatusActive    ListingStatus = "active"
	StatusSold      ListingStatus = "sold"
	StatusCancelled ListingStatus = "cancelled"
)

var (
	ErrNilDependency            = errors.New("fleamarket dependency is nil")
	ErrCharacterNotFound        = errors.New("character not found")
	ErrListingNotFound          = errors.New("fleamarket listing not found")
	ErrListingNotActive         = errors.New("fleamarket listing is not active")
	ErrCannotBuyOwnListing      = errors.New("cannot purchase your own flea market listing")
	ErrUnauthorizedSeller       = errors.New("only the seller can cancel this listing")
	ErrInsufficientGold         = errors.New("insufficient gold to purchase listing")
	ErrMaxListingsReached       = errors.New("maximum active listings limit reached")
	ErrServerMaxListingsReached = errors.New("server-wide maximum active listings limit (120) reached")
	ErrInvalidPrice             = errors.New("listing price must be between 1 and 999999 gold")
	ErrItemNotInDepot           = errors.New("item not found in depot storage")
	ErrInvalidInput             = errors.New("invalid flea market input parameters")
)

type Listing struct {
	ID                string        `json:"id"`
	SellerCharacterID string        `json:"seller_character_id"`
	SellerName        string        `json:"seller_name"`
	ItemID            string        `json:"item_id"`
	ItemName          string        `json:"item_name"`
	ItemCategory      string        `json:"item_category"`
	EnhancementLevel  int           `json:"enhancement_level"`
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
	GetListingByIDForUpdate(ctx context.Context, id string) (Listing, error)
	ListActiveListings(ctx context.Context, limit, offset int) ([]Listing, int, error)
	GetActiveListingsBySeller(ctx context.Context, sellerCharacterID string) ([]Listing, error)
	CountActiveListingsBySeller(ctx context.Context, sellerCharacterID string) (int, error)
	LockListingCapacityForUpdate(ctx context.Context) error
	CountTotalActiveListings(ctx context.Context) (int, error)
	UpdateListing(ctx context.Context, listing Listing) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

type ItemDefinitionProvider = coreitem.DefinitionProvider

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Service struct {
	mutationCheck      func(context.Context, string) error
	repo               FleaMarketRepository
	charRepo           CharacterRepository
	depotRepo          DepotRepository
	itemDefs           ItemDefinitionProvider
	txProvider         TransactionProvider
	collectionRecorder CollectionRecorder
}

type Option func(*Service)

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.collectionRecorder = recorder
	}
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.collectionRecorder = recorder
}

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
	depotRepo DepotRepository,
	opts ...Option,
) (*Service, error) {
	if repo == nil || charRepo == nil || depotRepo == nil {
		return nil, ErrNilDependency
	}

	svc := &Service{
		repo:      repo,
		charRepo:  charRepo,
		depotRepo: depotRepo,
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

// lockCharacter reports only a true missing row as not found; other read
// failures keep their cause so an unknown outcome is never a known rejection.
func (s *Service) lockCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	char, err := s.charRepo.FindByIDForUpdate(ctx, characterID)
	if errors.Is(err, corecharacter.ErrNotFound) {
		return corecharacter.Character{}, ErrCharacterNotFound
	}
	return char, err
}

// CreateListing lists an item from the character's depot onto the flea market.
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
		// 1. Serialize creators so the server-wide count cannot go stale (Rank 0).
		// Purchase and cancel only free slots, so they do not take this lock.
		if err := s.repo.LockListingCapacityForUpdate(txCtx); err != nil {
			return err
		}
		totalActive, err := s.repo.CountTotalActiveListings(txCtx)
		if err != nil {
			return err
		}
		if totalActive >= ServerMaxListings {
			return ErrServerMaxListingsReached
		}

		// 2. Lock seller character (Rank 2)
		seller, err := s.lockCharacter(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, sellerCharacterID); err != nil {
			return err
		}

		// 3. Check active listing count limit (5 + OverFlea)
		activeCount, err := s.repo.CountActiveListingsBySeller(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}
		maxListings := MaxListingsPerCharacter + seller.OverFlea
		if activeCount >= maxListings {
			return ErrMaxListingsReached
		}

		// 4. Lock seller depot (Rank 5)
		dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		// 5. Locate item in depot
		var targetInstanceID string
		for _, item := range dep.Items {
			if item.ID == itemInstanceOrDefID || item.DefinitionID == itemInstanceOrDefID {
				targetInstanceID = item.ID
				break
			}
		}
		if targetInstanceID == "" {
			return ErrItemNotInDepot
		}

		// 6. Consume 1 unit from depot
		consumedItem, err := dep.ConsumeOne(targetInstanceID)
		if err != nil {
			return ErrItemNotInDepot
		}
		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		// 7. Fetch definition for item metadata
		itemName := consumedItem.DefinitionID
		itemCategory := "misc"
		if s.itemDefs != nil {
			if def, defErr := s.itemDefs.FindByID(consumedItem.DefinitionID); defErr == nil {
				itemName = def.Name
				if def.Slot != "" {
					itemCategory = string(def.Slot)
				} else {
					itemCategory = "consumable"
				}
			}
		}

		// 8. Create Listing record
		created = Listing{
			ID:                id.New(),
			SellerCharacterID: seller.ID,
			SellerName:        seller.Name,
			ItemID:            consumedItem.DefinitionID,
			ItemName:          itemName,
			ItemCategory:      itemCategory,
			EnhancementLevel:  consumedItem.EnhancementLevel,
			Price:             price,
			Status:            StatusActive,
			CreatedAt:         now,
		}

		if err := s.repo.CreateListing(txCtx, created); err != nil {
			return err
		}
		if s.collectionRecorder != nil {
			if err := s.collectionRecorder.RecordItemDiscovered(txCtx, seller.ID, consumedItem.DefinitionID, itemName, itemCategory); err != nil {
				return err
			}
		}
		return nil
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
		// 1. Lock listing first (CAS / P2P shared entity lock, Rank 0)
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return err
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}
		if listing.SellerCharacterID == buyerCharacterID {
			return ErrCannotBuyOwnListing
		}

		// 2. Deterministic lock acquisition order for characters (ascending ID order, Rank 2)
		firstID, secondID := id.Sort2(listing.SellerCharacterID, buyerCharacterID)

		firstChar, err := s.lockCharacter(txCtx, firstID)
		if err != nil {
			return err
		}
		secondChar, err := s.lockCharacter(txCtx, secondID)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, buyerCharacterID); err != nil {
			return err
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

		// 4. Lock buyer depot (Rank 5)
		buyerDepot, err := depot.FindOrCreate(txCtx, s.depotRepo, buyerChar)
		if err != nil {
			return err
		}

		// 5. Transfer Gold
		if err := buyerChar.DeductMoney(listing.Price); err != nil {
			return ErrInsufficientGold
		}
		if err := sellerChar.AddMoney(listing.Price); err != nil {
			return err
		}

		// 6. Deliver Item to Buyer Depot (legacy &send_item)
		itemInst, err := coreitem.NewInstanceWithEnhancement(listing.ItemID, 1, listing.EnhancementLevel)
		if err != nil {
			return err
		}
		buyerDepot.Deliver(itemInst)

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
		if err := s.depotRepo.Save(txCtx, buyerDepot); err != nil {
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
		// 1. Lock listing (Rank 0)
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return err
		}
		if listing.SellerCharacterID != sellerCharacterID {
			return ErrUnauthorizedSeller
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}

		// 2. Lock seller character (Rank 2)
		sellerChar, err := s.lockCharacter(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, sellerCharacterID); err != nil {
			return err
		}

		// 3. Lock seller depot (Rank 5)
		sellerDepot, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}
		sellerDepot.RefreshCapacity(sellerChar.JobLevel, sellerChar.OverDepot)

		// 4. Return item instance to seller depot (legacy &send_item)
		itemInst, err := coreitem.NewInstanceWithEnhancement(listing.ItemID, 1, listing.EnhancementLevel)
		if err != nil {
			return err
		}
		sellerDepot.Deliver(itemInst)
		if err := s.depotRepo.Save(txCtx, sellerDepot); err != nil {
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
