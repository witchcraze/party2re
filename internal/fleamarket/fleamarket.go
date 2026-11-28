package fleamarket

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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
	ID string `json:"id"`
	// Number is the lowest number free among active listings when it was listed
	// (free.cgi number_select); purchase and cancel log rows name it.
	Number            int           `json:"number"`
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
	ItemInstance coreitem.Instance `json:"item_instance"`
}

type FleaMarketRepository interface {
	CreateListing(ctx context.Context, listing Listing) error
	GetListingByIDForUpdate(ctx context.Context, id string) (Listing, error)
	ListActiveListings(ctx context.Context, limit, offset int) ([]Listing, int, error)
	GetActiveListingsBySeller(ctx context.Context, sellerCharacterID string) ([]Listing, error)
	CountActiveListingsBySeller(ctx context.Context, sellerCharacterID string) (int, error)
	LockListingCapacityForUpdate(ctx context.Context) error
	// ActiveListingNumbers returns every active listing's number, ascending.
	ActiveListingNumbers(ctx context.Context) ([]int, error)
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

type InventoryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

// PaymentHolder holds a Flea Market sale payment for the seller at the Depot
// and raises the Home money-arrival notice inside the caller's transaction.
type PaymentHolder interface {
	HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error
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
	invRepo            InventoryRepository
	itemDefs           ItemDefinitionProvider
	txProvider         TransactionProvider
	collectionRecorder CollectionRecorder
	payment            PaymentHolder
}

type Option func(*Service)

func WithPaymentHolder(holder PaymentHolder) Option {
	return func(s *Service) {
		s.payment = holder
	}
}

func (s *Service) SetPaymentHolder(holder PaymentHolder) {
	s.payment = holder
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.collectionRecorder = recorder
	}
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.collectionRecorder = recorder
}

func WithInventoryRepository(invRepo InventoryRepository) Option {
	return func(s *Service) {
		s.invRepo = invRepo
	}
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
// Transaction: RunInTx.
// Lock Order: fleamarket_capacity_lock(0) -> characters(2) -> inventory_items(3) -> character_depots/depot_items(5).
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
		numbers, err := s.repo.ActiveListingNumbers(txCtx)
		if err != nil {
			return err
		}
		if len(numbers) >= ServerMaxListings {
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

		// 3.5. Lock seller hand inventory (Rank 3) for legacy add_collection on listing
		var inv coreinventory.Inventory
		if s.collectionRecorder != nil && s.invRepo != nil {
			inv, err = s.invRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
			if err != nil {
				return err
			}
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
			Number:            lowestFreeNumber(numbers),
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
		// Legacy free.cgi:130-131 calls add_collection, which registers the lister's held hand items, not the listed Depot item.
		if s.collectionRecorder != nil {
			for _, held := range inv.Items {
				heldName := held.DefinitionID
				handKind := coreitem.LegacyHandKindByID(held.DefinitionID)
				if s.itemDefs != nil {
					if def, defErr := s.itemDefs.FindByID(held.DefinitionID); defErr == nil {
						heldName = def.Name
						handKind = def.LegacyHandKind()
					}
				}
				if err := s.collectionRecorder.RecordItemDiscovered(txCtx, seller.ID, held.DefinitionID, heldName, collectionCategoryForHandKind(handKind)); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return Listing{}, err
	}
	return created, nil
}

func collectionCategoryForHandKind(kind int) string {
	switch kind {
	case 1:
		return "weapon"
	case 2:
		return "armor"
	default:
		return "item"
	}
}

// lowestFreeNumber returns the smallest positive number missing from the
// ascending, distinct numbers (free.cgi number_select).
func lowestFreeNumber(numbers []int) int {
	next := 1
	for _, n := range numbers {
		if n != next {
			break
		}
		next++
	}
	return next
}
